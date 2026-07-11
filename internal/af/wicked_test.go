package af

import (
	"reflect"
	"sort"
	"testing"

	"github.com/chazu/dlktk/internal/ibis"
)

// --- BuildUnder (value-based audience lens) ---

func vafGraph(prefs []ibis.Preference, values []ibis.ValueTag) *ibis.Graph {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which?"},
		{ID: "a", Kind: ibis.Position, Text: "fast"},
		{ID: "b", Kind: ibis.Position, Text: "safe"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "a", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "b", Dst: "I", Rel: ibis.RespondsTo},
	}
	return ibis.NewGraph(nodes, links, prefs, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, values, nil)
}

var secFirst = ibis.Audience{Name: "ops", Ranking: []string{"security", "velocity"}}

// A strict value ranking resolves a symmetric select_one rivalry without any
// pairwise prefer: the attack on the higher-ranked value fails, the reverse
// survives.
func TestBuildUnderResolvesTieByValue(t *testing.T) {
	g := vafGraph(nil, []ibis.ValueTag{
		{Node: "a", Value: "velocity"}, {Node: "b", Value: "security"},
	})
	fw, err := BuildUnder(g, secFirst)
	if err != nil {
		t.Fatal(err)
	}
	labels := fw.Grounded()
	if labels["b"] != IN || labels["a"] != OUT {
		t.Fatalf("want b IN / a OUT under security-first, got %v", labels)
	}
	if fw.AudienceBlocked[[2]string{"a", "b"}] == "" {
		t.Fatalf("audience-blocked attack not recorded: %v", fw.AudienceBlocked)
	}
}

// Unvalued nodes are untouched by the lens: the rivalry stays a mutual
// stalemate.
func TestBuildUnderIgnoresUnvaluedNodes(t *testing.T) {
	g := vafGraph(nil, []ibis.ValueTag{{Node: "b", Value: "security"}})
	fw, err := BuildUnder(g, secFirst)
	if err != nil {
		t.Fatal(err)
	}
	labels := fw.Grounded()
	if labels["a"] != UNDEC || labels["b"] != UNDEC {
		t.Fatalf("unvalued rivalry must stay UNDEC, got %v", labels)
	}
}

// The composed-filter soundness hole (review finding 1): `prefer a b` plus an
// audience ranking b's value higher must NOT neutralize both directions of the
// rivalry. The pairwise preference takes the pair; exactly one rival is IN.
func TestBuildUnderPairwisePreferenceTakesThePair(t *testing.T) {
	g := vafGraph(
		[]ibis.Preference{{ID: "p1", Winner: "a", Loser: "b", Basis: "benchmarks"}},
		[]ibis.ValueTag{{Node: "a", Value: "velocity"}, {Node: "b", Value: "security"}},
	)
	fw, err := BuildUnder(g, secFirst)
	if err != nil {
		t.Fatal(err)
	}
	labels := fw.Grounded()
	if labels["a"] != IN || labels["b"] != OUT {
		t.Fatalf("pairwise prefer must decide the pair (a IN / b OUT), got %v", labels)
	}
	in := 0
	for _, l := range []Label{labels["a"], labels["b"]} {
		if l == IN {
			in++
		}
	}
	if in != 1 {
		t.Fatalf("select_one rivals simultaneously IN — the Q2 collapse: %v", labels)
	}
}

// --- PreferredExtensions ---

func fwOf(args []string, defeat []Edge) *Framework {
	return &Framework{Args: args, Attack: defeat, Defeat: defeat}
}

func sortedExts(t *testing.T, f *Framework) [][]string {
	t.Helper()
	exts, tooContested := f.PreferredExtensions()
	if tooContested {
		t.Fatal("unexpectedly too contested")
	}
	return exts
}

// An odd cycle admits only the empty stance: preferred = grounded = {}.
func TestPreferredOddCycle(t *testing.T) {
	f := fwOf([]string{"a", "b", "c"}, []Edge{{"a", "b"}, {"b", "c"}, {"c", "a"}})
	exts := sortedExts(t, f)
	if len(exts) != 1 || len(exts[0]) != 0 {
		t.Fatalf("odd cycle: want one empty extension, got %v", exts)
	}
}

// A mutual pair admits exactly the two one-sided stances.
func TestPreferredMutualPair(t *testing.T) {
	f := fwOf([]string{"a", "b"}, []Edge{{"a", "b"}, {"b", "a"}})
	exts := sortedExts(t, f)
	want := [][]string{{"a"}, {"b"}}
	if !reflect.DeepEqual(exts, want) {
		t.Fatalf("mutual pair: want %v, got %v", want, exts)
	}
}

// The even 4-cycle exercises defense *inside* the residue: {a,c} and {b,d}
// each defend their members against attackers that only the set itself
// defeats.
func TestPreferredEvenFourCycle(t *testing.T) {
	f := fwOf([]string{"a", "b", "c", "d"},
		[]Edge{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "a"}})
	exts := sortedExts(t, f)
	want := [][]string{{"a", "c"}, {"b", "d"}}
	if !reflect.DeepEqual(exts, want) {
		t.Fatalf("4-cycle: want %v, got %v", want, exts)
	}
}

// Preferred extensions can differ in size; keeping only maximum-cardinality
// sets would wrongly drop the smaller stance (inclusion-maximality
// regression).
func TestPreferredExtensionsDifferInSize(t *testing.T) {
	// a<->b mutual; a attacks c; b attacks d and e.
	f := fwOf([]string{"a", "b", "c", "d", "e"},
		[]Edge{{"a", "b"}, {"b", "a"}, {"a", "c"}, {"b", "d"}, {"b", "e"}})
	exts := sortedExts(t, f)
	want := [][]string{{"a", "d", "e"}, {"b", "c"}}
	if !reflect.DeepEqual(exts, want) {
		t.Fatalf("want %v, got %v", want, exts)
	}
}

// Enumeration must run over Defeat, not raw Attack: a preference-neutralized
// attack inside the residue is not a conflict. Three select_one rivals with
// prefer(a,b): the surviving defeats leave exactly the stances {a} and {c}.
func TestPreferredUsesDefeatNotAttack(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which?"},
		{ID: "a", Kind: ibis.Position, Text: "a"},
		{ID: "b", Kind: ibis.Position, Text: "b"},
		{ID: "c", Kind: ibis.Position, Text: "c"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "a", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "b", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l3", Src: "c", Dst: "I", Rel: ibis.RespondsTo},
	}
	prefs := []ibis.Preference{{ID: "p1", Winner: "a", Loser: "b", Basis: "x"}}
	g := ibis.NewGraph(nodes, links, prefs, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
	fw, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	exts, tooContested := fw.PreferredExtensions()
	if tooContested {
		t.Fatal("unexpectedly too contested")
	}
	want := [][]string{{"a"}, {"c"}}
	if !reflect.DeepEqual(exts, want) {
		t.Fatalf("want %v, got %v", want, exts)
	}
}

// Independent components cross-combine; each component contributes its own
// maximal stances.
func TestPreferredCrossCombinesComponents(t *testing.T) {
	f := fwOf([]string{"a", "b", "x", "y"},
		[]Edge{{"a", "b"}, {"b", "a"}, {"x", "y"}, {"y", "x"}})
	exts := sortedExts(t, f)
	if len(exts) != 4 {
		t.Fatalf("two mutual pairs: want 4 worlds, got %v", exts)
	}
	for _, ext := range exts {
		if len(ext) != 2 {
			t.Fatalf("each world picks one per pair: %v", exts)
		}
	}
}

// A residue component past the guard reports too-contested instead of
// stalling.
func TestPreferredTooContestedGuard(t *testing.T) {
	n := WorldsMaxComponent + 1
	args := make([]string, n)
	var defeat []Edge
	for i := range args {
		args[i] = string(rune('a')) + string(rune('0'+i/10)) + string(rune('0'+i%10))
	}
	for i := range args {
		defeat = append(defeat, Edge{args[i], args[(i+1)%n]})
	}
	f := fwOf(args, defeat)
	exts, tooContested := f.PreferredExtensions()
	if !tooContested || exts != nil {
		t.Fatalf("want too-contested guard to fire, got %v", exts)
	}
}

// Restrict keeps only in-scope args and edges.
func TestRestrict(t *testing.T) {
	f := fwOf([]string{"a", "b", "c"}, []Edge{{"a", "b"}, {"b", "c"}})
	sub := f.Restrict(map[string]bool{"a": true, "b": true})
	sort.Strings(sub.Args)
	if !reflect.DeepEqual(sub.Args, []string{"a", "b"}) {
		t.Fatalf("args wrong: %v", sub.Args)
	}
	if len(sub.Defeat) != 1 || sub.Defeat[0] != (Edge{"a", "b"}) {
		t.Fatalf("edges wrong: %v", sub.Defeat)
	}
}
