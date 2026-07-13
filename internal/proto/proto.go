// Package proto dispatches dialectical moves: it enforces legality (via ibis)
// then performs the corresponding store writes. Each move runs its legality
// check and its writes inside one store.Move transaction, so the check cannot
// interleave with a concurrent agent's write (multi-process included) and a
// partial move never lands. proto is the only thing above af/ibis/store that
// mutates state.
package proto

import (
	"fmt"
	"strings"
	"time"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/id"
	"github.com/chazu/dlktk/internal/render"
	"github.com/chazu/dlktk/internal/store"
)

// Mover performs moves attributed to a given author (the stable ownership
// identity) acting under an optional role (persona). The two are distinct:
// concede/retract ownership rides on author, never the spoofable persona. When a
// role is set, every move auto-records the author↔role roster binding for the
// discussion (idempotent; design §16 Q8).
type Mover struct {
	s      *store.Store
	author string
	role   string
}

// New returns a Mover writing as author under role (role may be empty).
func New(s *store.Store, author, role string) *Mover {
	return &Mover{s: s, author: author, role: role}
}

// ensureRoster auto-records the author↔role binding for disc when a role is set.
// Called inside each move's transaction, so the binding lands atomically with the
// move (and rolls back if the move is illegal). Content-addressing dedups, so a
// repeated binding is a no-op at storage.
func (m *Mover) ensureRoster(s *store.Store, disc string) error {
	if m.role == "" {
		return nil
	}
	return s.AddRoster(ibis.Roster{Disc: disc, Author: m.author, Role: m.role})
}

// Raise adds an issue, optionally responding to a parent issue or recording
// the position/argument that revealed it (from), with the given cardinality
// (empty defaults to select_one). Cardinality is fixed at creation (design §16
// Q7). Returns the new issue id.
func (m *Mover) Raise(disc, text, parent, from string, card ibis.Cardinality) (string, error) {
	nid := id.New()
	err := m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanRaise(parent, from); err != nil {
			return err
		}
		if card == "" {
			card = ibis.SelectOne
		}
		if err := s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Issue, Text: text, Author: m.author}); err != nil {
			return err
		}
		if err := s.SetIssueCard(ibis.IssueCard{Issue: nid, Cardinality: card}); err != nil {
			return err
		}
		if parent != "" {
			return m.addLink(s, disc, nid, parent, ibis.RespondsTo)
		}
		if from != "" {
			return m.addLink(s, disc, nid, from, ibis.RaisedFrom)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return nid, nil
}

// Reframe replaces an issue's framing: it raises a fresh issue (its own
// cardinality; positions do not carry over — under a new framing they mean
// different things, the Q7 argument) and records the lineage with a mandatory
// basis. A decided issue cannot be reframed: supersede the decision first, or
// the reframe would silently bury it. Returns the new issue id.
func (m *Mover) Reframe(disc, old, text, basis string, card ibis.Cardinality) (string, error) {
	if basis == "" {
		return "", &ibis.IllegalMove{Node: old,
			Detail: "reframe requires --basis: record why the framing is replaced"}
	}
	nid := id.New()
	err := m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanReframe(old); err != nil {
			return err
		}
		if prior, err := standingDecision(s, disc, old); err != nil {
			return err
		} else if prior != nil {
			return &ibis.IllegalMove{Node: old, Detail: fmt.Sprintf(
				"issue %s has a standing decision (-> %s); supersede it before reframing, or raise a fresh issue", old, prior.Position)}
		}
		if card == "" {
			card = ibis.SelectOne
		}
		if err := s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Issue, Text: text, Author: m.author}); err != nil {
			return err
		}
		if err := s.SetIssueCard(ibis.IssueCard{Issue: nid, Cardinality: card}); err != nil {
			return err
		}
		return s.AddReframe(ibis.Reframe{Disc: disc, Old: old, New: nid, Basis: basis, Author: m.author})
	})
	if err != nil {
		return "", err
	}
	return nid, nil
}

// Propose adds a position responding to an issue, optionally recording the
// value it promotes. Returns the new position id.
func (m *Mover) Propose(disc, issue, text, promotes string) (string, error) {
	nid := id.New()
	err := m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanPropose(issue); err != nil {
			return err
		}
		if err := s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Position, Text: text, Author: m.author}); err != nil {
			return err
		}
		if err := m.addValue(s, disc, nid, promotes); err != nil {
			return err
		}
		return m.addLink(s, disc, nid, issue, ibis.RespondsTo)
	})
	if err != nil {
		return "", err
	}
	return nid, nil
}

// Synthesize adds a hybrid position recombining two or more existing positions
// on the same issue, recording the lineage and what the hybrid explicitly
// drops from its parents. The hybrid is an ordinary position to the evaluator
// — on a select_one issue it joins the rivalry, parents included, until they
// are conceded or a preference/audience elevates it. The returned warnings
// are advisory (the move stands): a ≥3-parent synthesis with no recorded
// drops is warned about, because a synthesis that drops nothing is a bundle
// (wicked-problems-2.md item 4).
func (m *Mover) Synthesize(disc, issue, text string, froms []string, promotes string, drops []string) (string, []string, error) {
	nid := id.New()
	err := m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanSynthesize(issue, froms); err != nil {
			return err
		}
		if err := s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Position, Text: text, Author: m.author, Drops: drops}); err != nil {
			return err
		}
		if err := m.addValue(s, disc, nid, promotes); err != nil {
			return err
		}
		if err := m.addLink(s, disc, nid, issue, ibis.RespondsTo); err != nil {
			return err
		}
		for _, f := range froms {
			if err := m.addLink(s, disc, nid, f, ibis.Synthesizes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	var warnings []string
	if len(froms) >= 3 && len(drops) == 0 {
		warnings = append(warnings, fmt.Sprintf(
			"synthesis %s recombines %d parents and records no drops — a synthesis that drops nothing is a bundle; state what it excludes with --drops (check --strict reports bundle_synthesis if it is decided as-is)",
			nid, len(froms)))
	}
	return nid, warnings, nil
}

// Support adds an argument supporting a position or argument. A non-empty
// answers names a parent objection this support dismisses on a synthesis
// ("here is why the hybrid escapes it") — recorded as an evaluator-inert
// addresses link.
func (m *Mover) Support(disc, target, text, promotes, answers string) (string, error) {
	return m.attach(disc, target, text, ibis.Supports, "", promotes, answers)
}

// Object adds an argument objecting to a position or argument. A non-empty
// answers names a parent objection this objection re-aims at a synthesis
// ("this still applies") — recorded as an evaluator-inert addresses link.
func (m *Mover) Object(disc, target, text, promotes, answers string) (string, error) {
	return m.attach(disc, target, text, ibis.ObjectsTo, "", promotes, answers)
}

// Assume adds an argument tagged as an assumption — a challengeable premise
// its target rests on. It supports the target (inert in the labelling, §3.5);
// the tag is bookkeeping that agenda/check use to surface unexamined or
// defeated premises.
func (m *Mover) Assume(disc, target, text string) (string, error) {
	return m.attach(disc, target, text, ibis.Supports, ibis.TagAssumption, "", "")
}

func (m *Mover) attach(disc, target, text string, rel ibis.Rel, tag, promotes, answers string) (string, error) {
	nid := id.New()
	err := m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanAttach(target, rel); err != nil {
			return err
		}
		if answers != "" {
			if err := g.CanAnswer(target, answers); err != nil {
				return err
			}
		}
		if err := s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Argument, Text: text, Author: m.author, Tag: tag}); err != nil {
			return err
		}
		if err := m.addValue(s, disc, nid, promotes); err != nil {
			return err
		}
		if err := m.addLink(s, disc, nid, target, rel); err != nil {
			return err
		}
		if answers != "" {
			return m.addLink(s, disc, nid, answers, ibis.Addresses)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return nid, nil
}

// Promote tags an existing node with the value it promotes (audience lens
// input). Ownership is checked: a value changes the node's fate under every
// audience and is otherwise unretractable.
func (m *Mover) Promote(disc, node, value string) error {
	if value == "" {
		return &ibis.IllegalMove{Node: node, Detail: "promote requires a value"}
	}
	return m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanPromote(node, m.author); err != nil {
			return err
		}
		return s.AddValue(ibis.ValueTag{Disc: disc, Node: node, Value: value, Author: m.author})
	})
}

// addValue writes an optional value tag for a node the mover just created (no
// ownership check needed: the node is the mover's own).
func (m *Mover) addValue(s *store.Store, disc, node, value string) error {
	if value == "" {
		return nil
	}
	return s.AddValue(ibis.ValueTag{Disc: disc, Node: node, Value: value, Author: m.author})
}

// DeclareAudience records a named strict value ranking. Re-declaring an
// existing name requires supersede=true and a basis — retiring a ranking that
// every robustness verdict depends on must record why (the Q4 pattern); the
// prior fact's vt interval is closed atomically with the new declaration.
func (m *Mover) DeclareAudience(disc, name string, ranking []string, supersede bool, basis string) error {
	if supersede && basis == "" {
		return &ibis.IllegalMove{Node: name,
			Detail: "audience --supersede requires --basis: record why the prior ranking is retired"}
	}
	return m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanAudience(name, ranking, supersede); err != nil {
			return err
		}
		if supersede {
			if err := s.SupersedeAudience(disc, name); err != nil {
				return err
			}
		}
		return s.AddAudience(ibis.Audience{Disc: disc, Name: name, Ranking: ranking, Basis: basis, Author: m.author})
	})
}

// Prefer records winner preferred to loser with a basis label. The acyclicity
// check and the write share one transaction: two agents concurrently asserting
// `prefer A B` and `prefer B A` can no longer both pass the check (§16 Q2's
// all-prefer-all collapse via the ANALYSIS §1.4 race).
//
// The returned warnings are advisory (the move stands): burying a synthesis
// parent whose undefeated objections have not been addressed on the hybrid is
// the subsumption dodge — if the hybrid truly contains the parent, the
// parent's unanswered critics apply to it (wicked-problems-2.md item 3).
func (m *Mover) Prefer(disc, winner, loser, basis string) (string, []string, error) {
	pid := id.New()
	var warnings []string
	err := m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanPrefer(winner, loser); err != nil {
			return err
		}
		if fw, err := af.Build(g); err == nil {
			if open, flagged := render.SelfElevation(g, fw.Grounded(), winner, loser); flagged {
				detail := "all recorded addresses are self-authored"
				if len(open) > 0 {
					detail = "open: " + strings.Join(open, ", ")
				}
				warnings = append(warnings, fmt.Sprintf(
					"self-elevated synthesis: %s subsumes %s but %s's undefeated objections are not answered on the hybrid (%s) — object/support %s --answers <id> first (the preference stands; check --strict reports self_elevated_synthesis)",
					winner, loser, loser, detail, winner))
			}
		}
		return s.AddPreference(ibis.Preference{
			ID: pid, Disc: disc, Winner: winner, Loser: loser, Basis: basis, Author: m.author,
		})
	})
	if err != nil {
		return "", nil, err
	}
	return pid, warnings, nil
}

// Decide closes an issue by accepting a position. The override flag is set when
// the accepted position is not IN under the current grounded labelling. A bare
// re-decide on an already-decided issue is rejected: overturning a standing
// decision must go through Supersede so the reversal carries a recorded basis
// (design §16 Q4). reviewBy (unix seconds, 0 = none) records a re-examination
// horizon: check reports the decision once the horizon passes.
func (m *Mover) Decide(disc, issue, position, basis string, reviewBy int64) error {
	if err := validReviewBy(issue, reviewBy); err != nil {
		return err
	}
	return m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanDecide(issue, position); err != nil {
			return err
		}
		if prior, err := standingDecision(s, disc, issue); err != nil {
			return err
		} else if prior != nil {
			return &ibis.IllegalMove{
				Node: issue,
				Detail: fmt.Sprintf("issue %s is already decided (-> %s); use `supersede %s <position> --basis <label>` to overturn it",
					issue, prior.Position, issue),
			}
		}
		fw, err := af.Build(g)
		if err != nil {
			return err
		}
		labels := fw.Grounded()
		return s.AddDecision(ibis.Decision{
			Disc: disc, Issue: issue, Position: position, Basis: basis,
			Decider: m.author, Override: labels[position] != af.IN,
			ReviewBy: reviewBy,
		})
	})
}

// validReviewBy rejects a review horizon that is already in the past.
func validReviewBy(issue string, reviewBy int64) error {
	if reviewBy != 0 && reviewBy <= time.Now().Unix() {
		return &ibis.IllegalMove{Node: issue,
			Detail: "review-by must be in the future (it records the re-examination horizon)"}
	}
	return nil
}

// Supersede overturns the standing decision on an issue with a new one. The
// basis is mandatory — the whole point of the move is forcing the reasoning for
// the reversal to be captured — and the new decision links the position it
// supersedes (design §16 Q4). Closing the prior decision and recording the new
// one are atomic: no window where the issue stands undecided. Superseding with
// the same position and a fresh reviewBy is how a review horizon is re-armed.
func (m *Mover) Supersede(disc, issue, position, basis string, reviewBy int64) error {
	if basis == "" {
		return &ibis.IllegalMove{Node: issue,
			Detail: "supersede requires --basis: record why the prior decision is overturned"}
	}
	if err := validReviewBy(issue, reviewBy); err != nil {
		return err
	}
	return m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanDecide(issue, position); err != nil {
			return err
		}
		prior, err := standingDecision(s, disc, issue)
		if err != nil {
			return err
		}
		if prior == nil {
			return &ibis.IllegalMove{Node: issue,
				Detail: fmt.Sprintf("issue %s has no standing decision to supersede; use decide", issue)}
		}
		fw, err := af.Build(g)
		if err != nil {
			return err
		}
		labels := fw.Grounded()
		// Close the prior decision's vt interval, then record the new one.
		if err := s.SupersedeDecision(disc, issue); err != nil {
			return err
		}
		return s.AddDecision(ibis.Decision{
			Disc: disc, Issue: issue, Position: position, Basis: basis,
			Decider: m.author, Override: labels[position] != af.IN,
			Supersedes: prior.Position, ReviewBy: reviewBy,
		})
	})
}

// standingDecision returns the in-force decision on an issue, or nil.
func standingDecision(s *store.Store, disc, issue string) (*ibis.Decision, error) {
	decs, err := s.Decisions(disc, store.Now())
	if err != nil {
		return nil, err
	}
	for i := range decs {
		if decs[i].Issue == issue {
			return &decs[i], nil
		}
	}
	return nil, nil
}

// Concede withdraws one of the author's own nodes (alias: retract).
func (m *Mover) Concede(disc, node string) error {
	return m.s.Move(func(s *store.Store) error {
		if err := m.ensureRoster(s, disc); err != nil {
			return err
		}
		g, err := s.Graph(disc, store.Now())
		if err != nil {
			return err
		}
		if err := g.CanConcede(node, m.author); err != nil {
			return err
		}
		return s.RetractNode(node)
	})
}

func (m *Mover) addLink(s *store.Store, disc, src, dst string, rel ibis.Rel) error {
	return s.AddLink(ibis.Link{ID: id.New(), Disc: disc, Src: src, Dst: dst, Rel: rel, Author: m.author})
}
