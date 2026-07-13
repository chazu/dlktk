package store_test

import (
	"testing"

	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/store"
)

// Every move under a role re-records the author↔role binding, but the roster is
// the attribution audit, not a move log: it reports one row per distinct
// binding (wicked-problems-2.md item 10).
func TestRosterDedupsBindings(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.AddDiscussion(ibis.Discussion{ID: "d", Title: "t", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	m := proto.New(s, "alice", "Reframer")
	issue, _ := m.Raise("d", "q?", "", "", "")
	// Several more moves under the same author+role — each auto-records the
	// binding again.
	for i := 0; i < 5; i++ {
		if _, err := m.Propose("d", issue, "candidate", ""); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := s.Rosters("d", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("want one distinct (author, role) binding, got %d: %+v", len(rs), rs)
	}
	if rs[0].Author != "alice" || rs[0].Role != "Reframer" {
		t.Fatalf("wrong binding: %+v", rs[0])
	}
}
