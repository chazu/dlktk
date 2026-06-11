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
