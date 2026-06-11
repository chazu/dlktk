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

// WhyView is the explanation envelope for one node (design §8.3).
type WhyView struct {
	Node    string           `json:"node"`
	Text    string           `json:"text"`
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

// IssueRef points at an issue needing attention, optionally with the position
// that resolves it.
type IssueRef struct {
	Issue        string `json:"issue"`
	Text         string `json:"text"`
	Position     string `json:"position,omitempty"`
	PositionText string `json:"position_text,omitempty"`
}

// AgendaView is the worklist that drives a discussion to closure: contested
// (UNDEC) nodes that need argument or preference, issues whose labelling has
// settled on a unique justified position and only await a decide, and issues
// with no positions at all.
type AgendaView struct {
	Undecided   []NodeRef  `json:"undecided"`
	Ready       []IssueRef `json:"ready"`
	Unpopulated []IssueRef `json:"unpopulated"`
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
	v.ToFlip = toFlip(g, fw, labels, node)
	return v
}

// Moves enumerates legal, useful next moves for an issue: decide when the
// labelling has settled (so the loop has a terminal move), propose, plus the
// flip suggestions for each of its positions.
func Moves(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, issue string, decs []ibis.Decision) MovesView {
	mv := MovesView{Issue: issue}
	if pos, ok := readyToDecide(g, labels, issue, decs); ok {
		mv.Moves = append(mv.Moves, MoveSuggestion{
			Move: "decide", Args: []string{issue, pos},
			Effect: fmt.Sprintf("close the issue: %s is the unique justified position", pos),
		})
	}
	mv.Moves = append(mv.Moves, MoveSuggestion{
		Move: "propose", Args: []string{issue}, Effect: "add another candidate position",
	})
	for _, p := range positionsFor(g, issue) {
		mv.Moves = append(mv.Moves, toFlip(g, fw, labels, p)...)
	}
	return mv
}

// Agenda returns the discussion's worklist: UNDEC nodes (sorted by id), issues
// ready to decide, and issues with no positions yet.
func Agenda(g *ibis.Graph, labels map[string]af.Label, decs []ibis.Decision) AgendaView {
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

	var issues []string
	for id, n := range g.Nodes {
		if n.Kind == ibis.Issue {
			issues = append(issues, id)
		}
	}
	sort.Strings(issues)
	for _, issue := range issues {
		if len(positionsFor(g, issue)) == 0 {
			v.Unpopulated = append(v.Unpopulated, IssueRef{Issue: issue, Text: g.Nodes[issue].Text})
			continue
		}
		if pos, ok := readyToDecide(g, labels, issue, decs); ok {
			v.Ready = append(v.Ready, IssueRef{
				Issue: issue, Text: g.Nodes[issue].Text,
				Position: pos, PositionText: g.Nodes[pos].Text,
			})
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
	fmt.Fprintf(&b, "%s  %q  is %s\n", v.Node, v.Text, v.Label)
	for _, r := range v.Because {
		fmt.Fprintf(&b, "  ← %s [%s]  %q  (%s)\n", r.Attacker, r.AttackerLabel, r.AttackerText, r.Reason)
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
	if len(v.Undecided) == 0 && len(v.Ready) == 0 && len(v.Unpopulated) == 0 {
		return "agenda empty — nothing contested, nothing awaiting a decision\n"
	}
	var b strings.Builder
	if len(v.Undecided) > 0 {
		fmt.Fprintln(&b, "live agenda (UNDEC):")
		for _, n := range v.Undecided {
			fmt.Fprintf(&b, "  %s%s  %q\n", ibis.PrefixFor(ibis.Kind(n.Kind)), n.ID, n.Text)
		}
	}
	if len(v.Ready) > 0 {
		fmt.Fprintln(&b, "ready to decide:")
		for _, r := range v.Ready {
			fmt.Fprintf(&b, "  %s%s  %q  → %s%s  %q\n",
				ibis.PrefixFor(ibis.Issue), r.Issue, r.Text,
				ibis.PrefixFor(ibis.Position), r.Position, r.PositionText)
		}
	}
	if len(v.Unpopulated) > 0 {
		fmt.Fprintln(&b, "no positions yet (propose one):")
		for _, r := range v.Unpopulated {
			fmt.Fprintf(&b, "  %s%s  %q\n", ibis.PrefixFor(ibis.Issue), r.Issue, r.Text)
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
	fmt.Fprintf(&b, "replay diff (as-of %s → now)\n", v.AsOf)
	if len(v.Added) == 0 && len(v.Removed) == 0 && len(v.Flipped) == 0 {
		b.WriteString("  no changes\n")
		return b.String()
	}
	for _, n := range v.Added {
		fmt.Fprintf(&b, "  + %s%s  %q  [%s]\n", ibis.PrefixFor(ibis.Kind(n.Kind)), n.ID, n.Text, n.Label)
	}
	for _, n := range v.Removed {
		fmt.Fprintf(&b, "  - %s%s  %q\n", ibis.PrefixFor(ibis.Kind(n.Kind)), n.ID, n.Text)
	}
	for _, c := range v.Flipped {
		fmt.Fprintf(&b, "  ~ %s  %s → %s  %q\n", c.Node, c.From, c.To, c.Text)
	}
	return b.String()
}

// LogText renders a store history (audit trail) as human text.
func LogText(entries []store.HistoryEntry) string {
	if len(entries) == 0 {
		return "no history\n"
	}
	var b strings.Builder
	for _, e := range entries {
		state := "+"
		if e.Retracted {
			state = "×"
		}
		ts := time.Unix(e.TxStart, 0).UTC().Format(time.RFC3339)
		fmt.Fprintf(&b, "%s %s  %s  %s", state, ts, e.Summary, e.Author)
		if e.ID != "" {
			fmt.Fprintf(&b, "  (%s)", e.ID)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// AttackView is one edge of the materialized attack relation, tagged with where
// it came from and whether a preference neutralized it.
type AttackView struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Source  string `json:"source"`          // "objection" | "select_one"
	Defeats bool   `json:"defeats"`         // false if a preference blocks the attack
	Basis   string `json:"basis,omitempty"` // preference basis when blocked
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
			v.Decided = &DecidedView{Position: d.Position, Basis: d.Basis, Decider: d.Decider, Override: d.Override, Supersedes: d.Supersedes}
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
	fmt.Fprintf(&b, "explain %s%s  %q  [%s]\n", ibis.PrefixFor(ibis.Issue), v.Issue, v.IssueText, v.Cardinality)
	if !brief {
		b.WriteString("\nhow this resolves — Dung grounded semantics:\n")
		b.WriteString("  objections and select_one rivalry become ATTACKS; a preference can DEFEAT\n")
		b.WriteString("  (neutralize) an attack; then a fixpoint labels each node IN (justified) /\n")
		b.WriteString("  OUT (defeated) / UNDEC (contested). Positions left IN are the standing answer.\n")
	}

	pre := func(id string) string { return ibis.PrefixFor(v.kinds[id]) + id }
	preList := func(ids []string) string {
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = pre(id)
		}
		return strings.Join(out, ", ")
	}

	b.WriteString("\n1. attacks derived:\n")
	if len(v.Attacks) == 0 {
		b.WriteString("   (none — no objections or select_one rivalry)\n")
	}
	for _, a := range v.Attacks {
		note := a.Source
		if !a.Defeats {
			note += ", neutralized by preference"
			if a.Basis != "" {
				note += " (basis=" + a.Basis + ")"
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

	b.WriteString("\n2. automated reasoning — grounded fixpoint:\n")
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
		fmt.Fprintf(&b, "     %-5s %s  (%s)\n", s.Label, pre(s.Node), reason)
	}
	if len(v.Steps) == 0 {
		b.WriteString("   (no positions or arguments yet)\n")
	}

	b.WriteString("\n3. outcome:\n")
	for _, p := range v.Outcome {
		marker := ""
		if p.Label == "IN" {
			marker = "  ← justified"
		}
		fmt.Fprintf(&b, "   %-5s %s%s  %q\n", p.Label, ibis.PrefixFor(ibis.Position)+p.ID, marker, p.Text)
	}
	if v.Decided != nil {
		if v.DecisionIsIN {
			fmt.Fprintf(&b, "   ✓ decision recorded: %s (matches the justified position)\n", ibis.PrefixFor(ibis.Position)+v.Decided.Position)
		} else {
			fmt.Fprintf(&b, "   ⚠ decision recorded: %s (OVERRIDE — not the justified position)\n", ibis.PrefixFor(ibis.Position)+v.Decided.Position)
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
