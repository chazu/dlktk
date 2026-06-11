// Package proto dispatches dialectical moves: it enforces legality (via ibis)
// then performs the corresponding store writes. Each move is atomic at the
// store layer. proto is the only thing above af/ibis/store that mutates state.
package proto

import (
	"fmt"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/id"
	"github.com/chazu/dlktk/internal/store"
)

// Mover performs moves attributed to a given author.
type Mover struct {
	s      *store.Store
	author string
}

// New returns a Mover writing as author.
func New(s *store.Store, author string) *Mover { return &Mover{s: s, author: author} }

// Raise adds an issue, optionally responding to a parent issue, with the given
// cardinality (empty defaults to select_one). Cardinality is fixed at creation
// (design §16 Q7). Returns the new issue id.
func (m *Mover) Raise(disc, text, parent string, card ibis.Cardinality) (string, error) {
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return "", err
	}
	if err := g.CanRaise(parent); err != nil {
		return "", err
	}
	if card == "" {
		card = ibis.SelectOne
	}
	nid := id.New()
	if err := m.s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Issue, Text: text, Author: m.author}); err != nil {
		return "", err
	}
	if err := m.s.SetIssueCard(ibis.IssueCard{Issue: nid, Cardinality: card}); err != nil {
		return "", err
	}
	if parent != "" {
		if err := m.addLink(disc, nid, parent, ibis.RespondsTo); err != nil {
			return "", err
		}
	}
	return nid, nil
}

// Propose adds a position responding to an issue. Returns the new position id.
func (m *Mover) Propose(disc, issue, text string) (string, error) {
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return "", err
	}
	if err := g.CanPropose(issue); err != nil {
		return "", err
	}
	nid := id.New()
	if err := m.s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Position, Text: text, Author: m.author}); err != nil {
		return "", err
	}
	if err := m.addLink(disc, nid, issue, ibis.RespondsTo); err != nil {
		return "", err
	}
	return nid, nil
}

// Support adds an argument supporting a position or argument.
func (m *Mover) Support(disc, target, text string) (string, error) {
	return m.attach(disc, target, text, ibis.Supports)
}

// Object adds an argument objecting to a position or argument.
func (m *Mover) Object(disc, target, text string) (string, error) {
	return m.attach(disc, target, text, ibis.ObjectsTo)
}

func (m *Mover) attach(disc, target, text string, rel ibis.Rel) (string, error) {
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return "", err
	}
	if err := g.CanAttach(target, rel); err != nil {
		return "", err
	}
	nid := id.New()
	if err := m.s.AddNode(ibis.Node{ID: nid, Disc: disc, Kind: ibis.Argument, Text: text, Author: m.author}); err != nil {
		return "", err
	}
	if err := m.addLink(disc, nid, target, rel); err != nil {
		return "", err
	}
	return nid, nil
}

// Prefer records winner preferred to loser with a basis label.
func (m *Mover) Prefer(disc, winner, loser, basis string) (string, error) {
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return "", err
	}
	if err := g.CanPrefer(winner, loser); err != nil {
		return "", err
	}
	pid := id.New()
	if err := m.s.AddPreference(ibis.Preference{
		ID: pid, Disc: disc, Winner: winner, Loser: loser, Basis: basis, Author: m.author,
	}); err != nil {
		return "", err
	}
	return pid, nil
}

// Decide closes an issue by accepting a position. The override flag is set when
// the accepted position is not IN under the current grounded labelling. A bare
// re-decide on an already-decided issue is rejected: overturning a standing
// decision must go through Supersede so the reversal carries a recorded basis
// (design §16 Q4).
func (m *Mover) Decide(disc, issue, position, basis string) error {
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return err
	}
	if err := g.CanDecide(issue, position); err != nil {
		return err
	}
	if prior, err := m.standingDecision(disc, issue); err != nil {
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
	return m.s.AddDecision(ibis.Decision{
		Disc: disc, Issue: issue, Position: position, Basis: basis,
		Decider: m.author, Override: labels[position] != af.IN,
	})
}

// Supersede overturns the standing decision on an issue with a new one. The
// basis is mandatory — the whole point of the move is forcing the reasoning for
// the reversal to be captured — and the new decision links the position it
// supersedes (design §16 Q4).
func (m *Mover) Supersede(disc, issue, position, basis string) error {
	if basis == "" {
		return &ibis.IllegalMove{Node: issue,
			Detail: "supersede requires --basis: record why the prior decision is overturned"}
	}
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return err
	}
	if err := g.CanDecide(issue, position); err != nil {
		return err
	}
	prior, err := m.standingDecision(disc, issue)
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
	if err := m.s.SupersedeDecision(disc, issue); err != nil {
		return err
	}
	return m.s.AddDecision(ibis.Decision{
		Disc: disc, Issue: issue, Position: position, Basis: basis,
		Decider: m.author, Override: labels[position] != af.IN,
		Supersedes: prior.Position,
	})
}

// standingDecision returns the in-force decision on an issue, or nil.
func (m *Mover) standingDecision(disc, issue string) (*ibis.Decision, error) {
	decs, err := m.s.Decisions(disc, store.Now())
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
	g, err := m.s.Graph(disc, store.Now())
	if err != nil {
		return err
	}
	if err := g.CanConcede(node, m.author); err != nil {
		return err
	}
	return m.s.RetractNode(node)
}

func (m *Mover) addLink(disc, src, dst string, rel ibis.Rel) error {
	return m.s.AddLink(ibis.Link{ID: id.New(), Disc: disc, Src: src, Dst: dst, Rel: rel, Author: m.author})
}
