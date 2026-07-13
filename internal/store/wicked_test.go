package store_test

import (
	"testing"
	"time"

	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/store"
)

// The new relations (reframe, value, audience) and extensions (node tag,
// synthesizes/raised_from links, decision review_by) survive the git-native
// export→import round-trip and the audit trail sees them.
func TestWickedRelationsRoundTrip(t *testing.T) {
	src, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.AddDiscussion(ibis.Discussion{ID: "d", Title: "t", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	m := proto.New(src, "alice", "Reframer")

	issue, err := m.Raise("d", "which lock?", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a, err := m.Propose("d", issue, "mutex", "velocity")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Propose("d", issue, "rwlock", "security")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Synthesize("d", issue, "hybrid", []string{a, b}, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Assume("d", a, "read-heavy workload"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Raise("d", "is shared state needed?", "", a, ""); err != nil {
		t.Fatal(err)
	}
	if err := m.DeclareAudience("d", "ops", []string{"security", "velocity"}, false, ""); err != nil {
		t.Fatal(err)
	}
	other, err := m.Raise("d", "side question?", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reframe("d", other, "sharper side question?", "was ambiguous", ""); err != nil {
		t.Fatal(err)
	}
	pos, err := m.Propose("d", issue, "spin lock", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = pos

	recs, err := src.Export("d")
	if err != nil {
		t.Fatal(err)
	}

	dst, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if _, err := dst.ImportAll(recs); err != nil {
		t.Fatal(err)
	}

	gs, err := src.Graph("d", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	gd, err := dst.Graph("d", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(gd.Nodes) != len(gs.Nodes) || len(gd.Links) != len(gs.Links) {
		t.Fatalf("structure lost: %d/%d nodes, %d/%d links", len(gd.Nodes), len(gs.Nodes), len(gd.Links), len(gs.Links))
	}
	if len(gd.Reframes) != 1 || gd.Reframes[0].Basis != "was ambiguous" {
		t.Fatalf("reframe lost: %+v", gd.Reframes)
	}
	if gd.Values[a] != "velocity" || gd.Values[b] != "security" {
		t.Fatalf("values lost: %+v", gd.Values)
	}
	if aud, ok := gd.Audiences["ops"]; !ok || len(aud.Ranking) != 2 {
		t.Fatalf("audience lost: %+v", gd.Audiences)
	}
	// Tags survive.
	tagged := 0
	for _, n := range gd.Nodes {
		if n.Tag == ibis.TagAssumption {
			tagged++
		}
	}
	if tagged != 1 {
		t.Fatalf("assumption tag lost: want 1, got %d", tagged)
	}

	// The audit trail sees the new relations.
	hist, err := src.History("d", "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, h := range hist {
		seen[h.Relation] = true
	}
	for _, rel := range []string{"dlktk/reframe", "dlktk/value", "dlktk/audience"} {
		if !seen[rel] {
			t.Fatalf("log blind to %s: %v", rel, seen)
		}
	}
}

// review_by survives the round-trip on decisions.
func TestReviewByRoundTrips(t *testing.T) {
	src, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.AddDiscussion(ibis.Discussion{ID: "d", Title: "t", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	m := proto.New(src, "alice", "")
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	horizon := time.Now().Add(24 * time.Hour).Unix()
	if err := m.Decide("d", issue, pos, "call", horizon); err != nil {
		t.Fatal(err)
	}

	recs, err := src.Export("d")
	if err != nil {
		t.Fatal(err)
	}
	dst, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if _, err := dst.ImportAll(recs); err != nil {
		t.Fatal(err)
	}
	decs, err := dst.Decisions("d", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(decs) != 1 || decs[0].ReviewBy != horizon {
		t.Fatalf("review_by lost: %+v", decs)
	}
}
