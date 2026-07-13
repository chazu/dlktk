package check

// Arc two, item 1 (wicked-problems-2.md): untested_decision must ignore rival
// attacks and resist self-dealing — the exercise's decided hybrid reached
// `decide` without one substantive objection and the old definition (any
// attacker) was structurally blind to it.

import (
	"testing"

	"github.com/chazu/dlktk/internal/proto"
)

// The exercise regression: a hybrid elevated over its parents by subsumption
// preferences is "attacked" by its rivals, but nobody examined it. Deciding
// it must warn.
func TestUntestedDecisionIgnoresRivalAttacks(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	h, _, err := m.Synthesize("d", issue, "hybrid", []string{a, b}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Prefer("d", h, a, "subsumption"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Prefer("d", h, b, "subsumption"); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, h, "unique justified", 0); err != nil {
		t.Fatal(err)
	}
	f := only(t, run(t, s), UntestedDecision, "warning")
	if f.Node != h {
		t.Fatalf("finding must name the hybrid: %+v", f)
	}
}

// A strawman self-objection, promptly rebutted by the same mind, must not
// clear the bar; the identical shape with a cross-author objection must.
func TestUntestedDecisionResistsSelfObjection(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	strawman, _ := m.Object("d", pos, "weak self-objection", "", "")
	if _, err := m.Object("d", strawman, "easy rebuttal", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, pos, "survived scrutiny", 0); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), UntestedDecision) != 1 {
		t.Fatal("self-authored objection must not count as a test")
	}

	s2, m2 := open(t)
	issue2, _ := m2.Raise("d", "q?", "", "", "")
	pos2, _ := m2.Propose("d", issue2, "yes", "")
	bob := proto.New(s2, "bob", "")
	obj2, _ := bob.Object("d", pos2, "real objection", "", "")
	if _, err := m2.Object("d", obj2, "rebuttal", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := m2.Decide("d", issue2, pos2, "survived scrutiny", 0); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s2), UntestedDecision) != 0 {
		t.Fatal("cross-author objection beaten by counter-argument must count as a test")
	}
}

// Neutralizing the only objection with a preference excuses the position from
// its test: the untested warning must (re-)arm.
func TestUntestedDecisionReArmsAfterPreference(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	bob := proto.New(s, "bob", "")
	obj, _ := bob.Object("d", pos, "real objection", "", "")
	if _, _, err := m.Prefer("d", pos, obj, "time-boxed call"); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, pos, "preferred through", 0); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), UntestedDecision) != 1 {
		t.Fatal("preference-excused objection must not count as a test")
	}
}
