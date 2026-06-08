// Package render turns a discussion graph + grounded labelling into human text
// or agent JSON. One computation, two backends.
package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// PositionView is one position's status within an issue.
type PositionView struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	Label      string   `json:"label"`
	AttackedBy []string `json:"attacked_by"`
	DefeatedBy []string `json:"defeated_by"`
	Reinstated bool     `json:"reinstated"`
}

// IssueStatus is the status envelope for one issue (design §8.3).
type IssueStatus struct {
	Issue       string         `json:"issue"`
	IssueText   string         `json:"issue_text"`
	Cardinality string         `json:"cardinality"`
	Positions   []PositionView `json:"positions"`
	Undecided   []string       `json:"undecided"`
	Advice      string         `json:"advice"`
}

// Status computes the status of one issue.
func Status(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, issue string) IssueStatus {
	attackers := map[string][]string{}
	for _, e := range fw.Attack {
		attackers[e.To] = append(attackers[e.To], e.From)
	}
	defeaters := map[string][]string{}
	for _, e := range fw.Defeat {
		if labels[e.From] == af.IN {
			defeaters[e.To] = append(defeaters[e.To], e.From)
		}
	}

	card := string(g.IssueCards[issue])
	if card == "" {
		card = string(ibis.SelectOne)
	}
	st := IssueStatus{
		Issue:       issue,
		IssueText:   g.Nodes[issue].Text,
		Cardinality: card,
	}

	positions := positionsFor(g, issue)
	for _, p := range positions {
		atk := dedup(attackers[p])
		def := dedup(defeaters[p])
		lbl := labels[p]
		st.Positions = append(st.Positions, PositionView{
			ID:         p,
			Text:       g.Nodes[p].Text,
			Label:      string(lbl),
			AttackedBy: atk,
			DefeatedBy: def,
			Reinstated: lbl == af.IN && len(atk) > 0,
		})
		if lbl == af.UNDEC {
			st.Undecided = append(st.Undecided, p)
		}
	}
	st.Advice = advise(st)
	return st
}

func advise(st IssueStatus) string {
	var in, undec []string
	for _, p := range st.Positions {
		switch p.Label {
		case "IN":
			in = append(in, p.ID)
		case "UNDEC":
			undec = append(undec, p.ID)
		}
	}
	switch {
	case len(st.Positions) == 0:
		return "no positions yet; propose one"
	case len(in) == 1 && len(undec) == 0:
		return fmt.Sprintf("%s justified", in[0])
	case len(undec) >= 2 && len(in) == 0:
		return fmt.Sprintf("%s tied — needs a preference or a defeating argument", strings.Join(undec, " vs "))
	case len(in) >= 1:
		return fmt.Sprintf("%s justified", strings.Join(in, ", "))
	default:
		return "contested; add an argument or preference to resolve"
	}
}

// StatusText renders an issue status as human text.
func StatusText(st IssueStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "issue %s  %q  [%s]\n", st.Issue, st.IssueText, st.Cardinality)
	for _, p := range st.Positions {
		fmt.Fprintf(&b, "  %-5s %s  %q", p.Label, ibis.PrefixFor(ibis.Position)+p.ID, p.Text)
		if len(p.DefeatedBy) > 0 {
			fmt.Fprintf(&b, "  (defeated by %s)", strings.Join(p.DefeatedBy, ", "))
		} else if p.Reinstated {
			fmt.Fprint(&b, "  (reinstated)")
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "  → %s\n", st.Advice)
	return b.String()
}

// JSON marshals any view as indented JSON.
func JSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Tree renders the IBIS graph rooted at the given issue (or all root issues if
// issue == "") as an indented outline.
func Tree(g *ibis.Graph, issue string) string {
	var b strings.Builder
	roots := []string{}
	if issue != "" {
		roots = []string{issue}
	} else {
		for id, n := range g.Nodes {
			if n.Kind == ibis.Issue && !respondsToSomething(g, id) {
				roots = append(roots, id)
			}
		}
		sort.Strings(roots)
	}
	for _, r := range roots {
		writeNode(&b, g, r, 0, map[string]bool{})
	}
	return b.String()
}

func writeNode(b *strings.Builder, g *ibis.Graph, node string, depth int, seen map[string]bool) {
	if seen[node] {
		return
	}
	seen[node] = true
	n := g.Nodes[node]
	fmt.Fprintf(b, "%s%s%s  %q\n", strings.Repeat("  ", depth), ibis.PrefixFor(n.Kind), node, n.Text)
	for _, l := range g.Links {
		if l.Dst == node && (l.Rel == ibis.RespondsTo || l.Rel == ibis.Supports || l.Rel == ibis.ObjectsTo) {
			fmt.Fprintf(b, "%s  [%s]\n", strings.Repeat("  ", depth+1), l.Rel)
			writeNode(b, g, l.Src, depth+2, seen)
		}
	}
}

func positionsFor(g *ibis.Graph, issue string) []string {
	var ps []string
	for _, l := range g.Links {
		if l.Rel == ibis.RespondsTo && l.Dst == issue {
			if n, ok := g.Nodes[l.Src]; ok && n.Kind == ibis.Position {
				ps = append(ps, l.Src)
			}
		}
	}
	sort.Strings(ps)
	return ps
}

func respondsToSomething(g *ibis.Graph, node string) bool {
	for _, l := range g.Links {
		if l.Src == node && l.Rel == ibis.RespondsTo {
			return true
		}
	}
	return false
}

func dedup(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
