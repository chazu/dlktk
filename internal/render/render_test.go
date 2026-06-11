package render

import (
	"testing"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// settledGraph: issue I (select_one), positions A and B, argument C objecting
// to B. C is unattacked -> IN, so B -> OUT; A's only attacker is gone -> A IN.
// The issue is settled on A with nothing contested.
func settledGraph() (*ibis.Graph, *af.Framework, map[string]af.Label) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which lock?"},
		{ID: "A", Kind: ibis.Position, Text: "mutex"},
		{ID: "B", Kind: ibis.Position, Text: "rwlock"},
		{ID: "C", Kind: ibis.Argument, Text: "writer starvation", Author: "carol"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l3", Src: "C", Dst: "B", Rel: ibis.ObjectsTo},
	}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}})
	fw, err := af.Build(g)
	if err != nil {
		panic(err)
	}
	return g, fw, fw.Grounded()
}

// When an issue's labelling has settled on a unique IN position, moves must
// offer the terminal move (decide) — otherwise an agent following the tool's
// suggestions can never close an issue.
func TestMovesSuggestsDecideWhenSettled(t *testing.T) {
	g, fw, labels := settledGraph()
	if labels["A"] != af.IN || labels["B"] != af.OUT {
		t.Fatalf("fixture not settled: %v", labels)
	}

	mv := Moves(g, fw, labels, "I", nil)
	first := mv.Moves[0]
	if first.Move != "decide" || first.Args[0] != "I" || first.Args[1] != "A" {
		t.Fatalf("want decide I A first, got %+v", first)
	}

	// Already decided -> no decide suggestion.
	mv = Moves(g, fw, labels, "I", []ibis.Decision{{Issue: "I", Position: "A"}})
	for _, m := range mv.Moves {
		if m.Move == "decide" {
			t.Fatalf("decide suggested on an already-decided issue: %+v", m)
		}
	}
}

func TestAgendaReadyAndUnpopulated(t *testing.T) {
	g, _, labels := settledGraph()
	// Add an issue with no positions.
	g.Nodes["J"] = ibis.Node{ID: "J", Kind: ibis.Issue, Text: "which cache?"}

	v := Agenda(g, labels, nil)
	if len(v.Ready) != 1 || v.Ready[0].Issue != "I" || v.Ready[0].Position != "A" {
		t.Fatalf("ready wrong: %+v", v.Ready)
	}
	if v.Ready[0].PositionText != "mutex" {
		t.Fatalf("ready missing position text: %+v", v.Ready[0])
	}
	if len(v.Unpopulated) != 1 || v.Unpopulated[0].Issue != "J" {
		t.Fatalf("unpopulated wrong: %+v", v.Unpopulated)
	}

	// Decided -> drops out of ready.
	v = Agenda(g, labels, []ibis.Decision{{Issue: "I", Position: "A"}})
	if len(v.Ready) != 0 {
		t.Fatalf("decided issue still ready: %+v", v.Ready)
	}
}

// why must carry the node's and each attacker's text so an agent can read what
// it is rebutting without extra round-trips.
func TestWhyIncludesText(t *testing.T) {
	g, fw, labels := settledGraph()
	v := Why(g, fw, labels, "B")
	if v.Text != "rwlock" {
		t.Fatalf("why missing node text: %+v", v)
	}
	var texts []string
	for _, b := range v.Because {
		texts = append(texts, b.AttackerText)
	}
	if len(texts) != 2 { // attackers of B: A (rival) and C (objection)
		t.Fatalf("want 2 attackers with text, got %v", texts)
	}
	for _, tx := range texts {
		if tx == "" {
			t.Fatalf("attacker text empty: %+v", v.Because)
		}
	}
}

func TestShowLinksBothDirections(t *testing.T) {
	g, _, labels := settledGraph()
	v := Show(g, labels, "B", nil)
	if v.Kind != "position" || v.Text != "rwlock" || v.Label != "OUT" {
		t.Fatalf("show head wrong: %+v", v)
	}
	var in, out int
	for _, l := range v.Links {
		switch l.Dir {
		case "in":
			in++
			if l.Peer != "C" || l.PeerText != "writer starvation" || l.PeerLabel != "IN" {
				t.Fatalf("incoming link wrong: %+v", l)
			}
		case "out":
			out++
			if l.Peer != "I" || l.PeerText != "which lock?" || l.PeerLabel != "" {
				t.Fatalf("outgoing link wrong: %+v", l)
			}
		}
	}
	if in != 1 || out != 1 {
		t.Fatalf("want 1 in + 1 out link, got %d/%d: %+v", in, out, v.Links)
	}

	// Issues carry their standing decision.
	v = Show(g, labels, "I", []ibis.Decision{{Issue: "I", Position: "A", Decider: "x"}})
	if v.Decided == nil || v.Decided.Position != "A" {
		t.Fatalf("issue show missing decision: %+v", v)
	}
	if v.Label != "" {
		t.Fatalf("issue must not carry an AF label: %+v", v)
	}
}
