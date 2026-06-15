package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// LinkView is one link incident to the shown node, with the peer's text inlined
// so a reader (human or agent) needs no second lookup.
type LinkView struct {
	Rel       string `json:"rel"`
	Dir       string `json:"dir"` // "out": node -> peer; "in": peer -> node
	Peer      string `json:"peer"`
	PeerKind  string `json:"peer_kind"`
	PeerText  string `json:"peer_text"`
	PeerLabel string `json:"peer_label,omitempty"` // AF peers only
}

// NodeView is the `show` envelope: one node in full, with every incident link
// (design §6.2).
type NodeView struct {
	ID      string       `json:"id"`
	Kind    string       `json:"kind"`
	Text    string       `json:"text"`
	Author  string       `json:"author,omitempty"`
	Label   string       `json:"label,omitempty"` // AF nodes only
	Links   []LinkView   `json:"links"`
	Decided *DecidedView `json:"decided,omitempty"` // issues with a standing decision
}

// Show builds the full view of one node.
func Show(g *ibis.Graph, labels map[string]af.Label, node string, decs []ibis.Decision) NodeView {
	n := g.Nodes[node]
	v := NodeView{ID: n.ID, Kind: string(n.Kind), Text: n.Text, Author: n.Author}
	if g.IsAFNode(node) {
		v.Label = string(labels[node])
	}
	for _, l := range g.Links {
		switch node {
		case l.Src:
			v.Links = append(v.Links, linkView(g, labels, l.Rel, "out", l.Dst))
		case l.Dst:
			v.Links = append(v.Links, linkView(g, labels, l.Rel, "in", l.Src))
		}
	}
	sort.Slice(v.Links, func(i, j int) bool {
		a, b := v.Links[i], v.Links[j]
		if a.Dir != b.Dir {
			return a.Dir < b.Dir // "in" before "out"
		}
		if a.Rel != b.Rel {
			return a.Rel < b.Rel
		}
		return a.Peer < b.Peer
	})
	if n.Kind == ibis.Issue {
		for _, d := range decs {
			if d.Issue == node {
				v.Decided = &DecidedView{Position: d.Position, Basis: d.Basis, Decider: d.Decider, Override: d.Override, Supersedes: d.Supersedes}
			}
		}
	}
	return v
}

func linkView(g *ibis.Graph, labels map[string]af.Label, rel ibis.Rel, dir, peer string) LinkView {
	p := g.Nodes[peer]
	lv := LinkView{Rel: string(rel), Dir: dir, Peer: peer, PeerKind: string(p.Kind), PeerText: p.Text}
	if g.IsAFNode(peer) {
		lv.PeerLabel = string(labels[peer])
	}
	return lv
}

// ShowText renders a NodeView as human text.
func ShowText(v NodeView) string {
	var b strings.Builder
	header := []string{}
	if v.Label != "" {
		header = append(header, labelInline(v.Label))
	}
	header = append(header, nid(v.Kind, v.ID))
	if v.Author != "" {
		header = append(header, cDim("by "+v.Author))
	}
	b.WriteString(strings.Join(header, "  ") + "\n")
	b.WriteString(para("  ", quote(v.Text)) + "\n")
	for _, l := range v.Links {
		arrow := cDim("←")
		if l.Dir == "out" {
			arrow = cDim("→")
		}
		peerLabel := ""
		if l.PeerLabel != "" {
			peerLabel = labelInline(l.PeerLabel) + " "
		}
		prefix := fmt.Sprintf("  %s %s %s%s  ", arrow, cDim(fmt.Sprintf("%-11s", l.Rel)), peerLabel, nid(l.PeerKind, l.Peer))
		b.WriteString(para(prefix, quote(l.PeerText)) + "\n")
	}
	if d := v.Decided; d != nil {
		flag := ""
		if d.Override {
			flag += cDim(" (OVERRIDE)")
		}
		if d.Supersedes != "" {
			flag += cDim(fmt.Sprintf(" (supersedes %s)", d.Supersedes))
		}
		fmt.Fprintf(&b, "  %s %s  %s%s\n", labelColor("IN", "✓ decided:"), pid(d.Position), cDim("by "+d.Decider), flag)
	}
	return b.String()
}
