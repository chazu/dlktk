package render

// Arc two, items 2-3 (wicked-problems-2.md): a synthesis inherits its
// parents' undefeated objections as open questions (transitively), discharged
// on the record via addresses links; preferring a hybrid over a parent whose
// questions are open is the subsumption dodge.

import (
	"strings"
	"testing"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// chainGraph: H2 synthesizes {H1, C}; H1 synthesizes {A, B}; A carries a live
// cross-author objection OBJ. Over the transitive closure, OBJ is H2's
// inherited question — chaining must not launder it.
func chainGraph() (*ibis.Graph, *af.Framework, map[string]af.Label) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "q?"},
		{ID: "A", Kind: ibis.Position, Text: "a", Author: "ann"},
		{ID: "B", Kind: ibis.Position, Text: "b", Author: "bob"},
		{ID: "C", Kind: ibis.Position, Text: "c", Author: "cal"},
		{ID: "H1", Kind: ibis.Position, Text: "h1", Author: "syn"},
		{ID: "H2", Kind: ibis.Position, Text: "h2", Author: "syn"},
		{ID: "OBJ", Kind: ibis.Argument, Text: "a is costly", Author: "bob"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l3", Src: "C", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l4", Src: "H1", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l5", Src: "H2", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l6", Src: "H1", Dst: "A", Rel: ibis.Synthesizes},
		{ID: "l7", Src: "H1", Dst: "B", Rel: ibis.Synthesizes},
		{ID: "l8", Src: "H2", Dst: "H1", Rel: ibis.Synthesizes},
		{ID: "l9", Src: "H2", Dst: "C", Rel: ibis.Synthesizes},
		{ID: "l10", Src: "OBJ", Dst: "A", Rel: ibis.ObjectsTo},
	}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.Open}}, nil, nil, nil)
	fw, err := af.Build(g)
	if err != nil {
		panic(err)
	}
	return g, fw, fw.Grounded()
}

func TestInheritedQuestionsTransitiveClosure(t *testing.T) {
	g, _, labels := chainGraph()
	qs := InheritedQuestions(g, labels, "H2")
	if len(qs) != 1 || qs[0].Objection != "OBJ" || qs[0].Parent != "A" {
		t.Fatalf("H2 must inherit A's objection through the chain: %+v", qs)
	}
	if !qs[0].Open {
		t.Fatalf("unaddressed inherited question must be open: %+v", qs[0])
	}
	// A non-synthesis has no inherited questions.
	if got := InheritedQuestions(g, labels, "A"); got != nil {
		t.Fatalf("a plain position has no inherited questions: %+v", got)
	}
}

// An OUT parent objection (beaten on the parent by a counter-argument) is not
// inherited: it was answered where it lived.
func TestInheritedQuestionsSkipDefeatedParentObjections(t *testing.T) {
	g, _, _ := chainGraph()
	// Defeat OBJ on the parent: a cross-author rebuttal.
	g.Nodes["REB"] = ibis.Node{ID: "REB", Kind: ibis.Argument, Text: "not costly", Author: "ann"}
	g.Links = append(g.Links, ibis.Link{ID: "l11", Src: "REB", Dst: "OBJ", Rel: ibis.ObjectsTo})
	fw, _ := af.Build(g)
	if qs := InheritedQuestions(g, fw.Grounded(), "H2"); len(qs) != 0 {
		t.Fatalf("a defeated parent objection must not be inherited: %+v", qs)
	}
}

// An addresses link discharges the question: it stops being open, and records
// which node answered it.
func TestInheritedQuestionDischarge(t *testing.T) {
	g, _, _ := chainGraph()
	g.Nodes["ANS"] = ibis.Node{ID: "ANS", Kind: ibis.Argument, Text: "hybrid escapes it", Author: "ann"}
	g.Links = append(g.Links,
		ibis.Link{ID: "l11", Src: "ANS", Dst: "H2", Rel: ibis.Supports},
		ibis.Link{ID: "l12", Src: "ANS", Dst: "OBJ", Rel: ibis.Addresses},
	)
	fw, _ := af.Build(g)
	qs := InheritedQuestions(g, fw.Grounded(), "H2")
	if len(qs) != 1 || qs[0].Open {
		t.Fatalf("addressed question must be closed: %+v", qs)
	}
	if len(qs[0].AddressedBy) != 1 || qs[0].AddressedBy[0] != "ANS" {
		t.Fatalf("discharge must record the addressing node: %+v", qs[0])
	}
	if len(openQuestions(qs)) != 0 {
		t.Fatal("no open questions should remain")
	}
}

// SelfElevation: preferring H1 over parent A while A's objection is open (or
// answered only by the hybrid's own author) is the dodge; a cross-author
// address clears it; no lineage or no undefeated objection is never flagged.
func TestSelfElevationDetection(t *testing.T) {
	g, _, labels := chainGraph()
	// H1 subsumes A, whose OBJ is open -> flagged.
	open, flagged := SelfElevation(g, labels, "H1", "A")
	if !flagged || len(open) != 1 || open[0] != "OBJ" {
		t.Fatalf("open inherited question must flag the subsumption: open=%v flagged=%v", open, flagged)
	}
	// No lineage between C and A -> not flagged.
	if _, f := SelfElevation(g, labels, "C", "A"); f {
		t.Fatal("a non-parent is not a subsumption dodge")
	}

	// Self-authored address does not clear it (syn owns H1 and the answer).
	g.Nodes["SELF"] = ibis.Node{ID: "SELF", Kind: ibis.Argument, Text: "n/a", Author: "syn"}
	g.Links = append(g.Links,
		ibis.Link{ID: "s1", Src: "SELF", Dst: "H1", Rel: ibis.Supports},
		ibis.Link{ID: "s2", Src: "SELF", Dst: "OBJ", Rel: ibis.Addresses},
	)
	fw, _ := af.Build(g)
	if _, f := SelfElevation(g, fw.Grounded(), "H1", "A"); !f {
		t.Fatal("a self-authored address must not clear the dodge")
	}

	// A cross-author address clears it.
	g.Nodes["CROSS"] = ibis.Node{ID: "CROSS", Kind: ibis.Argument, Text: "genuinely answered", Author: "dan"}
	g.Links = append(g.Links,
		ibis.Link{ID: "c1", Src: "CROSS", Dst: "H1", Rel: ibis.Supports},
		ibis.Link{ID: "c2", Src: "CROSS", Dst: "OBJ", Rel: ibis.Addresses},
	)
	fw, _ = af.Build(g)
	if _, f := SelfElevation(g, fw.Grounded(), "H1", "A"); f {
		t.Fatal("a cross-author address must clear the dodge")
	}
}

// moves on an issue with question-laden hybrids emits one composite
// stress-test prompt per hybrid (not three uncoordinated nags), naming the
// cost question.
func TestMovesCompositeStressTest(t *testing.T) {
	g, fw, labels := chainGraph()
	mv := Moves(g, fw, labels, "I", nil)
	var stress int
	for _, m := range mv.Moves {
		if m.Move == "object" && strings.Contains(m.Effect, "stress-test this hybrid") {
			stress++
			if !strings.Contains(m.Effect, "adoption cost") {
				t.Fatalf("composite prompt must name the cost question: %q", m.Effect)
			}
		}
	}
	// H1 and H2 both carry an open question -> two composite prompts, one each.
	if stress != 2 {
		t.Fatalf("want one composite stress-test per question-laden hybrid, got %d", stress)
	}
}

// On a select_one stalemate, a hybrid rival with an open inherited question
// gets its prefer suggestion annotated — address the questions first.
func TestMovesPreferAnnotationOnQuestionLadenHybrid(t *testing.T) {
	// select_one issue: rivals A and B, hybrid H synthesizing both; A carries
	// a live cross-author objection OBJ. OBJ is IN -> A OUT; B and H mutually
	// attack (both also hit by OUT A) -> both UNDEC, so H has a prefer path.
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "q?"},
		{ID: "A", Kind: ibis.Position, Text: "a", Author: "ann"},
		{ID: "B", Kind: ibis.Position, Text: "b", Author: "bob"},
		{ID: "H", Kind: ibis.Position, Text: "hybrid", Author: "syn"},
		{ID: "OBJ", Kind: ibis.Argument, Text: "a is costly", Author: "bob"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l3", Src: "H", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l4", Src: "H", Dst: "A", Rel: ibis.Synthesizes},
		{ID: "l5", Src: "H", Dst: "B", Rel: ibis.Synthesizes},
		{ID: "l6", Src: "OBJ", Dst: "A", Rel: ibis.ObjectsTo},
	}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
	fw, _ := af.Build(g)
	labels := fw.Grounded()
	if labels["H"] != af.UNDEC {
		t.Fatalf("fixture: hybrid must be UNDEC to have a prefer path: %v", labels)
	}
	mv := Moves(g, fw, labels, "I", nil)
	var annotated bool
	for _, m := range mv.Moves {
		if m.Move == "prefer" && m.Args[0] == "H" && strings.Contains(m.Effect, "address the open inherited questions") {
			annotated = true
		}
	}
	if !annotated {
		t.Fatalf("prefer on a question-laden hybrid must be annotated: %+v", mv.Moves)
	}
}

// show and why render inherited questions and recorded drops.
func TestShowRendersDropsAndInheritedQuestions(t *testing.T) {
	g, _, labels := chainGraph()
	g.Nodes["H1"] = ibis.Node{ID: "H1", Kind: ibis.Position, Text: "h1", Author: "syn", Drops: []string{"B's caching layer"}}
	v := Show(g, labels, "H1", nil)
	if len(v.Drops) != 1 || len(v.InheritedQuestions) != 1 {
		t.Fatalf("show must carry drops and inherited questions: %+v", v)
	}
	txt := ShowText(v)
	if !strings.Contains(txt, "drops") || !strings.Contains(txt, "inherited questions") {
		t.Fatalf("show text missing drops/inherited-questions sections:\n%s", txt)
	}
}
