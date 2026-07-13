package render

// Arc two, item 1 (wicked-problems-2.md): tested-ness must ignore rival
// attacks and resist self-dealing. A position counts as tested only when an
// objection from another author participates in the defeat relation;
// select_one rival edges, self-objections, and preference-excused attacks
// never count.

import (
	"strings"
	"testing"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// hybridGraph reproduces the exercise's failure shape: rivals A and B, hybrid
// H synthesized from both, elevated over its parents by two subsumption
// preferences, never objected to. H is IN (reinstated) without one
// substantive attack.
func hybridGraph() (*ibis.Graph, *af.Framework, map[string]af.Label) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which approach?"},
		{ID: "A", Kind: ibis.Position, Text: "a", Author: "ann"},
		{ID: "B", Kind: ibis.Position, Text: "b", Author: "bob"},
		{ID: "H", Kind: ibis.Position, Text: "hybrid", Author: "syn"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l3", Src: "H", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l4", Src: "H", Dst: "A", Rel: ibis.Synthesizes},
		{ID: "l5", Src: "H", Dst: "B", Rel: ibis.Synthesizes},
	}
	prefs := []ibis.Preference{
		{ID: "p1", Winner: "H", Loser: "A", Basis: "subsumption"},
		{ID: "p2", Winner: "H", Loser: "B", Basis: "subsumption"},
	}
	g := ibis.NewGraph(nodes, links, prefs, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
	fw, err := af.Build(g)
	if err != nil {
		panic(err)
	}
	return g, fw, fw.Grounded()
}

// The exercise regression: a hybrid elevated by subsumption preferences is
// attacked by its rivals yet examined by nobody — it must read as untested on
// every surface (status flag, stress-test move before decide, agenda entry).
func TestRivalElevatedHybridIsUntested(t *testing.T) {
	g, fw, labels := hybridGraph()
	if labels["H"] != af.IN || labels["A"] != af.OUT || labels["B"] != af.OUT {
		t.Fatalf("fixture not elevated: %v", labels)
	}

	st := Status(g, fw, labels, "I", nil)
	for _, p := range st.Positions {
		if p.ID == "H" && (!p.Untested || !p.Reinstated) {
			t.Fatalf("elevated hybrid must be reinstated AND untested: %+v", p)
		}
	}

	mv := Moves(g, fw, labels, "I", nil)
	if mv.Moves[0].Move != "object" || mv.Moves[0].Args[0] != "H" {
		t.Fatalf("want stress-test object H before decide, got %+v", mv.Moves[0])
	}
	if mv.Moves[1].Move != "decide" {
		t.Fatalf("want decide second, got %+v", mv.Moves[1])
	}

	v := Agenda(g, fw, labels, nil)
	if len(v.Untested) != 1 || v.Untested[0].Position != "H" {
		t.Fatalf("agenda must name the untested winner: %+v", v.Untested)
	}
}

// A self-objection (same author as the position), later rebutted, must not
// clear the tested bar; the same objection from another author must.
func TestSelfObjectionDoesNotTest(t *testing.T) {
	build := func(objAuthor string) (*ibis.Graph, *af.Framework) {
		nodes := []ibis.Node{
			{ID: "I", Kind: ibis.Issue, Text: "q?"},
			{ID: "P", Kind: ibis.Position, Text: "p", Author: "alice"},
			{ID: "O1", Kind: ibis.Argument, Text: "strawman", Author: objAuthor},
			{ID: "O2", Kind: ibis.Argument, Text: "rebuttal", Author: "alice"},
		}
		links := []ibis.Link{
			{ID: "l1", Src: "P", Dst: "I", Rel: ibis.RespondsTo},
			{ID: "l2", Src: "O1", Dst: "P", Rel: ibis.ObjectsTo},
			{ID: "l3", Src: "O2", Dst: "O1", Rel: ibis.ObjectsTo},
		}
		g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
		fw, err := af.Build(g)
		if err != nil {
			panic(err)
		}
		return g, fw
	}

	g, fw := build("alice")
	if Tested(g, fw, "P") {
		t.Fatal("self-authored objection must not count as a test")
	}
	g, fw = build("bob")
	if !Tested(g, fw, "P") {
		t.Fatal("cross-author objection beaten by counter-argument must count as a test")
	}
}

// A preference-neutralized objection is a test the position was excused from:
// the edge leaves the defeat relation, so the position is untested again.
func TestPreferenceExcusedObjectionDoesNotTest(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "q?"},
		{ID: "P", Kind: ibis.Position, Text: "p", Author: "alice"},
		{ID: "O", Kind: ibis.Argument, Text: "real objection", Author: "bob"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "P", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "O", Dst: "P", Rel: ibis.ObjectsTo},
	}
	prefs := []ibis.Preference{{ID: "p1", Winner: "P", Loser: "O", Basis: "call"}}
	g := ibis.NewGraph(nodes, links, prefs, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
	fw, err := af.Build(g)
	if err != nil {
		panic(err)
	}
	if fw.Grounded()["P"] != af.IN {
		t.Fatalf("fixture: P must be IN via the preference: %v", fw.Grounded())
	}
	if Tested(g, fw, "P") {
		t.Fatal("preference-excused objection must not count as a test")
	}
}

// Mid-divergence, untested positions stay off the agenda: an open issue with
// two justified, unexamined positions is not decide-adjacent (not ready), so
// the untested section stays empty instead of flooding.
func TestAgendaUntestedOnlyWhenDecideAdjacent(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which practices?"},
		{ID: "A", Kind: ibis.Position, Text: "a", Author: "ann"},
		{ID: "B", Kind: ibis.Position, Text: "b", Author: "bob"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
	}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.Open}}, nil, nil, nil)
	fw, err := af.Build(g)
	if err != nil {
		panic(err)
	}
	labels := fw.Grounded()
	if labels["A"] != af.IN || labels["B"] != af.IN {
		t.Fatalf("fixture: open-issue positions must both be IN: %v", labels)
	}

	v := Agenda(g, fw, labels, nil)
	if len(v.Untested) != 0 {
		t.Fatalf("mid-divergence untested positions must not flood the agenda: %+v", v.Untested)
	}
}

// On a stalemate between untested rivals, the advice and the prefer
// suggestions state one ordering — object first, then prefer — instead of
// offering the preference as a co-equal exit.
func TestUntestedStalemateOrdersObjectionFirst(t *testing.T) {
	g, fw, labels := stalemateGraph()

	st := Status(g, fw, labels, "I", nil)
	if !strings.Contains(st.Advice, "object first, then prefer") {
		t.Fatalf("stalemate advice must order objection before preference: %q", st.Advice)
	}
	if !strings.Contains(st.Advice, "synthesis") || !strings.Contains(st.Advice, "reframe") {
		t.Fatalf("generative exits must survive the reordering: %q", st.Advice)
	}

	mv := Moves(g, fw, labels, "I", nil)
	for _, m := range mv.Moves {
		if m.Move == "prefer" && !strings.Contains(m.Effect, "object first, then prefer") {
			t.Fatalf("prefer suggestion on an untested target must be annotated: %+v", m)
		}
	}
}
