// Package af builds a Dung argumentation framework from an IBIS graph and
// computes the grounded labelling. It is pure: given a graph it derives attack,
// transitive preference, defeat, and the grounded fixpoint. No storage, no CLI.
package af

import "github.com/chazu/dlktk/internal/ibis"

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

// Build derives the AF from an IBIS graph.
func Build(g *ibis.Graph) *Framework {
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

	// Preference, transitively closed.
	f.Preferred = closure(g.Preferences)

	// Defeat = attack surviving preference: attack(a,b) unless preferred(b,a).
	for _, e := range f.Attack {
		if !f.Preferred[[2]string{e.To, e.From}] {
			f.Defeat = append(f.Defeat, e)
		}
	}

	return f
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
