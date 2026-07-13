package check

import (
	"testing"
	"time"

	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/store"
)

func open(t *testing.T) (*store.Store, *proto.Mover) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.AddDiscussion(ibis.Discussion{ID: "d", Title: "t", CreatedBy: "x"}); err != nil {
		t.Fatal(err)
	}
	return s, proto.New(s, "x", "")
}

func run(t *testing.T, s *store.Store) View {
	t.Helper()
	v, err := Run(s, []string{"d"}, store.Now(), time.Now().Unix())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func only(t *testing.T, v View, kind, severity string) Finding {
	t.Helper()
	if len(v.Findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(v.Findings), v.Findings)
	}
	f := v.Findings[0]
	if f.Kind != kind || f.Severity != severity {
		t.Fatalf("want %s/%s, got %s/%s", kind, severity, f.Kind, f.Severity)
	}
	return f
}

func TestCleanDiscussionPasses(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	// Battle-test the position (a cross-author objection + rebuttal reinstates
	// it) so the decision is examined, not merely unopposed — a self-authored
	// objection would not count (arc two item 1).
	bob := proto.New(s, "bob", "")
	obj, _ := bob.Object("d", pos, "objection", "", "")
	if _, err := m.Object("d", obj, "rebuttal", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, pos, "obvious", 0); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if !v.OK || len(v.Findings) != 0 {
		t.Fatalf("clean discussion flagged: %+v", v)
	}
}

// A decision taken when its position was IN must be flagged once the dialectic
// moves out from under it (here: a fresh, undefeated objection).
func TestDecisionDrift(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	if err := m.Decide("d", issue, pos, "looked solid", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Object("d", pos, "new constraint kills this", "", ""); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	f := only(t, v, DecisionDrift, "error")
	if v.OK || f.Issue != issue || f.Node != pos {
		t.Fatalf("drift finding wrong: ok=%v %+v", v.OK, f)
	}
}

// Override decisions were knowingly divergent: not-IN is their expected state,
// not drift.
func TestOverrideDecisionIsNotDrift(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	if _, err := m.Object("d", pos, "objection", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, pos, "time-boxed call", 0); err != nil { // pos is OUT -> override
		t.Fatal(err)
	}
	v := run(t, s)
	if !v.OK || len(v.Findings) != 0 {
		t.Fatalf("override decision flagged as drift: %+v", v)
	}
}

func TestStalemateWarns(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	if _, err := m.Propose("d", issue, "a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Propose("d", issue, "b", ""); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	f := only(t, v, Stalemate, "warning")
	if !v.OK { // warnings don't fail a non-strict check
		t.Fatalf("warning-only check reported not OK: %+v", v)
	}
	if f.Issue != issue {
		t.Fatalf("stalemate finding wrong: %+v", f)
	}
}

// Store-level writes can bypass move legality (raced writers, manual edits);
// check must surface the resulting invariant violations.
func TestPreferenceCycleAndDuplicateNode(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	if err := s.AddPreference(ibis.Preference{ID: "p1", Disc: "d", Winner: a, Loser: b, Author: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPreference(ibis.Preference{ID: "p2", Disc: "d", Winner: b, Loser: a, Author: "x"}); err != nil {
		t.Fatal(err)
	}
	// Duplicate current id: same args.id, different text.
	if err := s.AddNode(ibis.Node{ID: a, Disc: "d", Kind: ibis.Position, Text: "a-clone", Author: "x"}); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if v.OK {
		t.Fatalf("corrupt store passed check: %+v", v)
	}
	kinds := map[string]bool{}
	for _, f := range v.Findings {
		kinds[f.Kind] = true
	}
	if !kinds[PreferenceCycle] || !kinds[StoreInvariant] {
		t.Fatalf("want preference_cycle + store_invariant, got %+v", v.Findings)
	}
}
