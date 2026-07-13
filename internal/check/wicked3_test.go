package check

// Arc two, items 3-4 (wicked-problems-2.md): self_elevated_synthesis and
// bundle_synthesis strict findings, with the item-3 suppression rule.

import (
	"testing"

	"github.com/chazu/dlktk/internal/proto"
)

// A hybrid preferred over a parent whose objection it never answered draws
// self_elevated_synthesis; a cross-author address clears it.
func TestSelfElevatedSynthesisFinding(t *testing.T) {
	s, m := open(t) // author "x"
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	bob := proto.New(s, "bob", "")
	obj, _ := bob.Object("d", a, "a is costly", "", "")
	h, _, err := m.Synthesize("d", issue, "hybrid", []string{a, b}, "", []string{"drops b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Prefer("d", h, a, "subsumption"); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), SelfElevatedSynthesis) != 1 {
		t.Fatal("want self_elevated_synthesis: parent's objection is unanswered on the hybrid")
	}

	// A cross-author address clears it.
	if _, err := bob.Support("d", h, "the hybrid escapes it", "", obj); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), SelfElevatedSynthesis) != 0 {
		t.Fatal("a cross-author address must clear self_elevated_synthesis")
	}
}

// A self-authored address does not clear the finding — the one mind cannot
// answer its own hybrid's inherited critics.
func TestSelfElevatedSynthesisResistsSelfAddress(t *testing.T) {
	s, m := open(t) // author "x"
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	bob := proto.New(s, "bob", "")
	obj, _ := bob.Object("d", a, "a is costly", "", "")
	h, _, _ := m.Synthesize("d", issue, "hybrid", []string{a, b}, "", []string{"drops b"})
	if _, err := m.Support("d", h, "n/a", "", obj); err != nil { // x owns h and the answer
		t.Fatal(err)
	}
	if _, _, err := m.Prefer("d", h, a, "subsumption"); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), SelfElevatedSynthesis) != 1 {
		t.Fatal("a self-authored address must not clear self_elevated_synthesis")
	}
}

// One defect, one finding: when untested_decision fires on the hybrid,
// self_elevated_synthesis on the same node is suppressed.
func TestSelfElevatedSuppressedByUntested(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	bob := proto.New(s, "bob", "")
	if _, err := bob.Object("d", a, "a is costly", "", ""); err != nil {
		t.Fatal(err)
	}
	h, _, _ := m.Synthesize("d", issue, "hybrid", []string{a, b}, "", []string{"drops b"})
	// Elevate over both parents so the hybrid is sole IN, never itself objected
	// to -> untested; its inherited question stays open -> self_elevated candidate.
	if _, _, err := m.Prefer("d", h, a, "subsumption"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Prefer("d", h, b, "subsumption"); err != nil {
		t.Fatal(err)
	}
	if err := m.Decide("d", issue, h, "call", 0); err != nil {
		t.Fatal(err)
	}
	v := run(t, s)
	if findKind(v, UntestedDecision) != 1 {
		t.Fatalf("untested_decision must fire: %+v", v)
	}
	if findKind(v, SelfElevatedSynthesis) != 0 {
		t.Fatalf("self_elevated_synthesis must be suppressed on the untested node: %+v", v)
	}
}

// A decided synthesis of three or more parents with no recorded drops draws
// bundle_synthesis; recording drops clears it; two parents never trigger it.
func TestBundleSynthesisFinding(t *testing.T) {
	build := func(t *testing.T, parents int, drops []string) View {
		s, m := open(t)
		issue, _ := m.Raise("d", "q?", "", "", "")
		var froms []string
		for i := 0; i < parents; i++ {
			p, _ := m.Propose("d", issue, string(rune('a'+i)), "")
			froms = append(froms, p)
		}
		h, _, err := m.Synthesize("d", issue, "hybrid", froms, "", drops)
		if err != nil {
			t.Fatal(err)
		}
		// Concede the parents so the hybrid is the sole IN position.
		for _, p := range froms {
			if err := m.Concede("d", p); err != nil {
				t.Fatal(err)
			}
		}
		if err := m.Decide("d", issue, h, "sole survivor", 0); err != nil {
			t.Fatal(err)
		}
		return run(t, s)
	}

	if findKind(build(t, 3, nil), BundleSynthesis) != 1 {
		t.Fatal("3 parents, no drops, decided -> want bundle_synthesis")
	}
	if findKind(build(t, 3, []string{"drops the second half"}), BundleSynthesis) != 0 {
		t.Fatal("recorded drops must clear bundle_synthesis")
	}
	if findKind(build(t, 2, nil), BundleSynthesis) != 0 {
		t.Fatal("two-parent syntheses must stay friction-free")
	}
}

// bundle_synthesis is a property of a decided synthesis: an undecided bundle
// does not fire it.
func TestBundleSynthesisOnlyWhenDecided(t *testing.T) {
	s, m := open(t)
	issue, _ := m.Raise("d", "q?", "", "", "")
	a, _ := m.Propose("d", issue, "a", "")
	b, _ := m.Propose("d", issue, "b", "")
	c, _ := m.Propose("d", issue, "c", "")
	if _, _, err := m.Synthesize("d", issue, "hybrid", []string{a, b, c}, "", nil); err != nil {
		t.Fatal(err)
	}
	if findKind(run(t, s), BundleSynthesis) != 0 {
		t.Fatal("an undecided bundle must not fire bundle_synthesis")
	}
}
