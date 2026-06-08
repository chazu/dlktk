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

	g := ibis.NewGraph(nodes, links, nil, cards)
	labels := Build(g).Grounded()

	assert(t, labels, "D", IN)
	assert(t, labels, "C", OUT)
	assert(t, labels, "A", UNDEC)
	assert(t, labels, "B", UNDEC)

	// Add prefer(B, A): B's attacker A is now blocked, B reinstated.
	prefs := []ibis.Preference{{Winner: "B", Loser: "A"}}
	g = ibis.NewGraph(nodes, links, prefs, cards)
	labels = Build(g).Grounded()

	assert(t, labels, "B", IN)
	assert(t, labels, "A", OUT)
	assert(t, labels, "D", IN)
	assert(t, labels, "C", OUT)
}

func assert(t *testing.T, labels map[string]Label, node string, want Label) {
	t.Helper()
	if got := labels[node]; got != want {
		t.Errorf("label[%s] = %s, want %s", node, got, want)
	}
}
