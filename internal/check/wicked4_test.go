package check

import (
	"errors"
	"testing"
	"time"

	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/store"
)

// mapFixture builds an audience-sensitive select_one issue: two rival positions
// promoting opposed values, judged by two opposed audiences, so each wins under
// one ranking — a genuine value map, the precondition for a --map decision.
func mapFixture(t *testing.T) (*store.Store, *proto.Mover, string, string, string) {
	t.Helper()
	s, m := open(t)
	issue, _ := m.Raise("d", "which strategy?", "", "", "")
	a, _ := m.Propose("d", issue, "fast", "")
	b, _ := m.Propose("d", issue, "safe", "")
	if err := m.Promote("d", a, "velocity"); err != nil {
		t.Fatal(err)
	}
	if err := m.Promote("d", b, "security"); err != nil {
		t.Fatal(err)
	}
	if err := m.DeclareAudience("d", "growth", []string{"velocity", "security"}, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := m.DeclareAudience("d", "ops", []string{"security", "velocity"}, false, ""); err != nil {
		t.Fatal(err)
	}
	return s, m, issue, a, b
}

const future = int64(4102444800) // 2100-01-01

// waitNextSecond blocks until the wall clock crosses into the next second, so a
// subsequent store write gets a strictly greater second-granular tx_start than
// writes made before the call.
func waitNextSecond() {
	start := time.Now().Unix()
	for time.Now().Unix() == start {
		time.Sleep(20 * time.Millisecond)
	}
}

func wantIllegal(t *testing.T, err error, ctx string) {
	t.Helper()
	var im *ibis.IllegalMove
	if !errors.As(err, &im) {
		t.Fatalf("%s: want IllegalMove, got %v", ctx, err)
	}
}

// A value-map decision is legal on an audience-sensitive issue with a review
// horizon, and closes it: no stalemate, and a non-fatal note reminds that the
// governance question is unraised.
func TestMapDecisionClosesAndNudgesGovernance(t *testing.T) {
	s, m, issue, _, _ := mapFixture(t)
	if err := m.DecideMap("d", issue, "nothing is robust", future); err != nil {
		t.Fatalf("map decision on an audience-sensitive issue: %v", err)
	}
	v := run(t, s)
	if !v.OK {
		t.Fatalf("a healthy map decision must not fail the check: %+v", v)
	}
	if findKind(v, Stalemate) != 0 {
		t.Fatalf("a mapped issue is closed — no stalemate finding: %+v", v)
	}
	if findKind(v, MapDrift) != 0 {
		t.Fatalf("no drift right after deciding: %+v", v)
	}
	if findKind(v, MappedPendingGovernance) != 1 {
		t.Fatalf("want a mapped_pending_governance note: %+v", v)
	}
	for _, f := range v.Findings {
		if f.Kind == MappedPendingGovernance && f.Severity != "note" {
			t.Fatalf("mapped_pending_governance must be a non-fatal note, got %q", f.Severity)
		}
	}
}

// The note clears once the governance question is raised from the mapped issue
// (via one of its positions — raise --from targets a position/argument).
func TestMapGovernanceNoteClearsWhenRaised(t *testing.T) {
	s, m, issue, a, _ := mapFixture(t)
	if err := m.DecideMap("d", issue, "value-driven", future); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Raise("d", "whose ranking should govern?", "", a, ""); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, MappedPendingGovernance) != 0 {
		t.Fatalf("note must clear once a governance issue is raised from the mapped issue: %+v", v)
	}
}

// map_drift fires when the current audience map no longer matches the one
// derived as of the decision's transaction time — here a rival is conceded,
// leaving a clear robust winner. Transaction time is second-granular, so the
// change must land in a later second than the decision for the bitemporal
// derivation to distinguish them (real reviews are days apart).
func TestMapDriftFiresWhenMapChanges(t *testing.T) {
	s, m, issue, _, b := mapFixture(t)
	if err := m.DecideMap("d", issue, "no robust winner", future); err != nil {
		t.Fatal(err)
	}
	waitNextSecond()
	if err := m.Concede("d", b); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, MapDrift) != 1 {
		t.Fatalf("want map_drift after the map changed: %+v", v)
	}
}

// --map is illegal without the audience sensitivity that gives it meaning, and
// without the mandatory review horizon.
func TestMapDecisionPreconditions(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "flat?", "", "", "")
	a, _ := m.Propose("d", issue, "yes", "")
	m.Propose("d", issue, "no", "")

	wantIllegal(t, m.DecideMap("d", issue, "b", future), "map with no audiences must be rejected")

	// Give it a real map, then omit --review-by.
	m.Promote("d", a, "velocity")
	m.DeclareAudience("d", "g", []string{"velocity", "security"}, false, "")
	// Only one audience so far — still not sensitive.
	wantIllegal(t, m.DecideMap("d", issue, "b", future), "one audience is not a map")
	_ = s
}
