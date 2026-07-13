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
	m = New(s, "tester", "")
	if err := s.AddDiscussion(ibis.Discussion{ID: "disc-1", Title: "t", CreatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	issue, err = m.Raise("disc-1", "which lock?", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	posA, err = m.Propose("disc-1", issue, "mutex", "")
	if err != nil {
		t.Fatal(err)
	}
	posB, err = m.Propose("disc-1", issue, "rwlock", "")
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

	if err := m.Decide("disc-1", issue, posA, "first call", 0); err != nil {
		t.Fatalf("first decide: %v", err)
	}
	wantIllegal(t, m.Decide("disc-1", issue, posB, "second call", 0), "bare re-decide")
	wantIllegal(t, m.Supersede("disc-1", issue, posB, "", 0), "supersede without basis")

	if err := m.Supersede("disc-1", issue, posB, "benchmarks favour rwlock", 0); err != nil {
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
	wantIllegal(t, m.Supersede("disc-1", issue, posA, "basis", 0), "supersede with nothing decided")
}

// Two agents concurrently asserting prefer A>B and prefer B>A is the ANALYSIS
// §1.4 TOCTOU race: with check and write in separate store operations both
// passed the cycle check and both landed, collapsing the labelling (§16 Q2).
// With moves running inside store.Move transactions, exactly one may win.
func TestConcurrentPreferCannotCreateCycle(t *testing.T) {
	s, _, _, posA, posB := fixture(t)

	agentX := New(s, "agent-x", "")
	agentY := New(s, "agent-y", "")

	errs := make(chan error, 2)
	go func() {
		_, _, err := agentX.Prefer("disc-1", posA, posB, "throughput")
		errs <- err
	}()
	go func() {
		_, _, err := agentY.Prefer("disc-1", posB, posA, "simplicity")
		errs <- err
	}()

	var failures, successes int
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			wantIllegal(t, err, "losing prefer")
			failures++
		} else {
			successes++
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("want exactly one prefer to win, got %d successes / %d rejections", successes, failures)
	}

	prefs, err := s.Preferences("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs) != 1 {
		t.Fatalf("want 1 stored preference, got %d", len(prefs))
	}
}

// Ownership rides on the author identity, not the persona: bob acting as the
// same "Reviewer" role cannot concede a node alice authored (design §16 Q6).
func TestConcedeOwnershipIsAuthorNotRole(t *testing.T) {
	s, _, _, _, _ := fixture(t)

	alice := New(s, "alice", "Reviewer")
	bob := New(s, "bob", "Reviewer")

	node, err := alice.Propose("disc-1", mustIssue(t, s), "alice's position", "")
	if err != nil {
		t.Fatal(err)
	}
	// Same role, different identity: must be rejected.
	wantIllegal(t, bob.Concede("disc-1", node), "bob conceding alice's node under shared role")
	// The real author can.
	if err := alice.Concede("disc-1", node); err != nil {
		t.Fatalf("alice conceding her own node: %v", err)
	}
}

// A non-empty role auto-records the author↔role roster binding on the first move
// and dedups thereafter (design §16 Q8). No role => no binding.
func TestRoleAutoRecordsRoster(t *testing.T) {
	s, _, _, posA, _ := fixture(t) // fixture's mover has no role

	sec := New(s, "alice", "Security")
	if _, err := sec.Object("disc-1", posA, "threat model gap", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := sec.Object("disc-1", posA, "second objection", "", ""); err != nil {
		t.Fatal(err)
	}

	rs, err := s.Rosters("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	// One binding despite two moves (content-addressed dedup); the no-role
	// fixture moves recorded none.
	if len(rs) != 1 {
		t.Fatalf("want 1 roster binding, got %d: %+v", len(rs), rs)
	}
	if rs[0].Author != "alice" || rs[0].Role != "Security" || rs[0].Disc != "disc-1" {
		t.Fatalf("roster binding wrong: %+v", rs[0])
	}
}

// mustIssue returns the discussion's single issue id.
func mustIssue(t *testing.T, s *store.Store) string {
	t.Helper()
	g, err := s.Graph("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	is := g.Issues()
	if len(is) != 1 {
		t.Fatalf("want 1 issue, got %d", len(is))
	}
	return is[0]
}

// A move whose later write fails must not leave earlier writes behind: the
// transaction rolls the whole move back. Raise on a bogus parent fails its
// legality check after no writes; a nested Move (programming error) surfaces
// as a store error rather than deadlocking.
func TestMoveRollsBackAtomically(t *testing.T) {
	s, m, _, _, _ := fixture(t)

	// Failing legality check: nothing written.
	before, err := s.Nodes("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Propose("disc-1", "no-such-issue", "ghost", ""); err == nil {
		t.Fatal("propose on missing issue should fail")
	}
	after, err := s.Nodes("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("failed move wrote nodes: %d -> %d", len(before), len(after))
	}

	// Mid-transaction error rolls back earlier writes in the same move.
	err = s.Move(func(txs *store.Store) error {
		if err := txs.AddNode(ibis.Node{ID: "tmp-node", Disc: "disc-1", Kind: ibis.Argument, Text: "x", Author: "tester"}); err != nil {
			return err
		}
		return errors.New("abort")
	})
	if err == nil || err.Error() != "abort" {
		t.Fatalf("want callback error back, got %v", err)
	}
	after, err = s.Nodes("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range after {
		if n.ID == "tmp-node" {
			t.Fatal("rolled-back write is visible")
		}
	}

	// Nested Move is rejected, not deadlocked.
	err = s.Move(func(txs *store.Store) error {
		return txs.Move(func(*store.Store) error { return nil })
	})
	if err == nil {
		t.Fatal("nested Move should error")
	}
}
