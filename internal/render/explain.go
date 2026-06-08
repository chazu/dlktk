package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// MoveSuggestion is a legal, useful next move in {move,args,effect} shape
// (shared by `why` to_flip and `moves`).
type MoveSuggestion struct {
	Move   string   `json:"move"`
	Args   []string `json:"args"`
	Effect string   `json:"effect"`
}

// Because explains one contributor to a node's label.
type Because struct {
	Attacker      string `json:"attacker"`
	AttackerLabel string `json:"attacker_label"`
	Reason        string `json:"reason"`
}

// WhyView is the explanation envelope for one node (design §8.3).
type WhyView struct {
	Node    string           `json:"node"`
	Label   string           `json:"label"`
	Because []Because        `json:"because"`
	ToFlip  []MoveSuggestion `json:"to_flip"`
}

// MovesView is the legal-move list for an issue.
type MovesView struct {
	Issue string           `json:"issue"`
	Moves []MoveSuggestion `json:"moves"`
}

// NodeRef is a minimal node reference for the agenda.
type NodeRef struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Text  string `json:"text"`
	Label string `json:"label"`
}

// AgendaView lists the genuinely-contested (UNDEC) AF nodes — the live agenda.
type AgendaView struct {
	Undecided []NodeRef `json:"undecided"`
}

// Why builds the explanation for a node.
func Why(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, node string) WhyView {
	v := WhyView{Node: node, Label: string(labels[node])}
	for _, b := range attackersOf(fw, node) {
		v.Because = append(v.Because, Because{
			Attacker:      b,
			AttackerLabel: string(labels[b]),
			Reason:        attackReason(g, fw, b, node),
		})
	}
	v.ToFlip = toFlip(g, fw, labels, node)
	return v
}

// Moves enumerates legal, useful next moves for an issue: propose, plus the
// flip suggestions for each of its positions.
func Moves(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, issue string) MovesView {
	mv := MovesView{Issue: issue}
	mv.Moves = append(mv.Moves, MoveSuggestion{
		Move: "propose", Args: []string{issue}, Effect: "add another candidate position",
	})
	for _, p := range positionsFor(g, issue) {
		mv.Moves = append(mv.Moves, toFlip(g, fw, labels, p)...)
	}
	return mv
}

// Agenda returns all UNDEC AF nodes, sorted by id.
func Agenda(g *ibis.Graph, labels map[string]af.Label) AgendaView {
	var v AgendaView
	ids := make([]string, 0, len(labels))
	for id := range labels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if labels[id] != af.UNDEC {
			continue
		}
		n := g.Nodes[id]
		v.Undecided = append(v.Undecided, NodeRef{ID: id, Kind: string(n.Kind), Text: n.Text, Label: "UNDEC"})
	}
	return v
}

// toFlip generates the moves that would change a node's label (design §4.3).
func toFlip(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, node string) []MoveSuggestion {
	var out []MoveSuggestion
	atk := attackersOf(fw, node)
	switch labels[node] {
	case af.IN:
		out = append(out, MoveSuggestion{
			Move: "object", Args: []string{node},
			Effect: fmt.Sprintf("introduce an undefeated attacker to make %s contested", node),
		})
		for _, b := range atk {
			out = append(out, MoveSuggestion{
				Move: "prefer", Args: []string{b, node},
				Effect: fmt.Sprintf("promote attacker %s to defeat %s", b, node),
			})
		}
	case af.OUT:
		for _, b := range atk {
			if labels[b] != af.IN {
				continue
			}
			out = append(out,
				MoveSuggestion{Move: "object", Args: []string{b},
					Effect: fmt.Sprintf("defeat attacker %s to reinstate %s", b, node)},
				MoveSuggestion{Move: "prefer", Args: []string{node, b},
					Effect: fmt.Sprintf("prefer %s over %s to block its attack", node, b)},
			)
		}
	case af.UNDEC:
		for _, b := range atk {
			if labels[b] != af.UNDEC {
				continue
			}
			out = append(out, MoveSuggestion{
				Move: "prefer", Args: []string{node, b},
				Effect: fmt.Sprintf("prefer %s over %s to break the tie (make %s IN)", node, b, node),
			})
		}
	}
	return out
}

// WhyText renders a WhyView as human text.
func WhyText(v WhyView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s is %s\n", v.Node, v.Label)
	for _, r := range v.Because {
		fmt.Fprintf(&b, "  ← %s [%s]  %s\n", r.Attacker, r.AttackerLabel, r.Reason)
	}
	if len(v.ToFlip) > 0 {
		fmt.Fprintln(&b, "  to flip:")
		for _, m := range v.ToFlip {
			fmt.Fprintf(&b, "    %s %s — %s\n", m.Move, strings.Join(m.Args, " "), m.Effect)
		}
	}
	return b.String()
}

// MovesText renders a MovesView.
func MovesText(v MovesView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "legal moves for %s:\n", v.Issue)
	for _, m := range v.Moves {
		fmt.Fprintf(&b, "  %s %s — %s\n", m.Move, strings.Join(m.Args, " "), m.Effect)
	}
	return b.String()
}

// AgendaText renders an AgendaView.
func AgendaText(v AgendaView) string {
	if len(v.Undecided) == 0 {
		return "agenda empty — nothing contested\n"
	}
	var b strings.Builder
	fmt.Fprintln(&b, "live agenda (UNDEC):")
	for _, n := range v.Undecided {
		fmt.Fprintf(&b, "  %s%s  %q\n", ibis.PrefixFor(ibis.Kind(n.Kind)), n.ID, n.Text)
	}
	return b.String()
}

func attackersOf(fw *af.Framework, node string) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range fw.Attack {
		if e.To == node && !seen[e.From] {
			seen[e.From] = true
			out = append(out, e.From)
		}
	}
	sort.Strings(out)
	return out
}

// attackReason describes why b attacks node and whether preference blocks it.
func attackReason(g *ibis.Graph, fw *af.Framework, b, node string) string {
	kind := "select-one rival"
	for _, l := range g.Links {
		if l.Rel == ibis.ObjectsTo && l.Src == b && l.Dst == node {
			kind = "objection"
			break
		}
	}
	if fw.Preferred[[2]string{node, b}] {
		basis := basisOf(g, node, b)
		if basis != "" {
			return fmt.Sprintf("%s, blocked by preferred(%s,%s) basis=%s", kind, node, b, basis)
		}
		return fmt.Sprintf("%s, blocked by preferred(%s,%s)", kind, node, b)
	}
	return kind
}

func basisOf(g *ibis.Graph, winner, loser string) string {
	for _, p := range g.Preferences {
		if p.Winner == winner && p.Loser == loser {
			return p.Basis
		}
	}
	return ""
}
