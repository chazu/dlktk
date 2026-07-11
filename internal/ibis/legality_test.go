package ibis

import (
	"errors"
	"testing"
)

func testGraph() *Graph {
	nodes := []Node{
		{ID: "I", Kind: Issue, Author: "alice"},
		{ID: "A", Kind: Position, Author: "alice"},
		{ID: "B", Kind: Position, Author: "bob"},
		{ID: "C", Kind: Argument, Author: "carol"},
	}
	links := []Link{
		{ID: "l1", Src: "A", Dst: "I", Rel: RespondsTo},
		{ID: "l2", Src: "B", Dst: "I", Rel: RespondsTo},
		{ID: "l3", Src: "C", Dst: "B", Rel: ObjectsTo},
	}
	prefs := []Preference{{ID: "p1", Winner: "A", Loser: "B"}}
	return NewGraph(nodes, links, prefs, []IssueCard{{Issue: "I", Cardinality: SelectOne}}, nil, nil, nil)
}

// The discover contract reserves exit 3 for "a referenced id does not exist"
// and 2 for ill-formed moves against nodes that do exist. Legality must return
// the matching error type for each.
func TestLegalityErrorKinds(t *testing.T) {
	g := testGraph()
	cases := []struct {
		name     string
		err      error
		notFound bool // else IllegalMove
	}{
		{"raise missing parent", g.CanRaise("nope", ""), true},
		{"raise non-issue parent", g.CanRaise("A", ""), false},
		{"propose missing issue", g.CanPropose("nope"), true},
		{"propose at position", g.CanPropose("A"), false},
		{"attach missing target", g.CanAttach("nope", ObjectsTo), true},
		{"attach at issue", g.CanAttach("I", ObjectsTo), false},
		{"prefer missing endpoint", g.CanPrefer("A", "nope"), true},
		{"prefer issue endpoint", g.CanPrefer("A", "I"), false},
		{"prefer self", g.CanPrefer("A", "A"), false},
		{"prefer closing a cycle", g.CanPrefer("B", "A"), false},
		{"decide missing issue", g.CanDecide("nope", "A"), true},
		{"decide missing position", g.CanDecide("I", "nope"), true},
		{"decide non-responding position", g.CanDecide("I", "C"), false},
		{"concede missing node", g.CanConcede("nope", "alice"), true},
		{"concede unowned node", g.CanConcede("B", "alice"), false},
	}
	for _, c := range cases {
		if c.err == nil {
			t.Errorf("%s: want error, got nil", c.name)
			continue
		}
		var nf *NotFound
		var im *IllegalMove
		switch {
		case c.notFound && !errors.As(c.err, &nf):
			t.Errorf("%s: want NotFound, got %T (%v)", c.name, c.err, c.err)
		case !c.notFound && !errors.As(c.err, &im):
			t.Errorf("%s: want IllegalMove, got %T (%v)", c.name, c.err, c.err)
		}
	}
}

func TestLegalityAccepts(t *testing.T) {
	g := testGraph()
	for name, err := range map[string]error{
		"raise root":      g.CanRaise("", ""),
		"raise child":     g.CanRaise("I", ""),
		"propose":         g.CanPropose("I"),
		"object position": g.CanAttach("B", ObjectsTo),
		"object argument": g.CanAttach("C", ObjectsTo),
		"prefer":          g.CanPrefer("C", "B"),
		"decide":          g.CanDecide("I", "A"),
		"concede own":     g.CanConcede("A", "alice"),
	} {
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
	}
}
