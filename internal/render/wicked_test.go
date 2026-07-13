package render

import (
	"strings"
	"testing"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// stalemateGraph: issue I (select_one) with rivals A and B and nothing else —
// a mutual stalemate.
func stalemateGraph() (*ibis.Graph, *af.Framework, map[string]af.Label) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which lock?"},
		{ID: "A", Kind: ibis.Position, Text: "mutex"},
		{ID: "B", Kind: ibis.Position, Text: "rwlock"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
	}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
	fw, err := af.Build(g)
	if err != nil {
		panic(err)
	}
	return g, fw, fw.Grounded()
}

// A sole position on an issue is IN by silence: status must say untested, and
// moves must suggest stress-testing before the decide.
func TestUntestedSurfaced(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "q?"},
		{ID: "A", Kind: ibis.Position, Text: "the obvious answer"},
	}
	links := []ibis.Link{{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo}}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, nil, nil)
	fw, _ := af.Build(g)
	labels := fw.Grounded()

	st := Status(g, fw, labels, "I", nil)
	if !st.Positions[0].Untested {
		t.Fatalf("sole unattacked IN position must be untested: %+v", st.Positions[0])
	}

	mv := Moves(g, fw, labels, "I", nil)
	if mv.Moves[0].Move != "object" || mv.Moves[1].Move != "decide" {
		t.Fatalf("want stress-test object before decide, got %+v", mv.Moves[:2])
	}

	v := Agenda(g, fw, labels, nil)
	if len(v.Untested) != 1 || v.Untested[0].Position != "A" {
		t.Fatalf("agenda untested wrong: %+v", v.Untested)
	}

	// A winner whose only attack was its rival's edge is still untested:
	// rival attacks are not examination (arc two item 1). settledGraph's A
	// won because C defeated B; nothing ever engaged A itself.
	g2, fw2, labels2 := settledGraph()
	st2 := Status(g2, fw2, labels2, "I", nil)
	for _, p := range st2.Positions {
		if p.ID == "A" && !p.Untested {
			t.Fatalf("rival-mediated winner must be untested: %+v", p)
		}
		if p.ID == "B" && p.Untested {
			t.Fatalf("defeated position must not be flagged untested: %+v", p)
		}
	}
}

// Stalemates get the generative exits: synthesize and reframe suggestions,
// with the honest joins-the-rivalry caveat.
func TestStalemateSuggestsSynthesisAndReframe(t *testing.T) {
	g, fw, labels := stalemateGraph()
	st := Status(g, fw, labels, "I", nil)
	if !st.Stalemate {
		t.Fatalf("fixture must be a stalemate: %+v", st)
	}
	if !strings.Contains(st.Advice, "synthesis") || !strings.Contains(st.Advice, "reframe") {
		t.Fatalf("stalemate advice missing the generative exits: %q", st.Advice)
	}
	mv := Moves(g, fw, labels, "I", nil)
	var synth, reframe *MoveSuggestion
	for i := range mv.Moves {
		switch mv.Moves[i].Move {
		case "synthesize":
			synth = &mv.Moves[i]
		case "reframe":
			reframe = &mv.Moves[i]
		}
	}
	if synth == nil || reframe == nil {
		t.Fatalf("stalemate moves missing synthesize/reframe: %+v", mv.Moves)
	}
	if !strings.Contains(synth.Effect, "joins the rivalry") {
		t.Fatalf("synthesize effect must be honest about the N+1-way stalemate: %q", synth.Effect)
	}
}

// A reframed issue leaves the agenda entirely — including its node scope — and
// the all-issues status marks it.
func TestReframeExclusions(t *testing.T) {
	g, _, _ := stalemateGraph()
	g.Nodes["J"] = ibis.Node{ID: "J", Kind: ibis.Issue, Text: "which concurrency strategy?"}
	g.Reframes = []ibis.Reframe{{Old: "I", New: "J", Basis: "false dichotomy"}}
	fw, _ := af.Build(g)
	labels := fw.Grounded()

	v := Agenda(g, fw, labels, nil)
	if len(v.Undecided) != 0 {
		t.Fatalf("dead framing's UNDEC nodes still on the agenda: %+v", v.Undecided)
	}
	for _, r := range append(v.Ready, v.Unpopulated...) {
		if r.Issue == "I" {
			t.Fatalf("reframed issue still listed: %+v", r)
		}
	}

	st := Status(g, fw, labels, "I", nil)
	if st.ReframedTo != "J" || !strings.Contains(st.Advice, "reframed") {
		t.Fatalf("status must mark the dead framing: %+v", st)
	}
}

// whatif reports the flips a hypothetical objection would cause — without
// touching the real graph.
func TestWhatIfObjection(t *testing.T) {
	g, _, labels := settledGraph()
	linksBefore, nodesBefore := len(g.Links), len(g.Nodes)

	v, err := WhatIf(g, labels, "I", []Hypothetical{HypObject("C")})
	if err != nil {
		t.Fatal(err)
	}
	// Defeating C revives the rivalry: A IN->UNDEC, B OUT->UNDEC, C IN->OUT.
	if len(v.Flipped) != 3 {
		t.Fatalf("want 3 flips, got %+v", v.Flipped)
	}
	if !v.Result.Stalemate {
		t.Fatalf("result must be the revived stalemate: %+v", v.Result)
	}
	if len(g.Links) != linksBefore || len(g.Nodes) != nodesBefore {
		t.Fatal("whatif mutated the real graph")
	}
}

func TestWhatIfRejectsCyclicPreference(t *testing.T) {
	g, _, labels := settledGraph()
	g.Preferences = []ibis.Preference{{ID: "p", Winner: "A", Loser: "B", Basis: "x"}}
	if _, err := WhatIf(g, labels, "I", []Hypothetical{HypPrefer("B", "A")}); err == nil {
		t.Fatal("hypothetical preference cycle must be rejected")
	}
}

// crux identifies the argument the verdict rests on: in the settled graph the
// whole outcome hinges on C.
func TestCruxFindsLoadBearingArgument(t *testing.T) {
	g, _, labels := settledGraph()
	v, err := Crux(g, labels, "I")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Cruxes) != 1 || v.Cruxes[0].Node != "C" {
		t.Fatalf("want C as the sole crux, got %+v", v.Cruxes)
	}
	if len(v.Cruxes[0].Flips) != 2 { // A and B both flip
		t.Fatalf("want 2 position flips, got %+v", v.Cruxes[0].Flips)
	}
}

// worlds on the stalemate: two coherent stances, both positions contingent.
func TestWorldsOnStalemate(t *testing.T) {
	g, fw, _ := stalemateGraph()
	v := Worlds(g, fw, "I")
	if len(v.Worlds) != 2 {
		t.Fatalf("mutual pair: want 2 worlds, got %+v", v.Worlds)
	}
	if len(v.Contingent) != 2 || len(v.Robust) != 0 {
		t.Fatalf("both positions must be contingent: %+v", v)
	}
}

// Unexamined assumptions surface on the agenda and drop off once examined.
func TestAgendaAssumptions(t *testing.T) {
	g, _, _ := settledGraph()
	g.Nodes["S"] = ibis.Node{ID: "S", Kind: ibis.Argument, Text: "read-heavy forever", Tag: ibis.TagAssumption}
	g.Links = append(g.Links, ibis.Link{ID: "l4", Src: "S", Dst: "A", Rel: ibis.Supports})
	fw, _ := af.Build(g)
	labels := fw.Grounded()

	v := Agenda(g, fw, labels, nil)
	if len(v.Assumptions) != 1 || v.Assumptions[0].ID != "S" {
		t.Fatalf("unexamined assumption missing: %+v", v.Assumptions)
	}

	// Objecting to it makes it examined.
	g.Nodes["X"] = ibis.Node{ID: "X", Kind: ibis.Argument, Text: "traffic mix is shifting"}
	g.Links = append(g.Links, ibis.Link{ID: "l5", Src: "X", Dst: "S", Rel: ibis.ObjectsTo})
	fw, _ = af.Build(g)
	v = Agenda(g, fw, fw.Grounded(), nil)
	if len(v.Assumptions) != 0 {
		t.Fatalf("examined assumption still listed: %+v", v.Assumptions)
	}
}

// A raise --from sub-issue renders nested under its spawning node, exactly
// once (root detection must treat raised_from as non-root).
func TestTreeRaisedFromRendersOnce(t *testing.T) {
	g, _, _ := settledGraph()
	g.Nodes["J"] = ibis.Node{ID: "J", Kind: ibis.Issue, Text: "deeper question"}
	g.Links = append(g.Links, ibis.Link{ID: "l6", Src: "J", Dst: "C", Rel: ibis.RaisedFrom})

	out := Tree(g, "", TreeOpts{NoLegend: true}, nil, nil, nil, nil)
	if n := strings.Count(out, "deeper question"); n != 1 {
		t.Fatalf("raised-from issue rendered %d times:\n%s", n, out)
	}
	// It must appear indented under C, not at the left margin.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "deeper question") && !strings.HasPrefix(line, " ") {
			t.Fatalf("raised-from issue rendered as a root:\n%s", out)
		}
	}
}

// Synthesis lineage is annotated on the hybrid's tree line.
func TestTreeShowsSynthesisLineage(t *testing.T) {
	g, _, _ := stalemateGraph()
	g.Nodes["H"] = ibis.Node{ID: "H", Kind: ibis.Position, Text: "hybrid"}
	g.Links = append(g.Links,
		ibis.Link{ID: "l3", Src: "H", Dst: "I", Rel: ibis.RespondsTo},
		ibis.Link{ID: "l4", Src: "H", Dst: "A", Rel: ibis.Synthesizes},
		ibis.Link{ID: "l5", Src: "H", Dst: "B", Rel: ibis.Synthesizes},
	)
	out := Tree(g, "I", TreeOpts{NoLegend: true}, nil, nil, nil, nil)
	if !strings.Contains(out, "⊕ from ") || !strings.Contains(out, "p:A") || !strings.Contains(out, "p:B") {
		t.Fatalf("synthesis lineage missing:\n%s", out)
	}
	if n := strings.Count(out, "hybrid"); n != 1 {
		t.Fatalf("hybrid rendered %d times (synthesizes must not nest):\n%s", n, out)
	}
}

// The audiences report separates robust from audience-sensitive positions.
func TestAudiencesReport(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I", Kind: ibis.Issue, Text: "which?"},
		{ID: "A", Kind: ibis.Position, Text: "fast"},
		{ID: "B", Kind: ibis.Position, Text: "safe"},
	}
	links := []ibis.Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: ibis.RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: ibis.RespondsTo},
	}
	values := []ibis.ValueTag{{Node: "A", Value: "velocity"}, {Node: "B", Value: "security"}}
	audiences := []ibis.Audience{
		{Name: "ops", Ranking: []string{"security", "velocity"}},
		{Name: "growth", Ranking: []string{"velocity", "security"}},
	}
	g := ibis.NewGraph(nodes, links, nil, []ibis.IssueCard{{Issue: "I", Cardinality: ibis.SelectOne}}, nil, values, audiences)

	v, err := Audiences(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Issues) != 1 {
		t.Fatalf("want one issue, got %+v", v.Issues)
	}
	ai := v.Issues[0]
	// Opposed rankings: each rival wins under one audience — both sensitive,
	// none robust.
	if len(ai.Robust) != 0 || len(ai.Sensitive) != 2 {
		t.Fatalf("want 0 robust / 2 sensitive, got %+v", ai)
	}
	if ai.ByAudience["ops"]["B"] != "IN" || ai.ByAudience["growth"]["A"] != "IN" {
		t.Fatalf("per-audience verdicts wrong: %+v", ai.ByAudience)
	}
}
