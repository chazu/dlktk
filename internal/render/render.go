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

// DecidedView is the standing decision on an issue, if any.
type DecidedView struct {
	Position string `json:"position"`
	Basis    string `json:"basis,omitempty"`
	Decider  string `json:"decider"`
	Override bool   `json:"override"`
}

// IssueStatus is the status envelope for one issue (design §8.3).
type IssueStatus struct {
	Issue       string         `json:"issue"`
	IssueText   string         `json:"issue_text"`
	Cardinality string         `json:"cardinality"`
	Positions   []PositionView `json:"positions"`
	Undecided   []string       `json:"undecided"`
	Stalemate   bool           `json:"stalemate"`
	Advice      string         `json:"advice"`
	Decided     *DecidedView   `json:"decided,omitempty"`
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
		})
		if lbl == af.UNDEC {
			st.Undecided = append(st.Undecided, p)
		}
	}
	st.Stalemate = isStalemate(st)
	st.Advice = advise(st)
	for _, d := range decs {
		if d.Issue == issue {
			st.Decided = &DecidedView{Position: d.Position, Basis: d.Basis, Decider: d.Decider, Override: d.Override}
		}
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
	switch {
	case len(st.Positions) == 0:
		return "no positions yet; propose one"
	case len(in) == 1 && len(undec) == 0:
		return fmt.Sprintf("%s justified", in[0])
	case st.Stalemate:
		return fmt.Sprintf("mutual stalemate — %s all UNDEC, none defeated; a preference resolves this (a new argument helps only if it defeats from outside the stalemate)", strings.Join(undec, " vs "))
	case len(undec) >= 2 && len(in) == 0:
		return fmt.Sprintf("%s contested — needs a preference or a defeating argument", strings.Join(undec, " vs "))
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
	if st.Decided != nil {
		flag := ""
		if st.Decided.Override {
			flag = " (OVERRIDE — was not IN at decision time)"
		}
		fmt.Fprintf(&b, "  ✓ decided: %s by %s%s\n", st.Decided.Position, st.Decided.Decider, flag)
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

// TreeOpts controls how Tree renders the IBIS graph.
type TreeOpts struct {
	Labels  bool // annotate positions/arguments with grounded label + decided/reinstated markers
	Authors bool // append the node author
	ASCII   bool // use ASCII connectors/glyphs instead of Unicode (dumb terminals)
	NoWrap  bool // one line per node, truncated to width (dense overview) instead of wrapping full text
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
	seen          map[string]bool
	width         int
}

// Tree renders the IBIS graph rooted at the given issue (or all root issues if
// issue == "") as an indented outline. Pass fw/labels/decs (any may be nil) to
// enable opts.Labels annotations; structure-only renders can pass nil.
func Tree(g *ibis.Graph, issue string, opts TreeOpts, fw *af.Framework, labels map[string]af.Label, decs []ibis.Decision) string {
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

	width := opts.Width
	if width == 0 {
		width = 100
	}
	tw := &treeWriter{
		b: &strings.Builder{}, g: g, opts: opts, labels: labels,
		decided: decided, issueDecision: issueDecision, reinstated: reinstated,
		seen: map[string]bool{}, width: width,
	}
	if !opts.NoLegend {
		tw.b.WriteString(tw.legend() + "\n")
	}
	for i, r := range roots {
		tw.write(r, "", true, i == len(roots)-1, "")
	}
	return tw.b.String()
}

// legend describes the glyphs in use, adapting to ASCII and --labels.
func (t *treeWriter) legend() string {
	parts := []string{
		t.glyph("◇", "#") + " issue",
		t.glyph("•", "*") + " position",
		t.glyph("⚔", "x") + " objects-to",
		"+ supports",
		t.glyph("★", "@") + " decided",
	}
	if t.opts.Labels {
		parts = append(parts,
			t.glyph("✓", "+")+" IN",
			t.glyph("✗", "-")+" OUT",
			"? UNDEC",
			t.glyph("↩", "^")+" reinstated",
		)
	}
	return "legend: " + strings.Join(parts, "  ")
}

// write emits one node and recurses into its children. prefix is the accumulated
// ancestor gutter; rel is the relation linking this node to its parent ("" at a root).
func (t *treeWriter) write(node, prefix string, isRoot, isLast bool, rel ibis.Rel) {
	connector, childGutter := "", prefix
	if !isRoot {
		if isLast {
			connector = t.glyph("└─ ", "`- ")
			childGutter = prefix + "   "
		} else {
			connector = t.glyph("├─ ", "|- ")
			childGutter = prefix + t.glyph("│  ", "|  ")
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
		head = t.glyph("◇ ", "# ")
	}
	head += t.badge(node)
	if n.Kind == ibis.Issue {
		if card := t.g.IssueCards[node]; card != "" {
			head += "[" + string(card) + "] "
		}
	}

	tail := ""
	if !t.opts.NoIDs {
		tail += "  " + t.idSuffix(node)
	}
	if t.opts.Authors && n.Author != "" {
		tail += " " + n.Author
	}
	// On the issue line, name the standing decision so it is visible without
	// scanning the subtree for the ★-marked position.
	if pos, ok := t.issueDecision[node]; ok {
		tail += "  " + t.glyph("✓", "v") + " decided: " + ibis.PrefixFor(ibis.Position) + pos
	}

	// Text column starts after prefix + connector + head; continuation lines
	// align there, replacing the connector with the subtree gutter.
	avail := t.width - runeLen(prefix) - runeLen(connector) - runeLen(head)
	cont := childGutter + strings.Repeat(" ", runeLen(head))

	var lines []string
	if t.opts.NoWrap {
		lines = []string{truncate(n.Text, avail)}
	} else {
		lines = wrapText(n.Text, avail)
	}
	// The id/author tail trails the final text line, but only if it fits the
	// width; otherwise it drops to its own aligned line so nothing overflows.
	indent := runeLen(prefix) + runeLen(connector) + runeLen(head)
	last := len(lines) - 1
	inlineTail := tail != "" && (t.opts.NoWrap ||
		indent+runeLen(lines[last])+runeLen(tail) <= t.width)

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
			s = t.glyph("✓ ", "+ ")
		case af.OUT:
			s = t.glyph("✗ ", "- ")
		case af.UNDEC:
			s = "? "
		}
	}
	if t.decided[node] {
		s += t.glyph("★ ", "@ ")
	}
	if t.opts.Labels && t.reinstated[node] {
		s += t.glyph("↩ ", "^ ")
	}
	return s
}

func (t *treeWriter) idSuffix(node string) string {
	return ibis.PrefixFor(t.g.Nodes[node].Kind) + node
}

func (t *treeWriter) relGlyph(rel ibis.Rel) string {
	switch rel {
	case ibis.ObjectsTo:
		return t.glyph("⚔ ", "x ")
	case ibis.Supports:
		return "+ "
	case ibis.RespondsTo:
		return t.glyph("• ", "* ")
	}
	return ""
}

func (t *treeWriter) glyph(unicode, ascii string) string {
	if t.opts.ASCII {
		return ascii
	}
	return unicode
}

// childrenOf returns the downward IBIS links from node, in graph order.
func childrenOf(g *ibis.Graph, node string) []childEdge {
	var out []childEdge
	for _, l := range g.Links {
		if l.Dst == node && (l.Rel == ibis.RespondsTo || l.Rel == ibis.Supports || l.Rel == ibis.ObjectsTo) {
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
