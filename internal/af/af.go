// Package af builds a Dung argumentation framework from an IBIS graph and
// computes the grounded labelling. It is pure: given a graph it derives attack,
// transitive preference, defeat, and the grounded fixpoint. No storage, no CLI.
package af

import (
	"fmt"
	"sort"

	"github.com/chazu/dlktk/internal/ibis"
)

// PreferenceCycleError reports a cyclic preference relation in stored data.
// CanPrefer rejects cycles at assert time and import validates batches, so a
// cycle reaching evaluation is a store-invariant violation: naive closure over
// it would collapse to all-prefer-all and label mutually exclusive positions
// simultaneously IN. Fail loud rather than compute nonsense (design §3.1 ethos).
type PreferenceCycleError struct{ Node string }

func (e *PreferenceCycleError) Error() string {
	return fmt.Sprintf("preference cycle through %s: stored preferences must be acyclic (store invariant violated)", e.Node)
}

// Label is the three-valued grounded label.
type Label string

const (
	IN    Label = "IN"    // currently justified
	OUT   Label = "OUT"   // defeated
	UNDEC Label = "UNDEC" // genuinely contested
)

// Edge is a directed (attacker -> target) pair.
type Edge struct{ From, To string }

// Framework is the materialized AF: the argument set plus the defeat relation,
// retaining the raw attack relation and preference closure for explanation.
type Framework struct {
	Args      []string
	Attack    []Edge
	Defeat    []Edge
	Preferred map[[2]string]bool // [winner,loser] -> true (transitive)
}

// Build derives the AF from an IBIS graph. It fails with PreferenceCycleError
// if the stored preference relation is cyclic.
func Build(g *ibis.Graph) (*Framework, error) {
	f := &Framework{Preferred: map[[2]string]bool{}}

	// Arg set: positions and arguments.
	for id := range g.Nodes {
		if g.IsAFNode(id) {
			f.Args = append(f.Args, id)
		}
	}

	// Attack source 1: explicit objections.
	for _, l := range g.Links {
		if l.Rel == ibis.ObjectsTo && g.IsAFNode(l.Src) && g.IsAFNode(l.Dst) {
			f.Attack = append(f.Attack, Edge{l.Src, l.Dst})
		}
	}

	// Attack source 2: select_one positions on the same issue mutually attack.
	positionsByIssue := map[string][]string{}
	for _, l := range g.Links {
		if l.Rel != ibis.RespondsTo {
			continue
		}
		if n, ok := g.Nodes[l.Src]; ok && n.Kind == ibis.Position {
			positionsByIssue[l.Dst] = append(positionsByIssue[l.Dst], l.Src)
		}
	}
	for issue, ps := range positionsByIssue {
		if g.IssueCards[issue] != ibis.SelectOne {
			continue
		}
		for i := range ps {
			for j := range ps {
				if i != j {
					f.Attack = append(f.Attack, Edge{ps[i], ps[j]})
				}
			}
		}
	}

	// Preference, transitively closed; a cycle voids the closure's meaning.
	f.Preferred = closure(g.Preferences)
	if n, cyclic := cycleNode(f.Preferred); cyclic {
		return nil, &PreferenceCycleError{Node: n}
	}

	// Defeat = attack surviving preference: attack(a,b) unless preferred(b,a).
	for _, e := range f.Attack {
		if !f.Preferred[[2]string{e.To, e.From}] {
			f.Defeat = append(f.Defeat, e)
		}
	}

	return f, nil
}

// PreferenceCycle reports a node sitting on a preference cycle, if any. Used by
// import validation to reject batches that would corrupt the store.
func PreferenceCycle(prefs []ibis.Preference) (string, bool) {
	return cycleNode(closure(prefs))
}

// cycleNode finds the smallest node that the transitive closure makes preferred
// over itself (smallest for deterministic reporting).
func cycleNode(closed map[[2]string]bool) (string, bool) {
	var hits []string
	for pair, ok := range closed {
		if ok && pair[0] == pair[1] {
			hits = append(hits, pair[0])
		}
	}
	if len(hits) == 0 {
		return "", false
	}
	sort.Strings(hits)
	return hits[0], true
}

// Grounded computes the grounded labelling over the defeat relation. The
// fixpoint is monotone and the grounded extension is unique, so output is
// deterministic regardless of iteration order.
func (f *Framework) Grounded() map[string]Label {
	attackers := map[string][]string{}
	for _, d := range f.Defeat {
		attackers[d.To] = append(attackers[d.To], d.From)
	}

	label := make(map[string]Label, len(f.Args))
	for _, a := range f.Args {
		label[a] = UNDEC
	}

	for changed := true; changed; {
		changed = false
		for _, a := range f.Args {
			if label[a] != UNDEC {
				continue
			}
			allOut, anyIn := true, false
			for _, b := range attackers[a] {
				switch label[b] {
				case IN:
					anyIn = true
					allOut = false
				case UNDEC:
					allOut = false
				}
			}
			switch {
			case allOut: // includes unattacked (vacuously IN)
				label[a] = IN
				changed = true
			case anyIn:
				label[a] = OUT
				changed = true
			}
		}
	}
	return label
}

// Step records one node's grounded labelling, with the round it was decided in
// and why. Rounds are layered (BFS over the defeat relation): every node decided
// in round N used only labels settled by round N-1, which makes the derivation
// legible. The final labelling equals Grounded()'s (the grounded extension is
// unique), only the assignment order is made explicit.
type Step struct {
	Round int      `json:"round"`
	Node  string   `json:"node"`
	Label Label    `json:"label"`
	Why   string   `json:"why"` // unattacked | reinstated | defeated | contested
	By    []string `json:"by"`  // relevant defeaters (IN for defeated, OUT for reinstated, UNDEC for contested)
}

// GroundedSteps computes the grounded labelling and a layered trace of how each
// node got its label. Use it to explain the automated reasoning; use Grounded()
// when only the result is needed.
func (f *Framework) GroundedSteps() ([]Step, map[string]Label) {
	attackers := map[string][]string{}
	for _, d := range f.Defeat {
		attackers[d.To] = append(attackers[d.To], d.From)
	}
	args := append([]string{}, f.Args...)
	sort.Strings(args)

	label := make(map[string]Label, len(args))
	for _, a := range args {
		label[a] = UNDEC
	}

	var steps []Step
	round := 0
	for {
		round++
		var assigned []Step
		for _, a := range args {
			if label[a] != UNDEC {
				continue
			}
			allOut, anyIn := true, false
			var inBy, outBy []string
			for _, b := range attackers[a] {
				switch label[b] {
				case IN:
					anyIn, allOut = true, false
					inBy = append(inBy, b)
				case UNDEC:
					allOut = false
				case OUT:
					outBy = append(outBy, b)
				}
			}
			switch {
			case allOut:
				why := "unattacked"
				if len(attackers[a]) > 0 {
					why = "reinstated"
				}
				assigned = append(assigned, Step{Round: round, Node: a, Label: IN, Why: why, By: outBy})
			case anyIn:
				assigned = append(assigned, Step{Round: round, Node: a, Label: OUT, Why: "defeated", By: inBy})
			}
		}
		if len(assigned) == 0 {
			break
		}
		for _, s := range assigned {
			label[s.Node] = s.Label
			steps = append(steps, s)
		}
	}
	// Whatever is still UNDEC sits in an unresolved attack cycle / mutual stalemate.
	for _, a := range args {
		if label[a] == UNDEC {
			var by []string
			for _, b := range attackers[a] {
				if label[b] == UNDEC {
					by = append(by, b)
				}
			}
			steps = append(steps, Step{Round: round, Node: a, Label: UNDEC, Why: "contested", By: by})
		}
	}
	return steps, label
}

// closure returns the reflexive-free transitive closure of the preference
// relation as a set of [winner,loser] pairs.
func closure(prefs []ibis.Preference) map[[2]string]bool {
	adj := map[string][]string{}
	nodes := map[string]bool{}
	for _, p := range prefs {
		adj[p.Winner] = append(adj[p.Winner], p.Loser)
		nodes[p.Winner] = true
		nodes[p.Loser] = true
	}
	out := map[[2]string]bool{}
	for start := range nodes {
		seen := map[string]bool{}
		stack := append([]string{}, adj[start]...)
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[cur] {
				continue
			}
			seen[cur] = true
			out[[2]string{start, cur}] = true
			stack = append(stack, adj[cur]...)
		}
	}
	return out
}
