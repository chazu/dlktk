package check

import (
	"testing"

	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/store"
)

// decidedSynthesis builds an issue with two rival positions and a synthesis of
// them authored by alice, objected to by `objector`. It returns the synthesis
// id; the caller decides it under whichever author it wants to test.
func decidedSynthesis(t *testing.T, s *store.Store, objector string) (malice, mbob *proto.Mover, issue, hybrid string) {
	t.Helper()
	malice = proto.New(s, "alice", "")
	mbob = proto.New(s, "bob", "")
	issue, _ = malice.Raise("d", "which?", "", "", "")
	a, _ := malice.Propose("d", issue, "A", "")
	b, _ := mbob.Propose("d", issue, "B", "")
	hybrid, _, err := malice.Synthesize("d", issue, "both", []string{a, b}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	obj := malice
	if objector == "bob" {
		obj = mbob
	}
	if _, err := obj.Object("d", hybrid, "too costly", "", ""); err != nil {
		t.Fatal(err)
	}
	return malice, mbob, issue, hybrid
}

// single_author_convergence fires when the decider of a synthesis shares its
// author, even though the synthesis was objected to across authors.
func TestSingleAuthorConvergenceDeciderSharesAuthor(t *testing.T) {
	s, _ := open(t)
	malice, _, issue, hybrid := decidedSynthesis(t, s, "bob")
	if err := malice.Decide("d", issue, hybrid, "ours", 0); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, SingleAuthorConvergence) != 1 {
		t.Fatalf("want single_author_convergence when the synthesis author decides: %+v", v)
	}
}

// It does not fire when a different author decides a synthesis that faced a
// genuine cross-author objection.
func TestSingleAuthorConvergenceCleanSeparation(t *testing.T) {
	s, _ := open(t)
	_, mbob, issue, hybrid := decidedSynthesis(t, s, "bob")
	if err := mbob.Decide("d", issue, hybrid, "independent", 0); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, SingleAuthorConvergence) != 0 {
		t.Fatalf("distinct decider + cross-author objection must not fire: %+v", v)
	}
}

// It fires when every objection against the synthesis shares its author, even
// if a different author records the decision.
func TestSingleAuthorConvergenceSelfObjectionsOnly(t *testing.T) {
	s, _ := open(t)
	_, mbob, issue, hybrid := decidedSynthesis(t, s, "alice") // objector == synthesis author
	if err := mbob.Decide("d", issue, hybrid, "rubber stamp", 0); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, SingleAuthorConvergence) != 1 {
		t.Fatalf("want single_author_convergence when every objector is the synthesis author: %+v", v)
	}
}
