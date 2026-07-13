package render

import (
	"testing"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// show renders reframe lineage from both sides: reframed_to on the dead framing
// and reframed_from on the new one — the side that was invisible before
// (wicked-problems-2.md item 10).
func TestShowRendersReframeLineageBothWays(t *testing.T) {
	nodes := []ibis.Node{
		{ID: "I1", Kind: ibis.Issue, Text: "old framing"},
		{ID: "I2", Kind: ibis.Issue, Text: "new framing"},
	}
	reframes := []ibis.Reframe{{Old: "I1", New: "I2", Basis: "false dichotomy", Author: "a"}}
	g := ibis.NewGraph(nodes, nil, nil, nil, reframes, nil, nil)
	labels := map[string]af.Label{}

	has := func(node, rel, peer string) bool {
		for _, l := range Show(g, labels, node, nil).Links {
			if l.Rel == rel && l.Peer == peer {
				return true
			}
		}
		return false
	}

	if !has("I1", "reframed_to", "I2") {
		t.Fatalf("old framing must show reframed_to → new: %+v", Show(g, labels, "I1", nil).Links)
	}
	if !has("I2", "reframed_from", "I1") {
		t.Fatalf("new framing must show reframed_from ← old: %+v", Show(g, labels, "I2", nil).Links)
	}
}
