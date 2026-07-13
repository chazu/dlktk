package render

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/store"
)

// MoveSuggestion is a legal, useful next move in {move,args,effect} shape
// (shared by `why` to_flip and `moves`).
type MoveSuggestion struct {
	Move   string   `json:"move"`
	Args   []string `json:"args"`
	Effect string   `json:"effect"`
}

// Because explains one contributor to a node's label. AttackerText carries the
// attacker's claim so an agent can read what it is rebutting without a second
// round-trip.
type Because struct {
	Attacker      string `json:"attacker"`
	AttackerText  string `json:"attacker_text"`
	AttackerLabel string `json:"attacker_label"`
	Reason        string `json:"reason"`
}

// WhyView is the explanation envelope for one node (design §8.3). A synthesis
// additionally lists its inherited questions — the parents' undefeated
// objections it must answer on the record (item 2).
type WhyView struct {
	Node               string              `json:"node"`
	Text               string              `json:"text"`
	Label              string              `json:"label"`
	Because            []Because           `json:"because"`
	InheritedQuestions []InheritedQuestion `json:"inherited_questions,omitempty"`
	ToFlip             []MoveSuggestion    `json:"to_flip"`
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

// IssueRef points at an issue needing attention, optionally with the position
// that resolves it.
type IssueRef struct {
	Issue        string `json:"issue"`
	Text         string `json:"text"`
	Position     string `json:"position,omitempty"`
	PositionText string `json:"position_text,omitempty"`
}

// AgendaView is the worklist that drives a discussion to closure — and keeps
// its openings honest: contested (UNDEC) nodes that need argument or
// preference, issues whose labelling has settled on a unique justified
// position and only await a decide, issues with no positions at all, ready
// winners that never faced a substantive objection (untested — stress-test
// before deciding; surfaced only when decide-adjacent, since mid-divergence
// every fresh position is untested by design), and assumptions nobody has
// examined. Nodes under a reframed issue's dead framing are excluded
// throughout: the question has moved.
type AgendaView struct {
	Undecided   []NodeRef  `json:"undecided"`
	Ready       []IssueRef `json:"ready"`
	Unpopulated []IssueRef `json:"unpopulated"`
	Untested    []IssueRef `json:"untested"`
	Assumptions []NodeRef  `json:"assumptions"`
}

// Why builds the explanation for a node.
func Why(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, node string) WhyView {
	v := WhyView{Node: node, Text: g.Nodes[node].Text, Label: string(labels[node])}
	for _, b := range attackersOf(fw, node) {
		v.Because = append(v.Because, Because{
			Attacker:      b,
			AttackerText:  g.Nodes[b].Text,
			AttackerLabel: string(labels[b]),
			Reason:        attackReason(g, fw, b, node),
		})
	}
	v.InheritedQuestions = InheritedQuestions(g, labels, node)
	v.ToFlip = toFlip(g, fw, labels, node)
	return v
}

// Moves enumerates legal, useful next moves for an issue: decide when the
// labelling has settled (so the loop has a terminal move) — preceded by a
// stress-test suggestion when the winner was never substantively attacked or
// is a synthesis with open inherited questions (one composite prompt, not
// three uncoordinated nags) — propose, the generative exits from a stalemate
// (synthesize / reframe), plus the flip suggestions for each of its positions.
func Moves(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, issue string, decs []ibis.Decision) MovesView {
	mv := MovesView{Issue: issue}
	st := Status(g, fw, labels, issue, decs)
	// Live syntheses with open inherited questions get the composite
	// stress-test prompt whatever their label: the questions are work.
	prompted := map[string]bool{}
	for _, p := range positionsFor(g, issue) {
		if labels[p] == af.OUT {
			continue
		}
		if open := openQuestions(InheritedQuestions(g, labels, p)); len(open) > 0 {
			prompted[p] = true
			mv.Moves = append(mv.Moves, MoveSuggestion{
				Move: "object", Args: []string{p},
				Effect: stressTestEffect(p, open),
			})
		}
	}
	if pos, ok := readyToDecide(g, labels, issue, decs); ok {
		if !Tested(g, fw, pos) && !prompted[pos] {
			mv.Moves = append(mv.Moves, MoveSuggestion{
				Move: "object", Args: []string{pos},
				Effect: fmt.Sprintf("stress-test %s before deciding: it is IN without a substantive objection (rival edges and self-objections don't count), not by surviving attack", pos),
			})
		}
		mv.Moves = append(mv.Moves, MoveSuggestion{
			Move: "decide", Args: []string{issue, pos},
			Effect: fmt.Sprintf("close the issue: %s is the unique justified position", pos),
		})
	}
	mv.Moves = append(mv.Moves, MoveSuggestion{
		Move: "propose", Args: []string{issue}, Effect: "add another candidate position",
	})
	if st.Stalemate {
		args := []string{issue}
		for _, u := range st.Undecided {
			args = append(args, "--from", u)
		}
		mv.Moves = append(mv.Moves,
			MoveSuggestion{Move: "synthesize", Args: args,
				Effect: "recombine the deadlocked rivals into a hybrid — note it joins the rivalry (stalemate becomes N+1-way) until the parents are conceded or a preference/audience elevates it; a synthesis that drops nothing is a bundle — record exclusions with --drops"},
			MoveSuggestion{Move: "reframe", Args: []string{issue},
				Effect: "replace the framing if the deadlock signals a false dichotomy (positions do not carry over; lineage is recorded)"},
		)
	}
	for _, p := range positionsFor(g, issue) {
		mv.Moves = append(mv.Moves, toFlip(g, fw, labels, p)...)
	}
	return mv
}

// Agenda returns the discussion's worklist: UNDEC nodes (sorted by id), issues
// ready to decide, issues with no positions yet, untested decide-adjacent
// winners, and unexamined assumptions. Everything under a reframed issue's
// dead framing is excluded.
func Agenda(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, decs []ibis.Decision) AgendaView {
	var v AgendaView

	// The dead framings and everything argued under them.
	reframed := map[string]bool{}
	deadScope := map[string]bool{}
	for _, r := range g.Reframes {
		reframed[r.Old] = true
		for id := range reachableAF(g, r.Old) {
			deadScope[id] = true
		}
	}

	ids := make([]string, 0, len(labels))
	for id := range labels {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if labels[id] != af.UNDEC || deadScope[id] {
			continue
		}
		n := g.Nodes[id]
		v.Undecided = append(v.Undecided, NodeRef{ID: id, Kind: string(n.Kind), Text: n.Text, Label: "UNDEC"})
	}

	var issues []string
	for id, n := range g.Nodes {
		if n.Kind == ibis.Issue && !reframed[id] {
			issues = append(issues, id)
		}
	}
	sort.Strings(issues)
	for _, issue := range issues {
		positions := positionsFor(g, issue)
		if len(positions) == 0 {
			v.Unpopulated = append(v.Unpopulated, IssueRef{Issue: issue, Text: g.Nodes[issue].Text})
			continue
		}
		if pos, ok := readyToDecide(g, labels, issue, decs); ok {
			v.Ready = append(v.Ready, IssueRef{
				Issue: issue, Text: g.Nodes[issue].Text,
				Position: pos, PositionText: g.Nodes[pos].Text,
			})
			// Untested: surfaced only when decide-adjacent — this issue is
			// about to close on a winner no substantive objection ever
			// engaged (rival edges never count). During divergence every
			// fresh position is untested by design; flooding the section
			// trains agents to skim it (wicked-problems-2.md item 1).
			if !Tested(g, fw, pos) {
				v.Untested = append(v.Untested, IssueRef{
					Issue: issue, Text: g.Nodes[issue].Text,
					Position: pos, PositionText: g.Nodes[pos].Text,
				})
			}
		}
	}

	// Assumptions nobody has examined: no support, no objection, not defeated.
	for _, id := range ids {
		n := g.Nodes[id]
		if n.Tag != ibis.TagAssumption || labels[id] == af.OUT || deadScope[id] {
			continue
		}
		examined := false
		for _, l := range g.Links {
			if l.Dst == id && (l.Rel == ibis.ObjectsTo || l.Rel == ibis.Supports) {
				examined = true
				break
			}
		}
		if !examined {
			v.Assumptions = append(v.Assumptions, NodeRef{ID: id, Kind: string(n.Kind), Text: n.Text, Label: string(labels[id])})
		}
	}
	return v
}

// readyToDecide reports the position an undecided issue is ready to close on:
// no position is contested and exactly one is justified.
func readyToDecide(g *ibis.Graph, labels map[string]af.Label, issue string, decs []ibis.Decision) (string, bool) {
	for _, d := range decs {
		if d.Issue == issue {
			return "", false
		}
	}
	var in []string
	for _, p := range positionsFor(g, issue) {
		switch labels[p] {
		case af.IN:
			in = append(in, p)
		case af.UNDEC:
			return "", false
		}
	}
	if len(in) == 1 {
		return in[0], true
	}
	return "", false
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
			// Preferring a position over its own objection excuses the test
			// rather than answering it — say so where the move is offered,
			// not after it lands.
			out = append(out,
				MoveSuggestion{Move: "object", Args: []string{b},
					Effect: fmt.Sprintf("defeat attacker %s to reinstate %s", b, node)},
				MoveSuggestion{Move: "prefer", Args: []string{node, b},
					Effect: fmt.Sprintf("prefer %s over %s to block its attack — note this excuses the objection rather than answering it; %s will count as untested again", node, b, node)},
			)
		}
	case af.UNDEC:
		// Elevating an untested or question-laden position by preference is
		// the laundering move arc two exists to catch: state one ordering,
		// not two options.
		untested := !Tested(g, fw, node)
		open := openQuestions(InheritedQuestions(g, labels, node))
		for _, b := range atk {
			if labels[b] != af.UNDEC {
				continue
			}
			effect := fmt.Sprintf("prefer %s over %s to break the tie (make %s IN)", node, b, node)
			switch {
			case len(open) > 0:
				effect += fmt.Sprintf(" — address the open inherited questions on %s first, then prefer", node)
			case untested:
				effect += fmt.Sprintf(" — %s is untested; object first, then prefer", node)
			}
			out = append(out, MoveSuggestion{
				Move: "prefer", Args: []string{node, b},
				Effect: effect,
			})
		}
	}
	return out
}

// WhyText renders a WhyView as human text.
func WhyText(v WhyView) string {
	var b strings.Builder
	b.WriteString(para(fmt.Sprintf("%s  %s  is ", cID(v.Node), quote(v.Text)), labelInline(v.Label)) + "\n")
	for _, r := range v.Because {
		prefix := fmt.Sprintf("  %s %s %s  ", cDim("←"), labelInline(r.AttackerLabel), cID(r.Attacker))
		b.WriteString(para(prefix, quote(r.AttackerText)) + "\n")
		b.WriteString(strings.Repeat(" ", visLen(prefix)) + cDim(r.Reason) + "\n")
	}
	if len(v.InheritedQuestions) > 0 {
		b.WriteString(questionsText(v.InheritedQuestions))
	}
	if len(v.ToFlip) > 0 {
		fmt.Fprintf(&b, "  %s%s\n", cBold("to flip "+v.Node), cDim(" (currently "+v.Label+") — copy, fill the placeholders, run:"))
		writeSuggestions(&b, v.ToFlip)
	}
	return b.String()
}

// MovesText renders a MovesView as runnable command lines.
func MovesText(v MovesView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s%s\n", cBold("legal moves for"), cID(v.Issue), cDim(" — copy, fill the placeholders, run:"))
	if len(v.Moves) == 0 {
		b.WriteString("  " + cDim("(none)") + "\n")
	}
	writeSuggestions(&b, v.Moves)
	return b.String()
}

// writeSuggestions prints each move suggestion as a runnable command followed by
// a dimmed one-line effect.
func writeSuggestions(b *strings.Builder, ms []MoveSuggestion) {
	for _, m := range ms {
		b.WriteString("    " + cID(suggestionCommand(m)) + "\n")
		b.WriteString("      " + cDim(m.Effect) + "\n")
	}
}

// AgendaText renders an AgendaView.
func AgendaText(v AgendaView) string {
	if len(v.Undecided) == 0 && len(v.Ready) == 0 && len(v.Unpopulated) == 0 &&
		len(v.Untested) == 0 && len(v.Assumptions) == 0 {
		return cDim("agenda empty — nothing contested, nothing awaiting a decision") + "\n"
	}
	var b strings.Builder
	if len(v.Undecided) > 0 {
		b.WriteString(cBold("live agenda (UNDEC) — needs an argument or preference:") + "\n")
		for _, n := range v.Undecided {
			b.WriteString(para(fmt.Sprintf("  %s %s  ", labelInline("UNDEC"), nid(n.Kind, n.ID)), quote(n.Text)) + "\n")
		}
	}
	if len(v.Ready) > 0 {
		b.WriteString(cBold("ready to decide:") + "\n")
		for _, r := range v.Ready {
			b.WriteString(para(fmt.Sprintf("  %s  ", cID(ibis.PrefixFor(ibis.Issue)+r.Issue)), quote(r.Text)) + "\n")
			fmt.Fprintf(&b, "      %s %s  %s\n", cDim("→ decide"), pid(r.Position), quote(r.PositionText))
		}
	}
	if len(v.Unpopulated) > 0 {
		b.WriteString(cBold("no positions yet (propose one):") + "\n")
		for _, r := range v.Unpopulated {
			b.WriteString(para(fmt.Sprintf("  %s  ", cID(ibis.PrefixFor(ibis.Issue)+r.Issue)), quote(r.Text)) + "\n")
		}
	}
	if len(v.Untested) > 0 {
		b.WriteString(cBold("untested (about to win without facing a substantive objection — stress-test before deciding):") + "\n")
		for _, r := range v.Untested {
			b.WriteString(para(fmt.Sprintf("  %s  ", pid(r.Position)), quote(r.PositionText)) + "\n")
			fmt.Fprintf(&b, "      %s %s  %s\n", cDim("on"), cID(ibis.PrefixFor(ibis.Issue)+r.Issue), quote(r.Text))
		}
	}
	if len(v.Assumptions) > 0 {
		b.WriteString(cBold("unexamined assumptions (support or object):") + "\n")
		for _, n := range v.Assumptions {
			b.WriteString(para(fmt.Sprintf("  %s %s  ", labelInline(n.Label), nid(n.Kind, n.ID)), quote(n.Text)) + "\n")
		}
	}
	return b.String()
}

// LabelChange records a node whose grounded label moved between two viewpoints.
type LabelChange struct {
	Node string `json:"node"`
	Text string `json:"text"`
	From string `json:"from"`
	To   string `json:"to"`
}

// DiffView is the replay --diff result: structural and label changes between a
// past transaction-time T and now.
type DiffView struct {
	AsOf    string        `json:"as_of"`
	Added   []NodeRef     `json:"added"`   // present now, absent at T
	Removed []NodeRef     `json:"removed"` // present at T, absent now (retracted)
	Flipped []LabelChange `json:"flipped"` // label changed between T and now
}

// Diff compares the graph + grounded labelling at T against now.
func Diff(asOf string, gThen, gNow *ibis.Graph, lThen, lNow map[string]af.Label) DiffView {
	v := DiffView{AsOf: asOf}
	for id, n := range gNow.Nodes {
		if _, ok := gThen.Nodes[id]; !ok {
			v.Added = append(v.Added, NodeRef{ID: id, Kind: string(n.Kind), Text: n.Text, Label: string(lNow[id])})
		}
	}
	for id, n := range gThen.Nodes {
		if _, ok := gNow.Nodes[id]; !ok {
			v.Removed = append(v.Removed, NodeRef{ID: id, Kind: string(n.Kind), Text: n.Text, Label: string(lThen[id])})
		}
	}
	for id := range gNow.Nodes {
		if _, ok := gThen.Nodes[id]; !ok {
			continue
		}
		from, to := lThen[id], lNow[id]
		if from != to {
			v.Flipped = append(v.Flipped, LabelChange{Node: id, Text: gNow.Nodes[id].Text, From: string(from), To: string(to)})
		}
	}
	sort.Slice(v.Added, func(i, j int) bool { return v.Added[i].ID < v.Added[j].ID })
	sort.Slice(v.Removed, func(i, j int) bool { return v.Removed[i].ID < v.Removed[j].ID })
	sort.Slice(v.Flipped, func(i, j int) bool { return v.Flipped[i].Node < v.Flipped[j].Node })
	return v
}

// DiffText renders a DiffView.
func DiffText(v DiffView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", cBold("replay diff"), cDim("(as-of "+v.AsOf+" → now)"))
	if len(v.Added) == 0 && len(v.Removed) == 0 && len(v.Flipped) == 0 {
		b.WriteString("  " + cDim("no changes") + "\n")
		return b.String()
	}
	for _, n := range v.Added {
		b.WriteString(para(fmt.Sprintf("  %s %s %s  ", labelColor("IN", "+"), labelInline(n.Label), nid(n.Kind, n.ID)), quote(n.Text)) + "\n")
	}
	for _, n := range v.Removed {
		b.WriteString(para(fmt.Sprintf("  %s %s  ", labelColor("OUT", "−"), nid(n.Kind, n.ID)), quote(n.Text)) + "\n")
	}
	for _, c := range v.Flipped {
		b.WriteString(para(fmt.Sprintf("  %s %s  %s → %s  ", cDim("~"), cID(c.Node), labelInline(c.From), labelInline(c.To)), quote(c.Text)) + "\n")
	}
	return b.String()
}

// LogText renders a store history (audit trail) as human text: an assert/retract
// marker, an absolute timestamp with a relative hint, the summary (truncated to
// width), the author, and the node id.
func LogText(entries []store.HistoryEntry) string {
	if len(entries) == 0 {
		return cDim("no history") + "\n"
	}
	now := time.Now()
	var b strings.Builder
	for _, e := range entries {
		state := labelColor("IN", "+")
		if e.Retracted {
			state = labelColor("OUT", "×")
		}
		t := time.Unix(e.TxStart, 0)
		when := cDim(t.UTC().Format("2006-01-02 15:04") + " (" + relTime(t, now) + ")")
		author := ""
		if e.Author != "" {
			author = "  " + cDim("by "+e.Author)
		}
		idtag := ""
		if e.ID != "" {
			idtag = "  " + cID(e.ID)
		}
		summary := e.Summary
		if wrapWidth > 0 {
			fixed := visLen(state) + 1 + visLen(when) + 2 + visLen(author) + visLen(idtag)
			if avail := wrapWidth - fixed; avail >= 12 {
				summary = truncate(summary, avail)
			}
		}
		fmt.Fprintf(&b, "%s %s  %s%s%s\n", state, when, summary, author, idtag)
	}
	return b.String()
}

// AttackView is one edge of the materialized attack relation, tagged with where
// it came from and whether a preference (or, under an audience lens, a value
// ranking) neutralized it.
type AttackView struct {
	From            string `json:"from"`
	To              string `json:"to"`
	Source          string `json:"source"`                     // "objection" | "select_one"
	Defeats         bool   `json:"defeats"`                    // false if a preference or the audience blocks the attack
	Basis           string `json:"basis,omitempty"`            // preference basis when blocked pairwise
	AudienceBlocked string `json:"audience_blocked,omitempty"` // "value(target)≻value(attacker)" when the audience lens blocks it
}

// PrefView is one preference edge (asserted or derived by transitivity).
type PrefView struct {
	Winner  string `json:"winner"`
	Loser   string `json:"loser"`
	Basis   string `json:"basis,omitempty"`
	Derived bool   `json:"derived"`
}

// StepView is one labelling step in the grounded derivation.
type StepView struct {
	Round int      `json:"round"`
	Node  string   `json:"node"`
	Label string   `json:"label"`
	Why   string   `json:"why"`
	By    []string `json:"by,omitempty"`
}

// ExplainView is the full derivation of an issue's automated labelling (design §8.3).
type ExplainView struct {
	Issue        string         `json:"issue"`
	IssueText    string         `json:"issue_text"`
	Cardinality  string         `json:"cardinality"`
	Attacks      []AttackView   `json:"attacks"`
	Preferences  []PrefView     `json:"preferences"`
	Steps        []StepView     `json:"steps"`
	Outcome      []PositionView `json:"outcome"`
	Decided      *DecidedView   `json:"decided,omitempty"`
	DecisionIsIN bool           `json:"decision_is_in"` // recorded decision matches the justified (IN) position

	kinds map[string]ibis.Kind // id -> kind, for prefixing ids in text (not serialized)
}

// Explain derives the full reasoning for one issue: how attacks/defeats were
// constructed, the round-by-round grounded fixpoint, and the outcome vs any
// recorded decision. Restricted to the AF nodes reachable from the issue.
func Explain(g *ibis.Graph, fw *af.Framework, issue string, decs []ibis.Decision) ExplainView {
	card := string(g.IssueCards[issue])
	if card == "" {
		card = string(ibis.SelectOne)
	}
	v := ExplainView{Issue: issue, IssueText: g.Nodes[issue].Text, Cardinality: card, kinds: map[string]ibis.Kind{}}

	// Scope: AF nodes reachable upward from this issue's positions.
	scope := reachableAF(g, issue)
	for id := range scope {
		v.kinds[id] = g.Nodes[id].Kind
	}

	// Attacks, tagged by origin and preference outcome.
	defeat := map[[2]string]bool{}
	for _, e := range fw.Defeat {
		defeat[[2]string{e.From, e.To}] = true
	}
	for _, e := range fw.Attack {
		if !scope[e.From] || !scope[e.To] {
			continue
		}
		av := AttackView{From: e.From, To: e.To, Source: "select_one", Defeats: defeat[[2]string{e.From, e.To}]}
		for _, l := range g.Links {
			if l.Rel == ibis.ObjectsTo && l.Src == e.From && l.Dst == e.To {
				av.Source = "objection"
				break
			}
		}
		if !av.Defeats {
			av.Basis = basisOf(g, e.To, e.From)
			if fw.AudienceBlocked != nil {
				av.AudienceBlocked = fw.AudienceBlocked[[2]string{e.From, e.To}]
			}
		}
		v.Attacks = append(v.Attacks, av)
	}
	sort.Slice(v.Attacks, func(i, j int) bool {
		if v.Attacks[i].From != v.Attacks[j].From {
			return v.Attacks[i].From < v.Attacks[j].From
		}
		return v.Attacks[i].To < v.Attacks[j].To
	})

	// Preferences: asserted edges, then closure-only (derived) edges.
	asserted := map[[2]string]bool{}
	for _, p := range g.Preferences {
		if !scope[p.Winner] || !scope[p.Loser] {
			continue
		}
		asserted[[2]string{p.Winner, p.Loser}] = true
		v.Preferences = append(v.Preferences, PrefView{Winner: p.Winner, Loser: p.Loser, Basis: p.Basis})
	}
	for pair := range fw.Preferred {
		if asserted[pair] || !scope[pair[0]] || !scope[pair[1]] {
			continue
		}
		v.Preferences = append(v.Preferences, PrefView{Winner: pair[0], Loser: pair[1], Derived: true})
	}
	sort.Slice(v.Preferences, func(i, j int) bool {
		if v.Preferences[i].Winner != v.Preferences[j].Winner {
			return v.Preferences[i].Winner < v.Preferences[j].Winner
		}
		return v.Preferences[i].Loser < v.Preferences[j].Loser
	})

	// Grounded derivation, scoped.
	steps, labels := fw.GroundedSteps()
	for _, s := range steps {
		if !scope[s.Node] {
			continue
		}
		v.Steps = append(v.Steps, StepView{Round: s.Round, Node: s.Node, Label: string(s.Label), Why: s.Why, By: s.By})
	}

	// Outcome: each position's final label.
	for _, p := range positionsFor(g, issue) {
		v.Outcome = append(v.Outcome, PositionView{ID: p, Text: g.Nodes[p].Text, Label: string(labels[p])})
	}
	for _, d := range decs {
		if d.Issue == issue {
			v.Decided = &DecidedView{Position: d.Position, Basis: d.Basis, Decider: d.Decider, Override: d.Override, Supersedes: d.Supersedes, ReviewBy: d.ReviewBy}
			v.DecisionIsIN = labels[d.Position] == af.IN
		}
	}
	return v
}

// reachableAF returns the set of AF nodes reachable upward (Dst<-Src over any
// IBIS relation) from an issue's positions.
func reachableAF(g *ibis.Graph, issue string) map[string]bool {
	seen := map[string]bool{}
	stack := append([]string{}, positionsFor(g, issue)...)
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for _, l := range g.Links {
			if l.Dst == cur && g.IsAFNode(l.Src) {
				stack = append(stack, l.Src)
			}
		}
	}
	return seen
}

// ExplainText renders an ExplainView as human text. When brief, the conceptual
// primer is omitted.
func ExplainText(v ExplainView, brief bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s  %s  %s\n", cBold("explain"), cID(ibis.PrefixFor(ibis.Issue)+v.Issue), quote(v.IssueText), cDim("["+v.Cardinality+"]"))
	if !brief {
		b.WriteString("\nhow this resolves — Dung grounded semantics:\n")
		b.WriteString("  objections and select_one rivalry become ATTACKS; a preference can DEFEAT\n")
		b.WriteString("  (neutralize) an attack; then a fixpoint labels each node IN (justified) /\n")
		b.WriteString("  OUT (defeated) / UNDEC (contested). Positions left IN are the standing answer.\n")
	}

	pre := func(id string) string { return cID(ibis.PrefixFor(v.kinds[id]) + id) }
	preList := func(ids []string) string {
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = pre(id)
		}
		return strings.Join(out, ", ")
	}

	b.WriteString("\n" + cBold("1. attacks derived:") + "\n")
	if len(v.Attacks) == 0 {
		b.WriteString("   (none — no objections or select_one rivalry)\n")
	}
	for _, a := range v.Attacks {
		note := a.Source
		if !a.Defeats {
			if a.AudienceBlocked != "" {
				note += ", neutralized by audience (" + a.AudienceBlocked + ")"
			} else {
				note += ", neutralized by preference"
				if a.Basis != "" {
					note += " (basis=" + a.Basis + ")"
				}
			}
		}
		fmt.Fprintf(&b, "   %s ⚔ %s  (%s)\n", pre(a.From), pre(a.To), note)
	}
	if len(v.Preferences) == 0 {
		b.WriteString("   preferences: none → every attack is a defeat\n")
	} else {
		b.WriteString("   preferences:\n")
		for _, p := range v.Preferences {
			tag := ""
			if p.Derived {
				tag = " (derived by transitivity)"
			} else if p.Basis != "" {
				tag = " (basis=" + p.Basis + ")"
			}
			fmt.Fprintf(&b, "     %s ≻ %s%s\n", pre(p.Winner), pre(p.Loser), tag)
		}
	}

	b.WriteString("\n" + cBold("2. automated reasoning — grounded fixpoint:") + "\n")
	round := 0
	for _, s := range v.Steps {
		if s.Round != round {
			round = s.Round
			fmt.Fprintf(&b, "   round %d:\n", round)
		}
		var reason string
		switch s.Why {
		case "unattacked":
			reason = "no defeaters"
		case "reinstated":
			reason = "reinstated — defeater(s) " + preList(s.By) + " now OUT"
		case "defeated":
			reason = "defeated by " + preList(s.By) + " [IN]"
		case "contested":
			reason = "contested — unresolved cycle/stalemate with " + preList(s.By)
		default:
			reason = s.Why
		}
		fmt.Fprintf(&b, "     %s %s  %s\n", labelCol(s.Label), pre(s.Node), cDim("("+reason+")"))
	}
	if len(v.Steps) == 0 {
		b.WriteString("   " + cDim("(no positions or arguments yet)") + "\n")
	}

	b.WriteString("\n" + cBold("3. outcome:") + "\n")
	for _, p := range v.Outcome {
		marker := ""
		if p.Label == "IN" {
			marker = labelColor("IN", "  ← justified")
		}
		fmt.Fprintf(&b, "   %s %s%s  %s\n", labelCol(p.Label), pid(p.ID), marker, quote(p.Text))
	}
	if v.Decided != nil {
		if v.DecisionIsIN {
			fmt.Fprintf(&b, "   %s %s %s\n", labelColor("IN", "✓ decision recorded:"), pid(v.Decided.Position), cDim("(matches the justified position)"))
		} else {
			fmt.Fprintf(&b, "   %s %s %s\n", labelColor("UNDEC", "⚠ decision recorded:"), pid(v.Decided.Position), cDim("(OVERRIDE — not the justified position)"))
		}
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
