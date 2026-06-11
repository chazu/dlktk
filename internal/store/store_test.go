package store

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/chazu/dlktk/internal/ibis"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func rec(t *testing.T, relation string, args any) ExportRecord {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return ExportRecord{Relation: relation, Args: b}
}

// An import batch whose preferences — combined with what the store already
// holds — would form a cycle must be rejected wholesale (nothing written):
// this is the path that previously labelled two select_one rivals both IN.
func TestImportRejectsPreferenceCycle(t *testing.T) {
	s := open(t)
	if err := s.AddPreference(ibis.Preference{ID: "p1", Disc: "d", Winner: "A", Loser: "B", Author: "x"}); err != nil {
		t.Fatal(err)
	}

	batch := []ExportRecord{
		rec(t, "dlktk/node", ibis.Node{ID: "n1", Disc: "d", Kind: ibis.Argument, Text: "t", Author: "x"}),
		rec(t, "dlktk/preference", ibis.Preference{ID: "p2", Disc: "d", Winner: "B", Loser: "A", Author: "x"}),
	}
	_, err := s.ImportAll(batch)
	var im *ibis.IllegalMove
	if !errors.As(err, &im) {
		t.Fatalf("want IllegalMove for cyclic import, got %v", err)
	}
	// Nothing from the batch may have landed — not even the valid node.
	nodes, err := s.Nodes("d", Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 0 {
		t.Fatalf("cyclic batch partially imported: %d nodes written", len(nodes))
	}
}

func TestImportRejectsMalformedRecords(t *testing.T) {
	s := open(t)
	cases := []ExportRecord{
		rec(t, "dlktk/bogus", map[string]string{"x": "y"}),
		rec(t, "dlktk/node", ibis.Node{ID: "n1", Disc: "d", Kind: "verdict", Text: "t"}),
		rec(t, "dlktk/node", ibis.Node{Disc: "d", Kind: ibis.Issue, Text: "missing id"}),
		rec(t, "dlktk/link", ibis.Link{ID: "l1", Disc: "d", Src: "a", Dst: "b", Rel: "refutes"}),
		rec(t, "dlktk/issue_card", ibis.IssueCard{Issue: "i", Cardinality: "many"}),
		rec(t, "dlktk/preference", ibis.Preference{ID: "p", Disc: "d", Winner: "A"}), // no loser
		{Relation: "dlktk/node", Args: json.RawMessage(`{"id": 42}`)},                // wrong type
	}
	for i, bad := range cases {
		_, err := s.ImportAll([]ExportRecord{bad})
		var im *ibis.IllegalMove
		if !errors.As(err, &im) {
			t.Errorf("case %d (%s): want IllegalMove, got %v", i, bad.Relation, err)
		}
	}
}

// A history export must replay retractions and decision supersessions into a
// fresh store: current state matches, and the audit trail (including closed
// facts) survives the round-trip. Importing twice must converge.
func TestHistoryExportRoundTrip(t *testing.T) {
	src := open(t)
	if err := src.AddDiscussion(ibis.Discussion{ID: "d", Title: "t", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []ibis.Node{
		{ID: "i1", Disc: "d", Kind: ibis.Issue, Text: "q?", Author: "x"},
		{ID: "p1", Disc: "d", Kind: ibis.Position, Text: "yes", Author: "x"},
		{ID: "a1", Disc: "d", Kind: ibis.Argument, Text: "withdrawn claim", Author: "x"},
	} {
		if err := src.AddNode(n); err != nil {
			t.Fatal(err)
		}
	}
	if err := src.RetractNode("a1"); err != nil { // concede
		t.Fatal(err)
	}
	if err := src.AddDecision(ibis.Decision{Disc: "d", Issue: "i1", Position: "p1", Decider: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := src.SupersedeDecision("d", "i1"); err != nil { // close vt interval
		t.Fatal(err)
	}
	if err := src.AddDecision(ibis.Decision{Disc: "d", Issue: "i1", Position: "p1", Basis: "again", Decider: "x", Supersedes: "p1"}); err != nil {
		t.Fatal(err)
	}

	recs, err := src.ExportHistory("d")
	if err != nil {
		t.Fatal(err)
	}
	events := 0
	for _, r := range recs {
		if r.Event != "" {
			events++
		}
	}
	if events != 2 { // a1 retract + first decision invalidate
		t.Fatalf("want 2 events in history export, got %d: %+v", events, recs)
	}

	dst := open(t)
	for round := 1; round <= 2; round++ {
		if _, err := dst.ImportAll(recs); err != nil {
			t.Fatalf("import round %d: %v", round, err)
		}
		g, err := dst.Graph("d", Now())
		if err != nil {
			t.Fatal(err)
		}
		if _, alive := g.Nodes["a1"]; alive || len(g.Nodes) != 2 {
			t.Fatalf("round %d: retraction not replayed: %v", round, g.Nodes)
		}
		decs, err := dst.Decisions("d", Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(decs) != 1 || decs[0].Basis != "again" {
			t.Fatalf("round %d: supersession not replayed: %+v", round, decs)
		}
		hist, err := dst.History("d", "a1")
		if err != nil {
			t.Fatal(err)
		}
		if len(hist) != 1 || !hist[0].Retracted {
			t.Fatalf("round %d: audit trail lost: %+v", round, hist)
		}
	}
}

func TestImportRejectsMalformedEvents(t *testing.T) {
	s := open(t)
	for i, bad := range []ExportRecord{
		{Relation: "dlktk/node", Event: "destroy", Ref: "r"},
		{Relation: "dlktk/node", Event: EventRetract},
		{Relation: "dlktk/bogus", Event: EventRetract, Ref: "r"},
	} {
		_, err := s.ImportAll([]ExportRecord{bad})
		var im *ibis.IllegalMove
		if !errors.As(err, &im) {
			t.Errorf("case %d: want IllegalMove, got %v", i, err)
		}
	}
	// A well-formed event referencing a fact that is in neither the store nor
	// the batch fails at apply time.
	_, err := s.ImportAll([]ExportRecord{{Relation: "dlktk/node", Event: EventRetract, Ref: "no-such-fact"}})
	var im *ibis.IllegalMove
	if !errors.As(err, &im) {
		t.Errorf("dangling ref: want IllegalMove, got %v", err)
	}
}

// export -> import into a fresh store must reproduce the graph, and importing
// the same batch twice must be a no-op (content-addressed dedup).
func TestExportImportRoundTrip(t *testing.T) {
	src := open(t)
	if err := src.AddDiscussion(ibis.Discussion{ID: "d", Title: "t", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := src.AddNode(ibis.Node{ID: "i1", Disc: "d", Kind: ibis.Issue, Text: "q?", Author: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := src.AddNode(ibis.Node{ID: "p1", Disc: "d", Kind: ibis.Position, Text: "yes", Author: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := src.AddLink(ibis.Link{ID: "l1", Disc: "d", Src: "p1", Dst: "i1", Rel: ibis.RespondsTo, Author: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := src.SetIssueCard(ibis.IssueCard{Issue: "i1", Cardinality: ibis.SelectOne}); err != nil {
		t.Fatal(err)
	}
	recs, err := src.Export("d")
	if err != nil {
		t.Fatal(err)
	}

	dst := open(t)
	for round := 1; round <= 2; round++ { // second round exercises idempotence
		if _, err := dst.ImportAll(recs); err != nil {
			t.Fatalf("import round %d: %v", round, err)
		}
		g, err := dst.Graph("d", Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Nodes) != 2 || len(g.Links) != 1 {
			t.Fatalf("round %d: got %d nodes / %d links, want 2/1", round, len(g.Nodes), len(g.Links))
		}
		if g.IssueCards["i1"] != ibis.SelectOne {
			t.Fatalf("round %d: issue card lost", round)
		}
	}
}
