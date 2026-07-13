package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// InheritedQuestion is one undefeated objection against a (transitive) parent
// of a synthesis — a criticism the hybrid sheds by construction and must
// answer on the record. It is open until some node carries an addresses link
// to the objection: an object against the hybrid that re-aims it ("this still
// applies"), or a support on the hybrid that dismisses it ("here is why the
// hybrid escapes it"). Both are written with --answers <objection-id>
// (wicked-problems-2.md item 2).
type InheritedQuestion struct {
	Objection     string   `json:"objection"`
	ObjectionText string   `json:"objection_text"`
	Author        string   `json:"author,omitempty"`
	Parent        string   `json:"parent"`
	ParentText    string   `json:"parent_text"`
	AddressedBy   []string `json:"addressed_by,omitempty"` // nodes carrying addresses links
	Open          bool     `json:"open"`
}

// InheritedQuestions computes a synthesis's inherited questions over the
// transitive closure of its synthesizes lineage: each parent's objections
// that are not OUT (a parent objection beaten by counter-argument was
// answered on the parent; one that still stands transfers to the hybrid).
// addresses links are evaluator-inert; they exist so discharge is a
// computable question. Nil for a non-synthesis.
func InheritedQuestions(g *ibis.Graph, labels map[string]af.Label, node string) []InheritedQuestion {
	ancestors := g.SynthesisAncestors(node)
	if len(ancestors) == 0 {
		return nil
	}
	addressedBy := map[string][]string{} // objection id -> addressing nodes
	for _, l := range g.Links {
		if l.Rel == ibis.Addresses {
			if _, ok := g.Nodes[l.Src]; ok { // a conceded addresser no longer discharges
				addressedBy[l.Dst] = append(addressedBy[l.Dst], l.Src)
			}
		}
	}
	seen := map[string]bool{}
	var out []InheritedQuestion
	for _, parent := range ancestors {
		for _, l := range g.Links {
			if l.Rel != ibis.ObjectsTo || l.Dst != parent || seen[l.Src] {
				continue
			}
			obj, ok := g.Nodes[l.Src]
			if !ok || labels[l.Src] == af.OUT {
				continue
			}
			seen[l.Src] = true
			by := append([]string{}, addressedBy[l.Src]...)
			sort.Strings(by)
			out = append(out, InheritedQuestion{
				Objection: l.Src, ObjectionText: obj.Text, Author: obj.Author,
				Parent: parent, ParentText: g.Nodes[parent].Text,
				AddressedBy: by, Open: len(by) == 0,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Objection < out[j].Objection })
	return out
}

// openQuestions filters to the still-open inherited questions.
func openQuestions(qs []InheritedQuestion) []InheritedQuestion {
	var out []InheritedQuestion
	for _, q := range qs {
		if q.Open {
			out = append(out, q)
		}
	}
	return out
}

// stressTestEffect is the one composite prompt for an unexamined or
// question-laden hybrid — a single suggestion instead of three uncoordinated
// nags (item 2's presentation rule), naming the Adopter's cost question among
// the candidate attacks (item 5).
func stressTestEffect(hybrid string, open []InheritedQuestion) string {
	ids := make([]string, 0, len(open))
	for _, q := range open {
		ids = append(ids, q.Objection)
	}
	return fmt.Sprintf(
		"stress-test this hybrid: %d inherited question(s) from its parents are open (%s) — re-aim one with `object %s ... --answers <id>`, dismiss one with `support %s ... --answers <id>`, and state the adoption cost: who does the added work, and what do they stop doing?",
		len(open), strings.Join(ids, ", "), hybrid, hybrid)
}

// SelfElevation analyzes prefer(winner, loser) for the subsumption dodge
// (wicked-problems-2.md item 3): the winner has transitive synthesizes
// lineage to the loser, and the loser's undefeated objections have not all
// been addressed on the winner with at least one address authored by someone
// other than the winner's author. Returns the loser's still-open objection
// ids and whether the pair is flagged. Not flagged when there is no lineage
// or the loser has no undefeated objections — burying a parent whose critics
// were all answered is legitimate subsumption.
func SelfElevation(g *ibis.Graph, labels map[string]af.Label, winner, loser string) (open []string, flagged bool) {
	inLineage := false
	for _, a := range g.SynthesisAncestors(winner) {
		if a == loser {
			inLineage = true
			break
		}
	}
	if !inLineage {
		return nil, false
	}
	var qs []InheritedQuestion
	for _, q := range InheritedQuestions(g, labels, winner) {
		if q.Parent == loser {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		return nil, false
	}
	crossAuthor := false
	winnerAuthor := g.Nodes[winner].Author
	for _, q := range qs {
		if q.Open {
			open = append(open, q.Objection)
			continue
		}
		for _, by := range q.AddressedBy {
			if a := g.Nodes[by].Author; a == "" || a != winnerAuthor {
				crossAuthor = true
			}
		}
	}
	return open, len(open) > 0 || !crossAuthor
}

// questionsText renders inherited questions for show/why text output.
func questionsText(qs []InheritedQuestion) string {
	var b strings.Builder
	b.WriteString("  " + cBold("inherited questions") + cDim(" (from synthesis parents; discharge with --answers):") + "\n")
	for _, q := range qs {
		state := labelColor("UNDEC", "open     ")
		if !q.Open {
			state = labelColor("IN", "addressed") + cDim(" by "+strings.Join(q.AddressedBy, ", "))
		}
		prefix := fmt.Sprintf("    %s %s  ", cID(q.Objection), cDim("on "+q.Parent))
		b.WriteString(para(prefix, quote(q.ObjectionText)) + "\n")
		b.WriteString("      " + state + "\n")
	}
	return b.String()
}
