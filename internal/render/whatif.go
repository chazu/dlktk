package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// Hypothetical is one speculative move applied to an in-memory copy of the
// graph — nothing is ever written. Exploration must be cheaper than polluting
// an append-only record with moves that then need conceding.
type Hypothetical struct {
	Kind    string `json:"kind"` // "object" | "prefer" | "without"
	Target  string `json:"target,omitempty"`
	Winner  string `json:"winner,omitempty"`
	Loser   string `json:"loser,omitempty"`
	Node    string `json:"node,omitempty"`
	Summary string `json:"summary"`
}

// HypObject is a hypothetical undefeated objection against target.
func HypObject(target string) Hypothetical {
	return Hypothetical{Kind: "object", Target: target, Summary: "object " + target}
}

// HypPrefer is a hypothetical preference of winner over loser.
func HypPrefer(winner, loser string) Hypothetical {
	return Hypothetical{Kind: "prefer", Winner: winner, Loser: loser, Summary: fmt.Sprintf("prefer %s>%s", winner, loser)}
}

// HypWithout removes a node (and its incident links) — a simulated concede.
func HypWithout(node string) Hypothetical {
	return Hypothetical{Kind: "without", Node: node, Summary: "without " + node}
}

// WhatIfView is the counterfactual read: which labels the hypothetical moves
// would flip, and the issue's resulting status.
type WhatIfView struct {
	Issue         string         `json:"issue"`
	Hypotheticals []Hypothetical `json:"hypotheticals"`
	Flipped       []LabelChange  `json:"flipped"`
	Result        IssueStatus    `json:"result"`
}

// WhatIf applies hypothetical moves to a copy of the graph and reports the
// label diff against the real labelling, plus the issue's resulting status.
func WhatIf(g *ibis.Graph, labels map[string]af.Label, issue string, hyps []Hypothetical) (WhatIfView, error) {
	v := WhatIfView{Issue: issue, Hypotheticals: hyps}
	cp := copyGraph(g)
	for i, h := range hyps {
		if err := applyHypothetical(cp, h, i); err != nil {
			return WhatIfView{}, err
		}
	}
	fw, err := af.Build(cp)
	if err != nil {
		return WhatIfView{}, err
	}
	after := fw.Grounded()
	v.Flipped = flips(g, labels, after)
	v.Result = Status(cp, fw, after, issue, nil)
	return v, nil
}

// CruxEntry is one load-bearing argument: removing it changes which positions
// stand.
type CruxEntry struct {
	Node   string        `json:"node"`
	Text   string        `json:"text"`
	Author string        `json:"author,omitempty"`
	Flips  []LabelChange `json:"flips"`
}

// CruxView identifies the arguments the issue's verdict actually rests on —
// where a novel objection or reinforcement has maximum leverage.
type CruxView struct {
	Issue  string      `json:"issue"`
	Cruxes []CruxEntry `json:"cruxes"`
	Note   string      `json:"note"`
}

const cruxNote = "single-node analysis: jointly load-bearing sets (e.g. two redundant arguments, neither individually pivotal) are not detected"

// Crux recomputes the issue's labelling without each argument in its scope,
// one at a time, and reports the arguments whose absence flips any position.
func Crux(g *ibis.Graph, labels map[string]af.Label, issue string) (CruxView, error) {
	v := CruxView{Issue: issue, Note: cruxNote}
	scope := reachableAF(g, issue)
	var args []string
	for id := range scope {
		if n, ok := g.Nodes[id]; ok && n.Kind == ibis.Argument {
			args = append(args, id)
		}
	}
	sort.Strings(args)

	positions := map[string]bool{}
	for _, p := range positionsFor(g, issue) {
		positions[p] = true
	}

	for _, a := range args {
		cp := copyGraph(g)
		removeNode(cp, a)
		fw, err := af.Build(cp)
		if err != nil {
			return CruxView{}, err
		}
		after := fw.Grounded()
		var posFlips []LabelChange
		for _, f := range flips(g, labels, after) {
			if positions[f.Node] {
				posFlips = append(posFlips, f)
			}
		}
		if len(posFlips) > 0 {
			n := g.Nodes[a]
			v.Cruxes = append(v.Cruxes, CruxEntry{Node: a, Text: n.Text, Author: n.Author, Flips: posFlips})
		}
	}
	sort.SliceStable(v.Cruxes, func(i, j int) bool {
		if len(v.Cruxes[i].Flips) != len(v.Cruxes[j].Flips) {
			return len(v.Cruxes[i].Flips) > len(v.Cruxes[j].Flips)
		}
		return v.Cruxes[i].Node < v.Cruxes[j].Node
	})
	return v, nil
}

// applyHypothetical mutates the graph copy. Synthetic objection nodes are
// arguments (anything else would silently vanish from the framework) with ids
// no proquint can collide with.
func applyHypothetical(cp *ibis.Graph, h Hypothetical, i int) error {
	switch h.Kind {
	case "object":
		if err := cp.CanAttach(h.Target, ibis.ObjectsTo); err != nil {
			return err
		}
		hid := fmt.Sprintf("~h%d", i+1)
		cp.Nodes[hid] = ibis.Node{ID: hid, Kind: ibis.Argument, Text: "(hypothetical objection)"}
		cp.Links = append(cp.Links, ibis.Link{ID: hid + "-link", Src: hid, Dst: h.Target, Rel: ibis.ObjectsTo})
	case "prefer":
		if err := cp.CanPrefer(h.Winner, h.Loser); err != nil {
			return err
		}
		cp.Preferences = append(cp.Preferences, ibis.Preference{Winner: h.Winner, Loser: h.Loser, Basis: "(hypothetical)"})
	case "without":
		if _, ok := cp.Nodes[h.Node]; !ok {
			return &ibis.NotFound{Node: h.Node, Detail: fmt.Sprintf("whatif --without node %q not found", h.Node)}
		}
		removeNode(cp, h.Node)
	default:
		return &ibis.IllegalMove{Detail: fmt.Sprintf("unknown hypothetical kind %q", h.Kind)}
	}
	return nil
}

// removeNode simulates a concede: the node and its incident links go; stored
// preferences stay (as they do after a real concede) — harmless, since attacks
// only ever connect live nodes.
func removeNode(cp *ibis.Graph, node string) {
	delete(cp.Nodes, node)
	delete(cp.Values, node)
	links := cp.Links[:0]
	for _, l := range cp.Links {
		if l.Src != node && l.Dst != node {
			links = append(links, l)
		}
	}
	cp.Links = links
}

// flips lists nodes present in both labellings whose label changed, sorted.
func flips(g *ibis.Graph, before, after map[string]af.Label) []LabelChange {
	var out []LabelChange
	for id, b := range before {
		a, ok := after[id]
		if !ok {
			continue
		}
		if a != b {
			out = append(out, LabelChange{Node: id, Text: g.Nodes[id].Text, From: string(b), To: string(a)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// copyGraph deep-copies the mutable parts of a graph.
func copyGraph(g *ibis.Graph) *ibis.Graph {
	cp := &ibis.Graph{
		Nodes:       make(map[string]ibis.Node, len(g.Nodes)),
		Links:       append([]ibis.Link{}, g.Links...),
		Preferences: append([]ibis.Preference{}, g.Preferences...),
		IssueCards:  make(map[string]ibis.Cardinality, len(g.IssueCards)),
		Reframes:    append([]ibis.Reframe{}, g.Reframes...),
		Values:      make(map[string]string, len(g.Values)),
		Audiences:   make(map[string]ibis.Audience, len(g.Audiences)),
	}
	for k, v := range g.Nodes {
		cp.Nodes[k] = v
	}
	for k, v := range g.IssueCards {
		cp.IssueCards[k] = v
	}
	for k, v := range g.Values {
		cp.Values[k] = v
	}
	for k, v := range g.Audiences {
		cp.Audiences[k] = v
	}
	return cp
}

// WhatIfText renders a WhatIfView.
func WhatIfText(v WhatIfView) string {
	var b strings.Builder
	b.WriteString(cBold("what if"))
	for _, h := range v.Hypotheticals {
		b.WriteString(" " + cID(h.Summary))
	}
	b.WriteString("  " + cDim("(nothing written)") + "\n")
	if len(v.Flipped) == 0 {
		b.WriteString("  " + cDim("no labels change") + "\n")
	}
	for _, c := range v.Flipped {
		b.WriteString(para(fmt.Sprintf("  %s %s  %s → %s  ", cDim("~"), cID(c.Node), labelInline(c.From), labelInline(c.To)), quote(c.Text)) + "\n")
	}
	b.WriteString("resulting " + StatusText(v.Result))
	return b.String()
}

// CruxText renders a CruxView.
func CruxText(v CruxView) string {
	var b strings.Builder
	b.WriteString(cBold("crux of "+v.Issue) + cDim(" — arguments the verdict rests on:") + "\n")
	if len(v.Cruxes) == 0 {
		b.WriteString("  " + cDim("none — no single argument's removal flips a position") + "\n")
	}
	for _, c := range v.Cruxes {
		prefix := fmt.Sprintf("  %s  ", cID(ibis.PrefixFor(ibis.Argument)+c.Node))
		b.WriteString(para(prefix, quote(c.Text)) + "\n")
		for _, f := range c.Flips {
			fmt.Fprintf(&b, "      %s %s  %s → %s\n", cDim("flips"), pid(f.Node), labelInline(f.From), labelInline(f.To))
		}
	}
	b.WriteString(cDim("note: "+v.Note) + "\n")
	return b.String()
}
