package main

import (
	"testing"

	"github.com/chazu/dlktk/internal/ibis"
)

// targetIssues must return issues in canonical (sorted) order regardless of map
// iteration order — design §8.1 promises byte-identical output for same inputs.
func TestTargetIssuesDeterministic(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "zz-issue", Kind: ibis.Issue},
		{ID: "aa-issue", Kind: ibis.Issue},
		{ID: "mm-issue", Kind: ibis.Issue},
		{ID: "pp-position", Kind: ibis.Position},
	}
	want := []string{"aa-issue", "mm-issue", "zz-issue"}
	for i := 0; i < 50; i++ { // map order varies run to run; hammer it
		g := ibis.NewGraph(nodes, nil, nil, nil)
		got, err := targetIssues(g, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("iteration %d: got %v, want %v", i, got, want)
			}
		}
	}
}
