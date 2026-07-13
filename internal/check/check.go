// Package check verifies that a store's dialectics still stand: standing
// decisions whose position is no longer justified (drift), lingering
// stalemates, and store-invariant violations. It is the engine behind
// `dlktk check`, designed to run in CI so recorded decisions stay living
// constraints rather than archaeology.
package check

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/render"
	"github.com/chazu/dlktk/internal/store"
)

// Finding kinds.
const (
	DecisionDrift         = "decision_drift"          // decided position no longer IN (error)
	PreferenceCycle       = "preference_cycle"        // stored preferences cyclic (error)
	StoreInvariant        = "store_invariant"         // e.g. duplicate current node ids (error)
	Stalemate             = "stalemate"               // all positions UNDEC (warning)
	UntestedDecision      = "untested_decision"       // decided position never substantively attacked (warning)
	ReviewDue             = "review_due"              // decision's review horizon has passed (warning)
	DefeatedAssumption    = "defeated_assumption"     // decided position rests on an OUT assumption (warning)
	SelfElevatedSynthesis = "self_elevated_synthesis" // hybrid preferred over a parent whose objections it never answered (warning)
	BundleSynthesis       = "bundle_synthesis"        // decided ≥3-parent synthesis with no recorded drops (warning)
	MapDrift              = "map_drift"               // a mapped issue's current audience map differs from its decision-time map (warning)
	// SingleAuthorConvergence fires when a decided synthesis's scrutiny or
	// decision never left the synthesis author's own hands — the decider shares
	// the synthesis author, or every objection against it does. It tests the
	// shape of the scrutiny, not the number of author strings it wore (warning).
	SingleAuthorConvergence = "single_author_convergence"
	// MappedPendingGovernance is a non-fatal note (never fails a check, even
	// under --strict): a value-map decision defers "whose ranking governs?" and
	// that question should be raised as its own issue.
	MappedPendingGovernance = "mapped_pending_governance"
)

// Finding is one problem (or warning) detected in a discussion.
type Finding struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"` // "error" | "warning"
	Discussion string `json:"discussion"`
	Issue      string `json:"issue,omitempty"`
	Node       string `json:"node,omitempty"`
	Detail     string `json:"detail"`
}

// View is the check envelope. OK means no error-severity findings; warnings
// (stalemates) do not fail a non-strict check.
type View struct {
	Discussions int       `json:"discussions"`
	Findings    []Finding `json:"findings"`
	OK          bool      `json:"ok"`
}

// Run checks the given discussions at the given temporal viewpoint. nowUnix is
// the moment review horizons are judged against — the --as-of time when
// travelling, wall clock otherwise. Output is deterministic: discussions and
// issues are visited in sorted order.
func Run(s *store.Store, discs []string, w store.When, nowUnix int64) (View, error) {
	v := View{Discussions: len(discs)}
	sorted := append([]string{}, discs...)
	sort.Strings(sorted)
	for _, disc := range sorted {
		fs, err := runOne(s, disc, w, nowUnix)
		if err != nil {
			return View{}, err
		}
		v.Findings = append(v.Findings, fs...)
	}
	v.OK = true
	for _, f := range v.Findings {
		if f.Severity == "error" {
			v.OK = false
			break
		}
	}
	return v, nil
}

func runOne(s *store.Store, disc string, w store.When, nowUnix int64) ([]Finding, error) {
	var out []Finding

	// Store invariant: a node id must be current in at most one fact (§3.1).
	nodes, err := s.Nodes(disc, w)
	if err != nil {
		return nil, err
	}
	count := map[string]int{}
	for _, n := range nodes {
		count[n.ID]++
	}
	for _, id := range sortedKeys(count) {
		if count[id] > 1 {
			out = append(out, Finding{
				Kind: StoreInvariant, Severity: "error", Discussion: disc, Node: id,
				Detail: fmt.Sprintf("node %s is current in %d facts (must be at most one)", id, count[id]),
			})
		}
	}

	g, err := s.Graph(disc, w)
	if err != nil {
		return nil, err
	}
	fw, err := af.Build(g)
	var cyc *af.PreferenceCycleError
	if errors.As(err, &cyc) {
		// No labelling is computable over a cyclic preference relation; report
		// and move on to the next discussion.
		out = append(out, Finding{
			Kind: PreferenceCycle, Severity: "error", Discussion: disc, Node: cyc.Node,
			Detail: cyc.Error(),
		})
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	labels := fw.Grounded()
	decs, err := s.Decisions(disc, w)
	if err != nil {
		return nil, err
	}

	// Nodes whose untested_decision fires, for the item-3 suppression rule:
	// one defect, one finding (untested wins over self-elevated).
	untestedNodes := map[string]bool{}

	for _, issue := range g.Issues() {
		st := render.Status(g, fw, labels, issue, decs)
		// Each standing decision is drift-checked independently — an open issue
		// carries one per adopted position, a select_one issue at most one
		// (wicked-problems-2.md item 6).
		for di := range st.Decisions {
			d := st.Decisions[di]
			if !d.Override {
				// The position was IN when decided (no override flag); if it is no
				// longer IN, the dialectic has moved out from under the decision.
				if l := labels[d.Position]; l != af.IN {
					out = append(out, Finding{
						Kind: DecisionDrift, Severity: "error", Discussion: disc, Issue: issue, Node: d.Position,
						Detail: fmt.Sprintf("decided position %s is no longer justified (now %s); re-argue or supersede", d.Position, l),
					})
				}
			}
			// A decision that survived zero tests is the kind most likely to
			// rot: IN by silence, not by surviving attack. Substantive means
			// an objection from another author that participates in the
			// defeat relation — select_one rival edges, self-objections, and
			// preference-excused attacks don't count (wicked-problems-2.md
			// item 1).
			if labels[d.Position] == af.IN && !render.Tested(g, fw, d.Position) {
				untestedNodes[d.Position] = true
				out = append(out, Finding{
					Kind: UntestedDecision, Severity: "warning", Discussion: disc, Issue: issue, Node: d.Position,
					Detail: fmt.Sprintf("decided position %s never faced a substantive objection (rival edges, self-objections, and preference-excused attacks don't count); its IN label is unexamined, not vindicated — stress-test it", d.Position),
				})
			}
			// A decided synthesis of three or more parents that records no
			// drops is a bundle wearing a synthesis's clothes (item 4).
			if parents := g.SynthesisParents(d.Position); len(parents) >= 3 && len(g.Nodes[d.Position].Drops) == 0 {
				out = append(out, Finding{
					Kind: BundleSynthesis, Severity: "warning", Discussion: disc, Issue: issue, Node: d.Position,
					Detail: fmt.Sprintf("decided synthesis %s recombines %d parents and records no drops — a synthesis that drops nothing is a bundle; state what it excludes (concede and restate with --drops, or supersede)", d.Position, len(parents)),
				})
			}
			// Temporal drift: the recorded re-examination horizon has passed.
			if d.ReviewBy != 0 && d.ReviewBy < nowUnix {
				out = append(out, Finding{
					Kind: ReviewDue, Severity: "warning", Discussion: disc, Issue: issue, Node: d.Position,
					Detail: fmt.Sprintf("decision on %s is past its review horizon (%s); re-affirm or revise via supersede", issue, time.Unix(d.ReviewBy, 0).UTC().Format("2006-01-02")),
				})
			}
			// A rebuttal demolished a premise but nobody revisited the
			// conclusion standing on it.
			for _, a := range defeatedAssumptions(g, labels, d.Position) {
				out = append(out, Finding{
					Kind: DefeatedAssumption, Severity: "warning", Discussion: disc, Issue: issue, Node: a,
					Detail: fmt.Sprintf("decided position %s rests on assumption %s, which is now defeated (OUT); re-argue or supersede", d.Position, a),
				})
			}
			// single_author_convergence (item 8): a decided synthesis whose
			// scrutiny or decision never left the synthesis author's own hands —
			// the echo chamber regardless of how many --author strings it wore.
			// Suppressed when untested_decision already fired (the every-objection-
			// is-a-self-objection shape is that finding; one defect, one finding).
			if parents := g.SynthesisParents(d.Position); len(parents) > 0 && !untestedNodes[d.Position] {
				synthAuthor := g.Nodes[d.Position].Author
				objectors, selfOnly := 0, true
				for _, l := range g.Links {
					if l.Rel == ibis.ObjectsTo && l.Dst == d.Position {
						objectors++
						if l.Author != synthAuthor {
							selfOnly = false
						}
					}
				}
				if d.Decider == synthAuthor || (objectors > 0 && selfOnly) {
					out = append(out, Finding{
						Kind: SingleAuthorConvergence, Severity: "warning", Discussion: disc, Issue: issue, Node: d.Position,
						Detail: fmt.Sprintf("decided synthesis %s was scrutinised or decided only within its own author (%s): the decider and/or every objector share it. Roster separation proves attribution, not independence — run the devil's-advocate turn and the decide as a separate agent under a different author", d.Position, synthAuthor),
					})
				}
			}
		}
		if st.Stalemate && st.ReframedTo == "" {
			// A stalemate under a reframed (dead) framing is no longer the
			// live question; only current framings warrant the warning. A mapped
			// issue is closed (st.Stalemate is already false there).
			out = append(out, Finding{
				Kind: Stalemate, Severity: "warning", Discussion: disc, Issue: issue,
				Detail: fmt.Sprintf("all %d positions UNDEC; a preference is needed to resolve", len(st.Positions)),
			})
		}
		// A value-map decision records no verdicts; its object is derived. Verify
		// it still stands as a living constraint (wicked-problems-2.md item 7).
		if md := st.MapDecided; md != nil {
			out = append(out, mapFindings(s, g, disc, issue, md, nowUnix)...)
		}
	}

	// Self-elevated syntheses (item 3): a stored preference burying a parent
	// under its own hybrid while the parent's undefeated objections are not
	// answered on the hybrid. A current-graph property — an addressing node
	// conceded after the preference re-arms it. Suppressed when the winner
	// already carries untested_decision (one defect, one finding).
	seen := map[[2]string]bool{}
	for _, p := range g.Preferences {
		pair := [2]string{p.Winner, p.Loser}
		if seen[pair] || untestedNodes[p.Winner] {
			continue
		}
		seen[pair] = true
		open, flagged := render.SelfElevation(g, labels, p.Winner, p.Loser)
		if !flagged {
			continue
		}
		detail := "all recorded addresses are self-authored"
		if len(open) > 0 {
			detail = "open: " + strings.Join(open, ", ")
		}
		out = append(out, Finding{
			Kind: SelfElevatedSynthesis, Severity: "warning", Discussion: disc, Issue: issueOf(g, p.Winner), Node: p.Winner,
			Detail: fmt.Sprintf("synthesis %s is preferred over its parent %s, but the parent's undefeated objections are not answered on the hybrid (%s) — object/support %s --answers <id>", p.Winner, p.Loser, detail, p.Winner),
		})
	}
	return out, nil
}

// issueOf returns the issue a position responds to (first, if several).
func issueOf(g *ibis.Graph, position string) string {
	for _, l := range g.Links {
		if l.Rel == ibis.RespondsTo && l.Src == position {
			if n, ok := g.Nodes[l.Dst]; ok && n.Kind == ibis.Issue {
				return l.Dst
			}
		}
	}
	return ""
}

// defeatedAssumptions returns the OUT-labelled assumption nodes on the
// transitive supports-chain into a position, sorted.
func defeatedAssumptions(g *ibis.Graph, labels map[string]af.Label, position string) []string {
	supporters := map[string]bool{position: true}
	for changed := true; changed; {
		changed = false
		for _, l := range g.Links {
			if l.Rel == ibis.Supports && supporters[l.Dst] && !supporters[l.Src] {
				supporters[l.Src] = true
				changed = true
			}
		}
	}
	var out []string
	for id := range supporters {
		if n, ok := g.Nodes[id]; ok && n.Tag == ibis.TagAssumption && labels[id] == af.OUT {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Text renders a View as human output.
func Text(v View) string {
	var b strings.Builder
	fmt.Fprintf(&b, "checked %d discussion(s): %d finding(s)\n", v.Discussions, len(v.Findings))
	for _, f := range v.Findings {
		sev := "WARN "
		switch f.Severity {
		case "error":
			sev = "ERROR"
		case "note":
			sev = "NOTE "
		}
		loc := f.Discussion
		if f.Issue != "" {
			loc += " issue=" + f.Issue
		}
		if f.Node != "" {
			loc += " node=" + f.Node
		}
		fmt.Fprintf(&b, "  %s %-17s %s\n        %s\n", sev, f.Kind, loc, f.Detail)
	}
	if v.OK {
		b.WriteString("✓ check passed\n")
	} else {
		b.WriteString("✗ check failed\n")
	}
	return b.String()
}

// mapFindings verifies a standing value-map decision: map drift (the current
// audience map differs from the one derived as of the decision's transaction
// time), the review horizon, and the standing residue that the deferred
// governance question has not yet been raised.
func mapFindings(s *store.Store, g *ibis.Graph, disc, issue string, md *render.DecidedView, nowUnix int64) []Finding {
	var out []Finding
	cur, _, err := render.IssueMap(g, issue)
	if err == nil {
		if tx, ok, e := s.DecisionTxStart(disc, issue); e == nil && ok {
			if gAt, e := s.Graph(disc, store.When{Tx: &tx}); e == nil {
				if was, _, e := render.IssueMap(gAt, issue); e == nil && was != cur {
					out = append(out, Finding{
						Kind: MapDrift, Severity: "warning", Discussion: disc, Issue: issue,
						Detail: "the audience-conditional map has changed since the decision was recorded (a verdict flipped, an audience was superseded, or a robust winner emerged); re-affirm or convert to a position via `supersede`",
					})
				}
			}
		}
	}
	if md.ReviewBy != 0 && md.ReviewBy < nowUnix {
		out = append(out, Finding{
			Kind: ReviewDue, Severity: "warning", Discussion: disc, Issue: issue,
			Detail: fmt.Sprintf("value-map decision on %s is past its review horizon (%s); re-affirm or revise via supersede", issue, time.Unix(md.ReviewBy, 0).UTC().Format("2006-01-02")),
		})
	}
	if !hasGovernanceIssue(g, issue) {
		out = append(out, Finding{
			Kind: MappedPendingGovernance, Severity: "note", Discussion: disc, Issue: issue,
			Detail: "this issue is closed as a value-map but the governance question it defers (\"whose ranking should govern?\") has not been raised as its own issue — raise it with `raise \"...\" --from <this issue or a position>`",
		})
	}
	return out
}

// hasGovernanceIssue reports whether some issue was raised (raise --from) from
// the mapped issue or one of its positions — the honest follow-up to a value
// map, naming the deferred value conflict as its own question.
func hasGovernanceIssue(g *ibis.Graph, issue string) bool {
	from := map[string]bool{issue: true}
	for _, l := range g.Links {
		if l.Rel == ibis.RespondsTo && l.Dst == issue {
			from[l.Src] = true
		}
	}
	for _, l := range g.Links {
		if l.Rel == ibis.RaisedFrom && from[l.Dst] {
			if n, ok := g.Nodes[l.Src]; ok && n.Kind == ibis.Issue {
				return true
			}
		}
	}
	return false
}

func sortedKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
