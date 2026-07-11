// Package af builds a Dung argumentation framework from an IBIS graph and
// computes the grounded labelling. It is pure: given a graph it derives attack,
// transitive preference, defeat, and the grounded fixpoint. No storage, no CLI.
package af

import (
	"fmt"
	"sort"
	"strings"

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
	// AudienceBlocked records attacks neutralized by the audience lens
	// ([attacker,target] -> "value(target)≻value(attacker)"), nil for a plain
	// Build. For explanation only.
	AudienceBlocked map[[2]string]string
}

// Build derives the AF from an IBIS graph. It fails with PreferenceCycleError
// if the stored preference relation is cyclic.
func Build(g *ibis.Graph) (*Framework, error) {
	return build(g, nil)
}

// BuildUnder derives the AF as seen by an audience: attacks additionally fail
// against a strictly higher-ranked value (Bench-Capon's value-based AF).
//
// Cross-mechanism precedence: for any pair that carries a pairwise
// (transitively closed) preference in either direction, the pairwise relation
// alone decides survival and the audience is ignored for that pair — a
// recorded dialectical move outranks the systematic lens. Composing the two
// filters independently would be unsound: `prefer a b` plus an audience
// ranking b's value above a's would neutralize both directions of a symmetric
// select_one rivalry, labelling both rivals IN (the Q2 collapse). Per-pair
// precedence restores antisymmetry: the pairwise relation is antisymmetric by
// cycle rejection, and audience-only is antisymmetric because a strict value
// order blocks at most one direction.
func BuildUnder(g *ibis.Graph, aud ibis.Audience) (*Framework, error) {
	return build(g, &aud)
}

func build(g *ibis.Graph, aud *ibis.Audience) (*Framework, error) {
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
	// Under an audience, a preference-free pair is additionally filtered by
	// value rank (see BuildUnder for why the pairwise relation takes the pair).
	var rank map[string]int
	if aud != nil {
		f.AudienceBlocked = map[[2]string]string{}
		rank = make(map[string]int, len(aud.Ranking))
		for i, v := range aud.Ranking {
			rank[v] = i // lower index = more important
		}
	}
	for _, e := range f.Attack {
		pair := [2]string{e.From, e.To}
		if f.Preferred[pair] || f.Preferred[[2]string{e.To, e.From}] {
			// Pairwise preference exists on this pair: it alone decides.
			if !f.Preferred[[2]string{e.To, e.From}] {
				f.Defeat = append(f.Defeat, e)
			}
			continue
		}
		if aud != nil {
			vFrom, okFrom := g.Values[e.From]
			vTo, okTo := g.Values[e.To]
			if okFrom && okTo {
				rFrom, knownFrom := rank[vFrom]
				rTo, knownTo := rank[vTo]
				if knownFrom && knownTo && rTo < rFrom {
					f.AudienceBlocked[pair] = fmt.Sprintf("%s≻%s", vTo, vFrom)
					continue // the target's value outranks the attacker's: attack fails
				}
			}
		}
		f.Defeat = append(f.Defeat, e)
	}

	return f, nil
}

// Restrict returns the sub-framework induced by the given node set (used to
// scope worlds/explanations to one issue's reachable arguments). Labels for
// in-scope nodes are preserved when the scope is upward-closed under attack —
// which reachableAF scopes are by construction.
func (f *Framework) Restrict(scope map[string]bool) *Framework {
	sub := &Framework{Preferred: f.Preferred, AudienceBlocked: f.AudienceBlocked}
	for _, a := range f.Args {
		if scope[a] {
			sub.Args = append(sub.Args, a)
		}
	}
	for _, e := range f.Attack {
		if scope[e.From] && scope[e.To] {
			sub.Attack = append(sub.Attack, e)
		}
	}
	for _, e := range f.Defeat {
		if scope[e.From] && scope[e.To] {
			sub.Defeat = append(sub.Defeat, e)
		}
	}
	return sub
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

// WorldsMaxComponent bounds the UNDEC-residue component size PreferredExtensions
// will enumerate. Beyond it the framework is reported too contested rather than
// stalling the CLI (enumeration is exponential in the component size).
const WorldsMaxComponent = 20

// PreferredExtensions enumerates the preferred extensions (maximal admissible
// sets) of the framework, over the defeat relation — the same relation the
// grounded labelling runs on; raw attacks neutralized by preference must not
// manufacture conflicts here.
//
// Soundness of the residue-only enumeration: every preferred extension is
// complete, hence contains the grounded extension G, and cannot contain a
// grounded-OUT node (it is defeated by a member of G). So E = G ∪ S with S a
// subset of the UNDEC residue, and E is admissible iff G∪S is conflict-free
// and every s∈S is acceptable w.r.t. G∪S (acceptability is monotone in the
// defending set, and every grounded-OUT attacker of S is already countered by
// G — so the check reduces to: each UNDEC defeater of S is counter-defeated by
// S). Inclusion-maximal such sets are exactly the preferred extensions.
//
// The residue decomposes into defeat-connected components with no cross-
// component defeats, so admissibility is component-local and the preferred
// extensions are the cross-product of per-component maximal admissible sets.
// A component larger than WorldsMaxComponent aborts with tooContested=true
// (nil extensions).
func (f *Framework) PreferredExtensions() (exts [][]string, tooContested bool) {
	labels := f.Grounded()
	var grounded, residue []string
	for _, a := range f.Args {
		switch labels[a] {
		case IN:
			grounded = append(grounded, a)
		case UNDEC:
			residue = append(residue, a)
		}
	}
	sort.Strings(residue)

	inResidue := map[string]bool{}
	for _, u := range residue {
		inResidue[u] = true
	}
	// Defeat edges within the residue (both directions indexed).
	defeats := map[string][]string{}   // defeater -> targets
	defeaters := map[string][]string{} // target -> defeaters
	for _, d := range f.Defeat {
		if inResidue[d.From] && inResidue[d.To] {
			defeats[d.From] = append(defeats[d.From], d.To)
			defeaters[d.To] = append(defeaters[d.To], d.From)
		}
	}

	// Undirected connected components of the residue.
	comp := map[string]int{}
	var comps [][]string
	for _, u := range residue {
		if _, seen := comp[u]; seen {
			continue
		}
		idx := len(comps)
		var members []string
		stack := []string{u}
		comp[u] = idx
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			members = append(members, cur)
			for _, nxt := range append(append([]string{}, defeats[cur]...), defeaters[cur]...) {
				if _, seen := comp[nxt]; !seen {
					comp[nxt] = idx
					stack = append(stack, nxt)
				}
			}
		}
		sort.Strings(members)
		comps = append(comps, members)
	}

	// Per component: enumerate conflict-free subsets (with pruning), filter to
	// admissible, keep inclusion-maximal.
	perComp := make([][][]string, len(comps))
	for ci, members := range comps {
		if len(members) > WorldsMaxComponent {
			return nil, true
		}
		conflict := func(s map[string]bool, cand string) bool {
			for _, t := range defeats[cand] {
				if s[t] || t == cand {
					return true
				}
			}
			for _, d := range defeaters[cand] {
				if s[d] {
					return true
				}
			}
			return false
		}
		var admissible [][]string
		var rec func(i int, cur map[string]bool)
		rec = func(i int, cur map[string]bool) {
			if i == len(members) {
				// Acceptability: every UNDEC defeater of a member is itself
				// defeated by a member.
				for m := range cur {
					for _, d := range defeaters[m] {
						countered := false
						for _, dd := range defeaters[d] {
							if cur[dd] {
								countered = true
								break
							}
						}
						if !countered {
							return
						}
					}
				}
				set := make([]string, 0, len(cur))
				for m := range cur {
					set = append(set, m)
				}
				sort.Strings(set)
				admissible = append(admissible, set)
				return
			}
			rec(i+1, cur) // exclude members[i]
			if !conflict(cur, members[i]) {
				cur[members[i]] = true
				rec(i+1, cur)
				delete(cur, members[i])
			}
		}
		rec(0, map[string]bool{})
		perComp[ci] = maximalSets(admissible)
	}

	// Cross-combine components, prepending the grounded extension to each.
	combos := [][]string{{}}
	for _, sets := range perComp {
		var next [][]string
		for _, base := range combos {
			for _, s := range sets {
				merged := append(append([]string{}, base...), s...)
				next = append(next, merged)
			}
		}
		combos = next
	}
	for _, c := range combos {
		ext := append(append([]string{}, grounded...), c...)
		sort.Strings(ext)
		exts = append(exts, ext)
	}
	sort.Slice(exts, func(i, j int) bool {
		a, b := exts[i], exts[j]
		for k := 0; k < len(a) && k < len(b); k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return len(a) < len(b)
	})
	return exts, false
}

// maximalSets keeps the ⊆-maximal sets (not maximum-cardinality — preferred
// extensions can differ in size).
func maximalSets(sets [][]string) [][]string {
	var out [][]string
	for i, s := range sets {
		maximal := true
		for j, t := range sets {
			if i != j && subset(s, t) && len(s) < len(t) {
				maximal = false
				break
			}
		}
		if maximal {
			out = append(out, s)
		}
	}
	// Dedup (identical sets can be enumerated once each; keep one).
	seen := map[string]bool{}
	var dedup [][]string
	for _, s := range out {
		k := strings.Join(s, "\x00")
		if !seen[k] {
			seen[k] = true
			dedup = append(dedup, s)
		}
	}
	return dedup
}

// subset reports a ⊆ b for sorted string slices.
func subset(a, b []string) bool {
	i := 0
	for _, x := range a {
		for i < len(b) && b[i] < x {
			i++
		}
		if i == len(b) || b[i] != x {
			return false
		}
		i++
	}
	return true
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
