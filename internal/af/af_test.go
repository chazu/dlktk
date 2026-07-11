package af

import "github.com/chazu/dlktk/internal/ibis"

import "testing"

// Worked example from design §2.3 (reinstatement):
//
//	issue I (select_one), positions A (mutex) and B (RWLock),
//	argument C objects_to B, argument D objects_to C.
//	D unattacked -> IN; C -> OUT; B then attacked only by A (select-one);
//	A,B mutually attack with no preference -> both UNDEC.
//	Adding prefer(B,A) -> B IN, A OUT.
func TestReinstatementAndPreference(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue},
		{ID: "A", Kind: ibis.Position},
		{ID: "B", Kind: ibis.Position},
		{ID: "C", Kind: ibis.Argument},
		{ID: "D", Kind: ibis.Argument},
	}
	links := []ibis.Link{
		{Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{Src: "B", Dst: "I", Rel: ibis.RespondsTo},
		{Src: "C", Dst: "B", Rel: ibis.ObjectsTo},
		{Src: "D", Dst: "C", Rel: ibis.ObjectsTo},
	}
	cards := []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}

	g := ibis.NewGraph(nodes, links, nil, cards, nil, nil, nil)
	labels := mustBuild(t, g).Grounded()

	assert(t, labels, "D", IN)
	assert(t, labels, "C", OUT)
	assert(t, labels, "A", UNDEC)
	assert(t, labels, "B", UNDEC)

	// Add prefer(B, A): B's attacker A is now blocked, B reinstated.
	prefs := []ibis.Preference{{Winner: "B", Loser: "A"}}
	g = ibis.NewGraph(nodes, links, prefs, cards, nil, nil, nil)
	labels = mustBuild(t, g).Grounded()

	assert(t, labels, "B", IN)
	assert(t, labels, "A", OUT)
	assert(t, labels, "D", IN)
	assert(t, labels, "C", OUT)
}

// A cyclic preference relation (only reachable by bypassing assert-time
// legality, e.g. a corrupted store) must fail loud, never label two mutually
// exclusive positions both IN via all-prefer-all closure collapse.
func TestPreferenceCycleFailsLoud(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue},
		{ID: "A", Kind: ibis.Position},
		{ID: "B", Kind: ibis.Position},
	}
	links := []ibis.Link{
		{Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{Src: "B", Dst: "I", Rel: ibis.RespondsTo},
	}
	cards := []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}
	prefs := []ibis.Preference{
		{Winner: "A", Loser: "B"},
		{Winner: "B", Loser: "A"},
	}

	g := ibis.NewGraph(nodes, links, prefs, cards, nil, nil, nil)
	_, err := Build(g)
	var cyc *PreferenceCycleError
	if err == nil {
		t.Fatal("Build accepted a cyclic preference relation")
	}
	if !asCycle(err, &cyc) {
		t.Fatalf("want PreferenceCycleError, got %v", err)
	}
	if node, ok := PreferenceCycle(prefs); !ok || node != "A" {
		t.Fatalf("PreferenceCycle = (%q, %v), want (A, true)", node, ok)
	}
	if _, ok := PreferenceCycle([]ibis.Preference{{Winner: "A", Loser: "B"}}); ok {
		t.Fatal("PreferenceCycle flagged an acyclic relation")
	}
}

func asCycle(err error, target **PreferenceCycleError) bool {
	c, ok := err.(*PreferenceCycleError)
	if ok {
		*target = c
	}
	return ok
}

func mustBuild(t *testing.T, g *ibis.Graph) *Framework {
	t.Helper()
	fw, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	return fw
}

func assert(t *testing.T, labels map[string]Label, node string, want Label) {
	t.Helper()
	if got := labels[node]; got != want {
		t.Errorf("label[%s] = %s, want %s", node, got, want)
	}
}
