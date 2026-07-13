package proto

import (
	"testing"

	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/store"
)

// openFixture opens a store with one open-cardinality issue and two
// unopposed (hence both IN) positions — the composable-answers shape.
func openFixture(t *testing.T) (s *store.Store, m *Mover, issue, posA, posB string) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m = New(s, "tester", "")
	if err := s.AddDiscussion(ibis.Discussion{ID: "disc-1", Title: "t", CreatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	issue, err = m.Raise("disc-1", "which practices?", "", "", ibis.Open)
	if err != nil {
		t.Fatal(err)
	}
	posA, err = m.Propose("disc-1", issue, "pairing", "")
	if err != nil {
		t.Fatal(err)
	}
	posB, err = m.Propose("disc-1", issue, "spaced retrieval", "")
	if err != nil {
		t.Fatal(err)
	}
	return s, m, issue, posA, posB
}

// An open issue records a standing decision per position: the winners compose,
// so a second decide on a *different* position stands rather than being
// rejected as an overturn. A repeat on the same position is still rejected
// (wicked-problems-2.md item 6).
func TestOpenIssueDecidesPerPosition(t *testing.T) {
	s, m, issue, posA, posB := openFixture(t)

	if err := m.Decide("disc-1", issue, posA, "adopt pairing", 0); err != nil {
		t.Fatalf("decide A on open issue: %v", err)
	}
	if err := m.Decide("disc-1", issue, posB, "adopt spaced retrieval", 0); err != nil {
		t.Fatalf("second decide on an open issue must stand (different position): %v", err)
	}
	wantIllegal(t, m.Decide("disc-1", issue, posA, "again", 0),
		"repeat decide on the same open-issue position must be rejected")

	decs, err := s.Decisions("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	standing := map[string]bool{}
	for _, d := range decs {
		if d.Issue == issue {
			standing[d.Position] = true
		}
	}
	if !standing[posA] || !standing[posB] {
		t.Fatalf("both per-position decisions must stand: %v", standing)
	}
}

// Supersede on an open issue targets the decision on the given position and
// re-decides that same position; sibling decisions stand untouched.
func TestOpenIssueSupersedeIsPerPosition(t *testing.T) {
	s, m, issue, posA, posB := openFixture(t)

	if err := m.Decide("disc-1", issue, posA, "adopt pairing", 0); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("disc-1", issue, posB, "adopt spaced retrieval", 0); err != nil {
		t.Fatal(err)
	}

	// Superseding a position that carries no standing decision is illegal.
	posC, err := m.Propose("disc-1", issue, "code review", "")
	if err != nil {
		t.Fatal(err)
	}
	wantIllegal(t, m.Supersede("disc-1", issue, posC, "revise", 0),
		"supersede on an undecided open-issue position must be rejected")

	// Re-deciding B leaves A's decision standing.
	if err := m.Supersede("disc-1", issue, posB, "narrow to load-bearing modules", 0); err != nil {
		t.Fatalf("supersede B: %v", err)
	}
	decs, err := s.Decisions("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	byPos := map[string]ibis.Decision{}
	for _, d := range decs {
		if d.Issue == issue {
			byPos[d.Position] = d
		}
	}
	if _, ok := byPos[posA]; !ok {
		t.Fatal("A's decision must survive a supersede targeting B")
	}
	if byPos[posB].Basis != "narrow to load-bearing modules" {
		t.Fatalf("B's decision must be the revised one: %q", byPos[posB].Basis)
	}
}
