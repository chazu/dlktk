package proto

import (
	"testing"
	"time"

	"github.com/chazu/dlktk/internal/store"
)

// --- reframe ---

func TestReframeRecordsLineageAndBasis(t *testing.T) {
	s, m, issue, _, _ := fixture(t)

	if _, err := m.Reframe("disc-1", issue, "which concurrency strategy?", "", ""); err == nil {
		t.Fatal("reframe without basis must be rejected")
	}
	newIssue, err := m.Reframe("disc-1", issue, "which concurrency strategy?", "lock choice presupposes shared state", "")
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.Graph("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if to, ok := g.ReframedTo(issue); !ok || to != newIssue {
		t.Fatalf("lineage not recorded: %v %v", to, ok)
	}
	// Positions do not carry over.
	if len(g.Links) != 2 { // the two original responds_to only
		t.Fatalf("reframe must not copy links: %v", g.Links)
	}
	// The dead framing cannot be reframed twice.
	wantIllegal(t, errOf(m.Reframe("disc-1", issue, "again?", "b", "")), "double reframe")
}

func TestReframeRejectsDecidedIssue(t *testing.T) {
	_, m, issue, posA, _ := fixture(t)
	if err := m.Decide("disc-1", issue, posA, "call", 0); err != nil {
		t.Fatal(err)
	}
	wantIllegal(t, errOf(m.Reframe("disc-1", issue, "new framing", "basis", "")),
		"reframing a decided issue must be illegal (supersede first)")
}

// errOf drops the id a two-value move returns, for wantIllegal.
func errOf(_ string, err error) error { return err }

// --- raise --from ---

func TestRaiseFromRecordsProvenance(t *testing.T) {
	s, m, _, posA, _ := fixture(t)
	sub, err := m.Raise("disc-1", "is shared state even needed?", "", posA, "")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := s.Graph("disc-1", store.Now())
	found := false
	for _, l := range g.Links {
		if l.Src == sub && l.Dst == posA && string(l.Rel) == "raised_from" {
			found = true
		}
	}
	if !found {
		t.Fatalf("raised_from link missing: %v", g.Links)
	}
	// parent and from are mutually exclusive.
	wantIllegal(t, errOf(m.Raise("disc-1", "x", sub, posA, "")), "--parent with --from")
}

// --- synthesize ---

func TestSynthesizeLineageAndLegality(t *testing.T) {
	s, m, issue, posA, posB := fixture(t)

	wantIllegal(t, errOf(m.Synthesize("disc-1", issue, "hybrid", []string{posA}, "")), "one parent")
	wantIllegal(t, errOf(m.Synthesize("disc-1", issue, "hybrid", []string{posA, posA}, "")), "duplicate parent")

	other, err := m.Raise("disc-1", "other issue?", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := m.Propose("disc-1", other, "foreign", "")
	if err != nil {
		t.Fatal(err)
	}
	wantIllegal(t, errOf(m.Synthesize("disc-1", issue, "hybrid", []string{posA, foreign}, "")), "parent from another issue")

	hybrid, err := m.Synthesize("disc-1", issue, "mutex for writes, rwlock reads", []string{posA, posB}, "")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := s.Graph("disc-1", store.Now())
	links := 0
	for _, l := range g.Links {
		if l.Src == hybrid && string(l.Rel) == "synthesizes" {
			links++
		}
	}
	if links != 2 {
		t.Fatalf("want 2 synthesizes links, got %d", links)
	}
}

// --- assume ---

func TestAssumeTagsTheArgument(t *testing.T) {
	s, m, _, posA, _ := fixture(t)
	a, err := m.Assume("disc-1", posA, "the workload stays read-heavy")
	if err != nil {
		t.Fatal(err)
	}
	g, _ := s.Graph("disc-1", store.Now())
	if g.Nodes[a].Tag != "assumption" {
		t.Fatalf("assumption tag missing: %+v", g.Nodes[a])
	}
}

// --- promote / audience ---

func TestPromoteOwnershipAndImmutability(t *testing.T) {
	s, m, _, posA, _ := fixture(t)

	stranger := New(s, "stranger", "")
	wantIllegal(t, stranger.Promote("disc-1", posA, "velocity"), "promote by non-owner")

	if err := m.Promote("disc-1", posA, "velocity"); err != nil {
		t.Fatal(err)
	}
	wantIllegal(t, m.Promote("disc-1", posA, "security"), "re-promote")

	// Concede-and-restate is the change-a-value path; the dangling tag is
	// ignored on load.
	if err := m.Concede("disc-1", posA); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Graph("disc-1", store.Now())
	if _, ok := g.Values[posA]; ok {
		t.Fatalf("dangling value tag must be ignored: %v", g.Values)
	}
}

func TestAudienceDeclareAndSupersede(t *testing.T) {
	s, m, _, _, _ := fixture(t)

	wantIllegal(t, m.DeclareAudience("disc-1", "ops", []string{"security"}, false, ""), "one-value ranking")
	wantIllegal(t, m.DeclareAudience("disc-1", "ops", []string{"security", "security"}, false, ""), "duplicate value")

	if err := m.DeclareAudience("disc-1", "ops", []string{"security", "velocity"}, false, ""); err != nil {
		t.Fatal(err)
	}
	wantIllegal(t, m.DeclareAudience("disc-1", "ops", []string{"velocity", "security"}, false, ""), "silent re-declare")
	wantIllegal(t, m.DeclareAudience("disc-1", "ops", []string{"velocity", "security"}, true, ""), "supersede without basis")

	if err := m.DeclareAudience("disc-1", "ops", []string{"velocity", "security"}, true, "priorities changed post-launch"); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Graph("disc-1", store.Now())
	if got := g.Audiences["ops"].Ranking[0]; got != "velocity" {
		t.Fatalf("superseding declaration not in force: %v", g.Audiences["ops"])
	}
	auds, err := s.Audiences("disc-1", store.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(auds) != 1 {
		t.Fatalf("retired audience still current: %+v", auds)
	}
}

// --- review-by ---

func TestReviewByMustBeFuture(t *testing.T) {
	_, m, issue, posA, _ := fixture(t)
	past := time.Now().Add(-time.Hour).Unix()
	wantIllegal(t, m.Decide("disc-1", issue, posA, "call", past), "review-by in the past")

	future := time.Now().Add(time.Hour).Unix()
	if err := m.Decide("disc-1", issue, posA, "call", future); err != nil {
		t.Fatal(err)
	}
}
