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

// Tested reports whether a position has faced at least one substantive attack:
// an objects_to edge that (1) is authored by someone other than the position's
// author and (2) participates in the defeat relation — if it no longer wins,
// it was beaten by a counter-argument, not excused by a preference (a
// preference-neutralized objection is dropped from Defeat at build time, so a
// later `prefer` re-arms untested-ness). select_one rival edges never count:
// on any multi-position issue every rival is trivially "attacked", which is
// not examination. Computed from the raw links plus fw.Defeat — the merged
// attack set has no provenance. An unattributed objection (empty author) gets
// the benefit of the doubt; only provable self-dealing is excluded, since
// authorship is attribution, not identity (wicked-problems-2.md item 1).
func Tested(g *ibis.Graph, fw *af.Framework, position string) bool {
	defeats := map[[2]string]bool{}
	for _, e := range fw.Defeat {
		defeats[[2]string{e.From, e.To}] = true
	}
	posAuthor := g.Nodes[position].Author
	for _, l := range g.Links {
		if l.Rel != ibis.ObjectsTo || l.Dst != position {
			continue
		}
		if a := g.Nodes[l.Src].Author; a != "" && a == posAuthor {
			continue // self-objection: the one mind testing itself
		}
		if defeats[[2]string{l.Src, position}] {
			return true
		}
	}
	return false
}

// PositionView is one position's status within an issue. Untested marks a
// live (IN or UNDEC) label that never faced a substantive objection (see
// Tested): "justified" then means "unexamined", not "vindicated".
type PositionView struct {
	ID         string   `json:"id"`
	Text       string   `json:"text"`
	Label      string   `json:"label"`
	AttackedBy []string `json:"attacked_by"`
	DefeatedBy []string `json:"defeated_by"`
	Reinstated bool     `json:"reinstated"`
	Untested   bool     `json:"untested,omitempty"`
}

// DecidedView is the standing decision on an issue, if any.
type DecidedView struct {
	Position   string `json:"position"`
	Basis      string `json:"basis,omitempty"`
	Decider    string `json:"decider"`
	Override   bool   `json:"override"`
	Supersedes string `json:"supersedes,omitempty"`
	ReviewBy   int64  `json:"review_by,omitempty"`
}

// IssueStatus is the status envelope for one issue (design §8.3).
type IssueStatus struct {
	Issue       string         `json:"issue"`
	IssueText   string         `json:"issue_text"`
	Cardinality string         `json:"cardinality"`
	Under       string         `json:"under,omitempty"` // audience lens, when --under is set
	Positions   []PositionView `json:"positions"`
	Undecided   []string       `json:"undecided"`
	Stalemate   bool           `json:"stalemate"`
	Advice      string         `json:"advice"`
	ReframedTo  string         `json:"reframed_to,omitempty"` // this framing was replaced
	Decided     *DecidedView   `json:"decided,omitempty"`     // the single standing decision (select_one)
	Decisions   []DecidedView  `json:"decisions,omitempty"`   // every standing decision (open cardinality records one per position)
}

// Status computes the status of one issue. decs are the in-force decisions at
// the current viewpoint (used to surface a standing decision).
func Status(g *ibis.Graph, fw *af.Framework, labels map[string]af.Label, issue string, decs []ibis.Decision) IssueStatus {
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
			Untested:   (lbl == af.IN || lbl == af.UNDEC) && !Tested(g, fw, p),
		})
		if lbl == af.UNDEC {
			st.Undecided = append(st.Undecided, p)
		}
	}
	if to, ok := g.ReframedTo(issue); ok {
		st.ReframedTo = to
	}
	st.Stalemate = isStalemate(st)
	st.Advice = advise(st)
	// An open issue records a standing decision per position (multiple winners
	// that compose); a select_one issue has at most one. Decisions holds every
	// standing decision; Decided points at the sole one for select_one, so
	// existing single-decision consumers are unchanged.
	for _, d := range decs {
		if d.Issue == issue {
			st.Decisions = append(st.Decisions, DecidedView{Position: d.Position, Basis: d.Basis, Decider: d.Decider, Override: d.Override, Supersedes: d.Supersedes, ReviewBy: d.ReviewBy})
		}
	}
	if card != string(ibis.Open) && len(st.Decisions) > 0 {
		st.Decided = &st.Decisions[len(st.Decisions)-1]
	}
	return st
}

// isStalemate reports a mutual-attack stalemate: every position is UNDEC and none
// is defeated-OUT, so no new argument on these nodes can break the deadlock — only
// a preference (or an argument attacking from outside) can. Covers odd cycles and
// even mutual attacks alike without enumerating cycle parity (design §16 Q3).
func isStalemate(st IssueStatus) bool {
	if len(st.Positions) < 2 {
		return false
	}
	for _, p := range st.Positions {
		if p.Label != "UNDEC" {
			return false
		}
	}
	return true
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
	var untested []string
	for _, p := range st.Positions {
		if p.Untested {
			untested = append(untested, p.ID)
		}
	}
	switch {
	case st.ReframedTo != "":
		return fmt.Sprintf("reframed → %s (this framing was replaced; argue there)", st.ReframedTo)
	case len(st.Positions) == 0:
		return "no positions yet; propose one"
	case len(in) == 1 && len(undec) == 0 && len(untested) == 1:
		return fmt.Sprintf("%s justified — but untested (no substantive objection has engaged it); stress-test it before deciding", in[0])
	case len(in) == 1 && len(undec) == 0:
		return fmt.Sprintf("%s justified", in[0])
	case st.Stalemate && len(untested) > 0:
		// Untested rivals must not be offered `prefer` as a co-equal exit: one
		// ordering, not two options (wicked-problems-2.md item 1).
		return fmt.Sprintf("mutual stalemate — %s all UNDEC, none defeated; %s untested (rival edges only, no substantive objection) — object first, then prefer; a synthesis of the rivals may transcend it, and a reframe is worth considering if this is a false dichotomy (a new argument helps only if it defeats from outside the stalemate)", strings.Join(undec, " vs "), strings.Join(untested, ", "))
	case st.Stalemate:
		return fmt.Sprintf("mutual stalemate — %s all UNDEC, none defeated; a preference resolves it, a synthesis of the rivals may transcend it, and a reframe is worth considering if this is a false dichotomy (a new argument helps only if it defeats from outside the stalemate)", strings.Join(undec, " vs "))
	case len(undec) >= 2 && len(in) == 0:
		return fmt.Sprintf("%s contested — needs a preference, a defeating argument, or a synthesis", strings.Join(undec, " vs "))
	case len(in) >= 1:
		return fmt.Sprintf("%s justified", strings.Join(in, ", "))
	default:
		return "contested; add an argument or preference to resolve"
	}
}

// StatusText renders an issue status as human text.
func StatusText(st IssueStatus) string {
	var b strings.Builder
	lens := ""
	if st.Under != "" {
		lens = cDim(" [under audience " + st.Under + "]")
	}
	fmt.Fprintf(&b, "%s %s  %s  %s%s\n",
		cBold("issue"), cID(ibis.PrefixFor(ibis.Issue)+st.Issue), quote(st.IssueText), cDim("["+st.Cardinality+"]"), lens)
	if len(st.Positions) == 0 {
		b.WriteString("  " + cDim("(no positions yet — propose one)") + "\n")
	}
	for _, p := range st.Positions {
		prefix := fmt.Sprintf("  %s %s  ", labelCol(p.Label), pid(p.ID))
		b.WriteString(para(prefix, quote(p.Text)) + "\n")
		if note := positionNote(p); note != "" {
			b.WriteString(strings.Repeat(" ", visLen(prefix)) + cDim(note) + "\n")
		}
	}
	for i := range st.Decisions {
		d := st.Decisions[i]
		ptext := ""
		for _, p := range st.Positions {
			if p.ID == d.Position {
				ptext = p.Text
			}
		}
		flag := ""
		if d.Override {
			flag += cDim(" (OVERRIDE — was not IN at decision time)")
		}
		if d.Supersedes != "" {
			flag += cDim(fmt.Sprintf(" (supersedes %s)", d.Supersedes))
		}
		fmt.Fprintf(&b, "  %s: %s", labelColor("IN", "✓ decided"), pid(d.Position))
		if ptext != "" {
			fmt.Fprintf(&b, "  %s", quote(ptext))
		}
		fmt.Fprintf(&b, "  %s%s\n", cDim("by "+d.Decider), flag)
	}
	fmt.Fprintf(&b, "  %s %s\n", cBold("→"), st.Advice)
	return b.String()
}

// positionNote explains a position's label in one short, dimmable phrase.
func positionNote(p PositionView) string {
	switch p.Label {
	case "IN":
		// Untested outranks reinstated: a winner whose only attackers were
		// rivals (or excused objections) is unexamined, whatever beat them.
		if p.Untested {
			return "justified — but untested (no substantive objection has engaged it)"
		}
		if p.Reinstated {
			return "reinstated — an attacker was itself defeated"
		}
		return "justified — no surviving attacker"
	case "OUT":
		if len(p.DefeatedBy) > 0 {
			return "defeated by " + joinIDs(p.DefeatedBy)
		}
		return "defeated"
	case "UNDEC":
		if p.Untested {
			return "contested and untested — object first, then prefer"
		}
		return "contested — needs an argument or preference"
	}
	return ""
}

// joinIDs renders a comma-separated id list, each colored as an id.
func joinIDs(ids []string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = cID(id)
	}
	return strings.Join(out, ", ")
}

// JSON marshals any view as indented JSON.
func JSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TreeOpts controls how Tree renders the IBIS graph.
type TreeOpts struct {
	Labels   bool // annotate positions/arguments with grounded label + decided/reinstated markers
	Authors  bool // also reveal each participant's stable author id alongside their role
	NoWho    bool // suppress participant attribution (role/identity) entirely
	ASCII    bool // use ASCII connectors/glyphs instead of Unicode (dumb terminals)
	NoWrap   bool // one line per node, truncated to width (dense overview) instead of wrapping full text
	NoIDs    bool // omit the node id suffix
	NoLegend bool // suppress the glyph legend header
	Width    int  // max line width for wrapping/truncation; 0 falls back to 100
}

// childEdge is one downward link from a node, kept in graph (insertion) order.
type childEdge struct {
	src string
	rel ibis.Rel
}

// treeWriter carries the per-render state for the recursive outline.
type treeWriter struct {
	b             *strings.Builder
	g             *ibis.Graph
	opts          TreeOpts
	labels        map[string]af.Label
	decided       map[string]bool   // position id -> decided in its favour
	issueDecision map[string]string // issue id -> the position decided for it
	reinstated    map[string]bool
	roster        map[string]string // author id -> role (persona) within this discussion
	seen          map[string]bool
	width         int
}

// Tree renders the IBIS graph rooted at the given issue (or all root issues if
// issue == "") as an indented outline. Pass fw/labels/decs (any may be nil) to
// enable opts.Labels annotations; structure-only renders can pass nil.
func Tree(g *ibis.Graph, issue string, opts TreeOpts, fw *af.Framework, labels map[string]af.Label, decs []ibis.Decision, rosters []ibis.Roster) string {
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

	decided := map[string]bool{}
	issueDecision := map[string]string{}
	for _, d := range decs {
		decided[d.Position] = true
		issueDecision[d.Issue] = d.Position
	}
	// Reinstated = IN despite having attackers (a defeater was itself defeated).
	reinstated := map[string]bool{}
	if fw != nil && labels != nil {
		attacked := map[string]bool{}
		for _, e := range fw.Attack {
			attacked[e.To] = true
		}
		for id, l := range labels {
			if l == af.IN && attacked[id] {
				reinstated[id] = true
			}
		}
	}

	roster := map[string]string{}
	for _, r := range rosters {
		roster[r.Author] = r.Role
	}

	width := opts.Width
	if width == 0 {
		width = 100
	}
	tw := &treeWriter{
		b: &strings.Builder{}, g: g, opts: opts, labels: labels,
		decided: decided, issueDecision: issueDecision, reinstated: reinstated,
		roster: roster, seen: map[string]bool{}, width: width,
	}
	if !opts.NoLegend {
		tw.b.WriteString(tw.legend() + "\n")
	}
	for i, r := range roots {
		tw.write(r, "", true, i == len(roots)-1, "")
	}
	return tw.b.String()
}

// legend describes the glyphs in use, adapting to ASCII and --labels. Glyphs are
// shown in their live colors so the legend doubles as a color key.
func (t *treeWriter) legend() string {
	parts := []string{
		cBold(t.glyph("◇", "#")) + " issue",
		t.relGlyph(ibis.RespondsTo) + "position",
		t.relGlyph(ibis.ObjectsTo) + "objects-to",
		t.relGlyph(ibis.Supports) + "supports",
		t.relGlyph(ibis.RaisedFrom) + "raised-from",
		cBold(t.glyph("★", "@")) + " decided",
	}
	if t.opts.Labels {
		parts = append(parts,
			labelColorBold("IN", t.glyph("✓", "+")+" IN"),
			labelColorBold("OUT", t.glyph("✗", "-")+" OUT"),
			labelColorBold("UNDEC", "? UNDEC"),
			labelColorBold("IN", t.glyph("↩", "^")+" reinstated"),
		)
	}
	out := cDim("legend: ") + strings.Join(parts, "  ")
	if t.showsWho() {
		lead := "        "
		if colorOn {
			lead += "each participant has a stable color · "
		}
		out += "\n" + cDim(lead) + t.glyph("▸ ", "by ") + "role" + cDim(" ⟨identity⟩")
	}
	return out
}

// showsWho reports whether participant attribution is rendered. Off only when
// the caller opts out or no node carries an author.
func (t *treeWriter) showsWho() bool {
	if t.opts.NoWho {
		return false
	}
	for _, n := range t.g.Nodes {
		if n.Author != "" {
			return true
		}
	}
	return false
}

// write emits one node and recurses into its children. prefix is the accumulated
// ancestor gutter; rel is the relation linking this node to its parent ("" at a root).
func (t *treeWriter) write(node, prefix string, isRoot, isLast bool, rel ibis.Rel) {
	connector, childGutter := "", prefix
	if !isRoot {
		if isLast {
			connector = cDim(t.glyph("└─ ", "`- "))
			childGutter = prefix + "   "
		} else {
			connector = cDim(t.glyph("├─ ", "|- "))
			childGutter = prefix + cDim(t.glyph("│  ", "|  "))
		}
	}

	if t.seen[node] {
		t.b.WriteString(prefix + connector + t.relGlyph(rel) + "↪ " + t.idSuffix(node) + " (above)\n")
		return
	}
	t.seen[node] = true

	t.writeBody(node, rel, prefix, connector, childGutter)

	children := childrenOf(t.g, node)
	for i, c := range children {
		t.write(c.src, childGutter, false, i == len(children)-1, c.rel)
	}
}

// writeBody emits a node's content. The relation glyph + label badge form the
// head; the (possibly multi-line, word-wrapped) text follows, with continuation
// lines aligned under the first text column and the subtree gutter preserved so
// vertical connectors stay unbroken. The id/author tail trails the final line.
func (t *treeWriter) writeBody(node string, rel ibis.Rel, prefix, connector, childGutter string) {
	n := t.g.Nodes[node]
	head := t.relGlyph(rel)
	if n.Kind == ibis.Issue {
		head = cBold(t.glyph("◇ ", "# "))
	}
	head += t.badge(node)
	if n.Kind == ibis.Issue {
		if card := t.g.IssueCards[node]; card != "" {
			head += cDim("[" + string(card) + "] ")
		}
	}
	if n.Tag == ibis.TagAssumption {
		head += cDim("[assumption] ")
	}

	tail := ""
	if !t.opts.NoIDs {
		tail += "  " + t.idSuffix(node)
	}
	if who := t.attribution(n); who != "" {
		tail += "  " + who
	}
	// Synthesis lineage: the hybrid names its parents inline, and what it
	// recorded as dropped (a synthesis that drops nothing is a bundle).
	if parents := synthesizedFrom(t.g, node); len(parents) > 0 {
		for i, p := range parents {
			parents[i] = pid(p)
		}
		tail += "  " + cDim(t.glyph("⊕ from ", "(+) from ")) + strings.Join(parents, cDim("+"))
		if drops := n.Drops; len(drops) > 0 {
			tail += "  " + cDim(t.glyph("⊖ drops: ", "(-) drops: ")+strings.Join(drops, " · "))
		}
	}
	// On the issue line, name the standing decision so it is visible without
	// scanning the subtree for the ★-marked position.
	if pos, ok := t.issueDecision[node]; ok {
		tail += "  " + labelColorBold("IN", t.glyph("✓", "v")+" decided: ") + pid(pos)
	}
	// Reframe lineage: a replaced framing points at its successor.
	if n.Kind == ibis.Issue {
		if to, ok := t.g.ReframedTo(node); ok {
			tail += "  " + cDim(t.glyph("↻ reframed → ", "~> reframed: ")) + cID(ibis.PrefixFor(ibis.Issue)+to)
		}
	}

	// Text column starts after prefix + connector + head; continuation lines
	// align there, replacing the connector with the subtree gutter. Widths are
	// measured ignoring ANSI color so alignment holds when color is on.
	avail := t.width - visLen(prefix) - visLen(connector) - visLen(head)
	cont := childGutter + strings.Repeat(" ", visLen(head))

	var lines []string
	if t.opts.NoWrap {
		lines = []string{truncate(n.Text, avail)}
	} else {
		lines = wrapText(n.Text, avail)
	}
	// The id/author tail trails the final text line, but only if it fits the
	// width; otherwise it drops to its own aligned line so nothing overflows.
	indent := visLen(prefix) + visLen(connector) + visLen(head)
	last := len(lines) - 1
	inlineTail := tail != "" && (t.opts.NoWrap ||
		indent+runeLen(lines[last])+visLen(tail) <= t.width)

	for i, ln := range lines {
		if i == 0 {
			t.b.WriteString(prefix + connector + head + ln)
		} else {
			t.b.WriteString(cont + ln)
		}
		if i == last && inlineTail {
			t.b.WriteString(tail)
		}
		t.b.WriteByte('\n')
	}
	if tail != "" && !inlineTail {
		t.b.WriteString(cont + strings.TrimLeft(tail, " ") + "\n")
	}
}

// badge renders the per-node markers: the grounded label (IN/OUT/UNDEC) and the
// reinstated mark only under --labels; the ★ "decided in favour of" mark always,
// so a reader sees which position carried an issue without opting into labels.
func (t *treeWriter) badge(node string) string {
	s := ""
	if t.opts.Labels {
		switch t.labels[node] {
		case af.IN:
			s = labelColorBold("IN", t.glyph("✓ ", "+ "))
		case af.OUT:
			s = labelColorBold("OUT", t.glyph("✗ ", "- "))
		case af.UNDEC:
			s = labelColorBold("UNDEC", "? ")
		}
	}
	if t.decided[node] {
		s += cBold(t.glyph("★ ", "@ "))
	}
	if t.opts.Labels && t.reinstated[node] {
		s += labelColorBold("IN", t.glyph("↩ ", "^ "))
	}
	return s
}

// attribution renders who is responsible for a node: the role (persona) they
// moved under, with their stable identity in dim when it adds information or the
// reader opted in with --authors. Each participant keeps a stable color so the
// same voice is the same hue everywhere in the tree. Empty when there is nothing
// to show or the reader opted out with --no-who.
func (t *treeWriter) attribution(n ibis.Node) string {
	if t.opts.NoWho || n.Author == "" {
		return ""
	}
	role := t.roster[n.Author]
	persona := role
	if persona == "" {
		persona = n.Author // no role bound — the author is the identity we have
	}
	out := cParticipant(n.Author, t.glyph("▸ ", "by ")+persona)
	// Append the stable identity when it carries information the role doesn't:
	// a distinct author behind the persona, or an explicit --authors request.
	if role != "" && (t.opts.Authors || role != n.Author) {
		out += " " + cDim("⟨"+n.Author+"⟩")
	}
	return out
}

func (t *treeWriter) idSuffix(node string) string {
	return cID(ibis.PrefixFor(t.g.Nodes[node].Kind) + node)
}

func (t *treeWriter) relGlyph(rel ibis.Rel) string {
	switch rel {
	case ibis.ObjectsTo:
		return labelColorBold("OUT", t.glyph("⚔ ", "x ")) // red: an attack
	case ibis.Supports:
		return labelColorBold("IN", "+ ") // green: reinforcement
	case ibis.RespondsTo:
		return paint(ansiBlueBold, t.glyph("• ", "* ")) // blue: a stance on the issue
	case ibis.RaisedFrom:
		return cDim(t.glyph("⤷ ", "~ ")) // a deeper question this node revealed
	}
	return ""
}

// synthesizedFrom returns the parent positions a hybrid was recombined from,
// sorted, or nil.
func synthesizedFrom(g *ibis.Graph, node string) []string {
	return g.SynthesisParents(node)
}

func (t *treeWriter) glyph(unicode, ascii string) string {
	if t.opts.ASCII {
		return ascii
	}
	return unicode
}

// childrenOf returns the downward IBIS links from node, in graph order.
// raised_from nests a spawned sub-issue under the node that revealed it;
// synthesizes is lineage, not nesting (the hybrid already renders under its
// issue).
func childrenOf(g *ibis.Graph, node string) []childEdge {
	var out []childEdge
	for _, l := range g.Links {
		if l.Dst == node && (l.Rel == ibis.RespondsTo || l.Rel == ibis.Supports || l.Rel == ibis.ObjectsTo || l.Rel == ibis.RaisedFrom) {
			out = append(out, childEdge{src: l.Src, rel: l.Rel})
		}
	}
	return out
}

func runeLen(s string) int { return len([]rune(s)) }

// truncate shortens s to at most n visible runes, adding an ellipsis. If n is too
// small to be useful the full text is returned (better wide than blank).
func truncate(s string, n int) string {
	if n < 8 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// wrapText word-wraps s to lines of at most width visible runes, hard-breaking
// any single word longer than width. Whitespace is collapsed (prose, not code).
// A too-small width falls back to a single unwrapped line.
func wrapText(s string, width int) []string {
	if width < 12 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, w := range words {
		for runeLen(w) > width { // hard-break an over-long token
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			r := []rune(w)
			lines = append(lines, string(r[:width]))
			w = string(r[width:])
		}
		switch {
		case cur == "":
			cur = w
		case runeLen(cur)+1+runeLen(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
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

// respondsToSomething reports whether a node hangs off another node in the
// tree — via responds_to (a sub-issue/position) or raised_from (an issue
// spawned by an argument). Such nodes are not roots: they render nested.
func respondsToSomething(g *ibis.Graph, node string) bool {
	for _, l := range g.Links {
		if l.Src == node && (l.Rel == ibis.RespondsTo || l.Rel == ibis.RaisedFrom) {
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
