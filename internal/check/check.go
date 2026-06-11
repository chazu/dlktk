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

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/render"
	"github.com/chazu/dlktk/internal/store"
)

// Finding kinds.
const (
	DecisionDrift   = "decision_drift"   // decided position no longer IN (error)
	PreferenceCycle = "preference_cycle" // stored preferences cyclic (error)
	StoreInvariant  = "store_invariant"  // e.g. duplicate current node ids (error)
	Stalemate       = "stalemate"        // all positions UNDEC (warning)
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

// Run checks the given discussions at the given temporal viewpoint. Output is
// deterministic: discussions and issues are visited in sorted order.
func Run(s *store.Store, discs []string, w store.When) (View, error) {
	v := View{Discussions: len(discs)}
	sorted := append([]string{}, discs...)
	sort.Strings(sorted)
	for _, disc := range sorted {
		fs, err := runOne(s, disc, w)
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

func runOne(s *store.Store, disc string, w store.When) ([]Finding, error) {
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
		if st.Stalemate {
			out = append(out, Finding{
				Kind: Stalemate, Severity: "warning", Discussion: disc, Issue: issue,
				Detail: fmt.Sprintf("all %d positions UNDEC; a preference is needed to resolve", len(st.Positions)),
			})
		}
	}
	return out, nil
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
