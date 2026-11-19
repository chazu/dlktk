package check_test

import (
	"testing"
	"time"

	"github.com/chazu/dlktk/internal/check"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/store"
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
	if findKind(v, check.PrematurePreference) != 1 {
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
	if findKind(v, check.PrematurePreference) != 0 {
		t.Fatalf("two positions from two authors is not premature: %+v", v)
	}
}

// Only an explicit acknowledgment clears the missing-acknowledgment finding.
func TestUnacknowledgedWarning(t *testing.T) {
	build := func(basis string, ack ...string) check.View {
		s, m := open(t)
		issue, _ := m.Raise("d", "q?", "", "", "")
		pos, _ := m.Propose("d", issue, "yes", "")
		if err := m.Decide("d", issue, pos, basis, 0, ack...); err != nil {
			t.Fatal(err)
		}
		return run(t, s)
	}

	// Basis says nothing about the untested_decision it closed over.
	over := build("team consensus")
	if findKind(over, check.UntestedDecision) != 1 {
		t.Fatalf("fixture must produce an untested_decision: %+v", over)
	}
	if findKind(over, check.UnacknowledgedWarning) != 1 {
		t.Fatalf("closing over an unacknowledged warning must be flagged: %+v", over)
	}

	// The decision names the warning and explains the exception.
	ack := build("the release date takes precedence", check.UntestedDecision)
	if findKind(ack, check.UnacknowledgedWarning) != 0 {
		t.Fatalf("acknowledging the warning in the basis must clear it: %+v", ack)
	}
}

func TestAcknowledgmentsStayWithTheDecision(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "which practices?", "", "", "open")
	a, _ := m.Propose("d", issue, "A", "")
	b, _ := m.Propose("d", issue, "B", "")
	if err := m.Decide("d", issue, a, "release deadline", 0, check.UntestedDecision); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, b, "accepted", 0); err != nil {
		t.Fatal(err)
	}
	verify := func(v check.View, missing string) {
		t.Helper()
		if findKind(v, check.UntestedDecision) != 2 || findKind(v, check.UnacknowledgedWarning) != 1 {
			t.Fatalf("unexpected findings: %+v", v)
		}
		for _, f := range v.Findings {
			if f.Kind == check.UnacknowledgedWarning && f.Node != missing {
				t.Fatalf("wrong decision: %+v", f)
			}
		}
	}
	verify(run(t, s), b)
	if err := m.Supersede("d", issue, b, "wrong finding", 0, check.ReviewDue); err == nil {
		t.Fatal("accepted a future warning")
	}
	verify(run(t, s), b)
	if err := m.Supersede("d", issue, a, "revised trial", 0, check.UntestedDecision); err != nil {
		t.Fatal(err)
	}
	for _, export := range []func(string) ([]store.ExportRecord, error){s.Export, s.ExportHistory} {
		dst, err := store.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { dst.Close() })
		rows, err := export("d")
		if err != nil {
			t.Fatal(err)
		}
		for round := 0; round < 2; round++ {
			if _, err := dst.ImportAll(rows); err != nil {
				t.Fatal(err)
			}
			verify(run(t, dst), b)
		}
	}
	if err := m.Supersede("d", issue, b, "short-term exception", time.Now().Unix()+3600, check.UntestedDecision); err != nil {
		t.Fatal(err)
	}
	v, err := check.Run(s, []string{"d"}, store.Now(), time.Now().Unix()+7200)
	if err != nil || findKind(v, check.ReviewDue) != 1 || findKind(v, check.UnacknowledgedWarning) != 0 {
		t.Fatalf("later warning attributed to closure: %+v, %v", v, err)
	}
}
