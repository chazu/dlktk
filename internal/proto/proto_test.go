package proto

import (
	"errors"
	"testing"

	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/store"
)

// fixture opens a temp store with one discussion, one issue, two positions.
func fixture(t *testing.T) (s *store.Store, m *Mover, issue, posA, posB string) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m = New(s, "tester")
	if err := s.AddDiscussion(ibis.Discussion{ID: "disc-1", Title: "t", CreatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	issue, err = m.Raise("disc-1", "which lock?", "", "")
	if err != nil {
		t.Fatal(err)
	}
	posA, err = m.Propose("disc-1", issue, "mutex")
	if err != nil {
		t.Fatal(err)
	}
	posB, err = m.Propose("disc-1", issue, "rwlock")
	if err != nil {
		t.Fatal(err)
	}
	return s, m, issue, posA, posB
}

func wantIllegal(t *testing.T, err error, context string) {
	t.Helper()
	var im *ibis.IllegalMove
	if !errors.As(err, &im) {
		t.Fatalf("%s: want IllegalMove, got %v", context, err)
	}
}

// Bare re-decide on a decided issue must be rejected (design §16 Q4); the
// overturn path is the explicit supersede move, which requires a basis and
// links the prior position.
func TestDecideSupersedePolicy(t *testing.T) {
	s, m, issue, posA, posB := fixture(t)

	if err := m.Decide("disc-1", issue, posA, "first call"); err != nil {
		t.Fatalf("first decide: %v", err)
	}
	wantIllegal(t, m.Decide("disc-1", issue, posB, "second call"), "bare re-decide")
	wantIllegal(t, m.Supersede("disc-1", issue, posB, ""), "supersede without basis")

	if err := m.Supersede("disc-1", issue, posB, "benchmarks favour rwlock"); err != nil {
		t.Fatalf("supersede: %v", err)
	}
	decs, err := s.Decisions("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(decs) != 1 {
		t.Fatalf("want 1 in-force decision, got %d", len(decs))
	}
	d := decs[0]
	if d.Position != posB || d.Supersedes != posA || d.Basis != "benchmarks favour rwlock" {
		t.Fatalf("superseding decision wrong: %+v", d)
	}
}

func TestSupersedeRequiresStandingDecision(t *testing.T) {
	_, m, issue, posA, _ := fixture(t)
	wantIllegal(t, m.Supersede("disc-1", issue, posA, "basis"), "supersede with nothing decided")
}
