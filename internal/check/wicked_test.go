package check

import (
	"testing"
	"time"

	"github.com/chazu/dlktk/internal/store"
)

func findKind(v View, kind string) int {
	n := 0
	for _, f := range v.Findings {
		if f.Kind == kind {
			n++
		}
	}
	return n
}

// A decision on a never-attacked position is flagged: IN by silence is
// unexamined, not vindicated.
func TestUntestedDecisionWarns(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	if err := m.Decide("d", issue, pos, "obvious", 0); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, UntestedDecision) != 1 {
		t.Fatalf("want untested_decision warning: %+v", v)
	}
	if !v.OK {
		t.Fatalf("warnings must not fail a non-strict check: %+v", v)
	}
}

// A decision past its review horizon is flagged — against the supplied "now",
// not the wall clock.
func TestReviewDueFiresAfterHorizon(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	obj, _ := m.Object("d", pos, "objection", "", "")
	if _, err := m.Object("d", obj, "rebuttal", "", ""); err != nil {
		t.Fatal(err)
	}
	horizon := time.Now().Add(time.Hour).Unix()
	if err := m.Decide("d", issue, pos, "call", horizon); err != nil {
		t.Fatal(err)
	}

	before, err := Run(s, []string{"d"}, store.Now(), horizon-1)
	if err != nil {
		t.Fatal(err)
	}
	if findKind(before, ReviewDue) != 0 {
		t.Fatalf("review_due fired before the horizon: %+v", before)
	}
	after, err := Run(s, []string{"d"}, store.Now(), horizon+1)
	if err != nil {
		t.Fatal(err)
	}
	if findKind(after, ReviewDue) != 1 {
		t.Fatalf("want review_due after the horizon: %+v", after)
	}
}

// A decided position resting on a defeated assumption is flagged: the premise
// fell, the conclusion was never revisited.
func TestDefeatedAssumptionWarns(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	pos, _ := m.Propose("d", issue, "yes", "")
	obj, _ := m.Object("d", pos, "objection", "", "")
	if _, err := m.Object("d", obj, "rebuttal", "", ""); err != nil {
		t.Fatal(err)
	}
	assumption, err := m.Assume("d", pos, "load stays flat")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, pos, "call", 0); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), DefeatedAssumption) != 0 {
		t.Fatal("assumption not yet defeated; no warning expected")
	}
	if _, err := m.Object("d", assumption, "traffic doubled last quarter", "", ""); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, DefeatedAssumption) != 1 {
		t.Fatalf("want defeated_assumption warning: %+v", v)
	}
}

// A stalemate under a reframed (dead) framing stops warning: the question
// moved.
func TestReframedStalemateSuppressed(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	if _, err := m.Propose("d", issue, "a", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Propose("d", issue, "b", ""); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), Stalemate) != 1 {
		t.Fatal("fixture must stalemate before the reframe")
	}
	if _, err := m.Reframe("d", issue, "better question?", "false dichotomy", ""); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, Stalemate) != 0 {
		t.Fatalf("dead framing still warns: %+v", v)
	}
}
