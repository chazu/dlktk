package check

import (
	"testing"

	"github.com/chazu/dlktk/internal/proto"
)

// A preference recorded on an issue with two positions from a single author is
// premature — the "no prefer until real divergence" rule, made checkable.
func TestPrematurePreferenceSingleAuthor(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	if _, _, err := m.Prefer("d", a, b, "gut"); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, PrematurePreference) != 1 {
		t.Fatalf("want premature_preference for a one-author issue: %+v", v)
	}
}

// A preference is not premature once two authors have each staked a position.
func TestPreferenceNotPrematureWithTwoAuthors(t *testing.T) {
	s, _ := open(t)
	malice := proto.New(s, "alice", "")
	mbob := proto.New(s, "bob", "")
	issue, _ := malice.Raise("d", "q?", "", "", "")
	a, _ := malice.Propose("d", issue, "a", "")
	b, _ := mbob.Propose("d", issue, "b", "")
	if _, _, err := malice.Prefer("d", a, b, "on the merits"); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, PrematurePreference) != 0 {
		t.Fatalf("two positions from two authors is not premature: %+v", v)
	}
}

// Deciding an issue over a live warning without naming it in the basis draws
// unacknowledged_warning; naming it clears the finding.
func TestUnacknowledgedWarning(t *testing.T) {
	build := func(basis string) View {
		s, m := open(t)
		issue, _ := m.Raise("d", "q?", "", "", "")
		pos, _ := m.Propose("d", issue, "yes", "")
		if err := m.Decide("d", issue, pos, basis, 0); err != nil {
			t.Fatal(err)
		}
		return run(t, s)
	}

	// Basis says nothing about the untested_decision it closed over.
	over := build("team consensus")
	if findKind(over, UntestedDecision) != 1 {
		t.Fatalf("fixture must produce an untested_decision: %+v", over)
	}
	if findKind(over, UnacknowledgedWarning) != 1 {
		t.Fatalf("closing over an unacknowledged warning must be flagged: %+v", over)
	}

	// Basis names the warning: obligation discharged.
	ack := build("untested but the risk is acceptable here")
	if findKind(ack, UnacknowledgedWarning) != 0 {
		t.Fatalf("acknowledging the warning in the basis must clear it: %+v", ack)
	}
}
