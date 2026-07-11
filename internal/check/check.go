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
	DecisionDrift      = "decision_drift"      // decided position no longer IN (error)
	PreferenceCycle    = "preference_cycle"    // stored preferences cyclic (error)
	StoreInvariant     = "store_invariant"     // e.g. duplicate current node ids (error)
	Stalemate          = "stalemate"           // all positions UNDEC (warning)
	UntestedDecision   = "untested_decision"   // decided position never attacked (warning)
	ReviewDue          = "review_due"          // decision's review horizon has passed (warning)
	DefeatedAssumption = "defeated_assumption" // decided position rests on an OUT assumption (warning)
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

	for _, issue := range g.Issues() {
		st := render.Status(g, fw, labels, issue, decs)
		if d := st.Decided; d != nil && !d.Override {
			// The position was IN when decided (no override flag); if it is no
			// longer IN, the dialectic has moved out from under the decision.
			if l := labels[d.Position]; l != af.IN {
				out = append(out, Finding{
					Kind: DecisionDrift, Severity: "error", Discussion: disc, Issue: issue, Node: d.Position,
					Detail: fmt.Sprintf("decided position %s is no longer justified (now %s); re-argue or supersede", d.Position, l),
				})
			}
		}
		if d := st.Decided; d != nil {
			// A decision that survived zero tests is the kind most likely to
			// rot: IN by silence, not by surviving attack.
			if labels[d.Position] == af.IN && len(attackersOf(fw, d.Position)) == 0 {
				out = append(out, Finding{
					Kind: UntestedDecision, Severity: "warning", Discussion: disc, Issue: issue, Node: d.Position,
					Detail: fmt.Sprintf("decided position %s was never attacked; its IN label is unexamined, not vindicated — stress-test it", d.Position),
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
		}
		if st.Stalemate && st.ReframedTo == "" {
			// A stalemate under a reframed (dead) framing is no longer the
			// live question; only current framings warrant the warning.
			out = append(out, Finding{
				Kind: Stalemate, Severity: "warning", Discussion: disc, Issue: issue,
				Detail: fmt.Sprintf("all %d positions UNDEC; a preference is needed to resolve", len(st.Positions)),
			})
		}
	}
	return out, nil
}

// attackersOf lists the distinct attackers of a node in the raw attack relation.
func attackersOf(fw *af.Framework, node string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range fw.Attack {
		if e.To == node && !seen[e.From] {
			seen[e.From] = true
			out = append(out, e.From)
		}
	}
	return out
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
		if f.Severity == "error" {
			sev = "ERROR"
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

func sortedKeys(m map[string]int) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
