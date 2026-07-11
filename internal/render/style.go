package render

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chazu/dlktk/internal/ibis"
)

// Display state for the text renderers. Both are off/zero by default so library
// callers and tests get plain, unwrapped output; the CLI opts in via SetColor /
// SetWidth once it knows the terminal (see cmd/dlktk). Package-level rather than
// threaded because rendering is a single-process, single-invocation concern.
var (
	colorOn   bool
	wrapWidth int
)

// SetColor enables or disables ANSI color in the text renderers.
func SetColor(on bool) { colorOn = on }

// SetWidth sets the wrap width for prose in the text renderers (0 disables
// wrapping). Typically the terminal width.
func SetWidth(w int) { wrapWidth = w }

const (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiDim      = "\033[2m"
	ansiRed      = "\033[31m"
	ansiGreen    = "\033[32m"
	ansiYellow   = "\033[33m"
	ansiCyan     = "\033[36m"
	ansiBlueBold = "\033[1;38;5;39m" // bold azure — the position backbone
	// Bold+color combos for the grounded labels: maximum contrast so a reader
	// tells IN/OUT/UNDEC apart at a glance even on a dense tree.
	ansiInBold    = "\033[1;32m" // bold green
	ansiOutBold   = "\033[1;31m" // bold red
	ansiUndecBold = "\033[1;33m" // bold yellow
)

func paint(code, s string) string {
	if !colorOn || s == "" {
		return s
	}
	return code + s + ansiReset
}

func cDim(s string) string  { return paint(ansiDim, s) }
func cBold(s string) string { return paint(ansiBold, s) }
func cID(s string) string   { return paint(ansiCyan, s) }

// labelColor wraps s in the color for a grounded label (no glyph).
func labelColor(label, s string) string {
	switch label {
	case "IN":
		return paint(ansiGreen, s)
	case "OUT":
		return paint(ansiRed, s)
	case "UNDEC":
		return paint(ansiYellow, s)
	}
	return s
}

// labelColorBold is labelColor at full contrast (bold + color) — used in the
// tree where badges must pop out of a busy outline.
func labelColorBold(label, s string) string {
	switch label {
	case "IN":
		return paint(ansiInBold, s)
	case "OUT":
		return paint(ansiOutBold, s)
	case "UNDEC":
		return paint(ansiUndecBold, s)
	}
	return s
}

// participantPalette is a set of high-contrast 256-color hues, deliberately
// avoiding the green/red/yellow reserved for grounded labels. Each participant
// is assigned one stable hue so the eye can track who said what across the tree.
var participantPalette = []string{
	"\033[38;5;39m",  // azure
	"\033[38;5;208m", // orange
	"\033[38;5;170m", // magenta
	"\033[38;5;45m",  // cyan
	"\033[38;5;213m", // pink
	"\033[38;5;141m", // violet
	"\033[38;5;111m", // periwinkle
	"\033[38;5;215m", // apricot
}

// participantColor maps a participant identity to a stable palette hue. The hash
// is order-stable so the same name always paints the same color within a run.
func participantColor(name string) string {
	h := 0
	for _, r := range name {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return participantPalette[h%len(participantPalette)]
}

// cParticipant paints s in the stable hue assigned to a participant identity.
func cParticipant(name, s string) string { return paint(participantColor(name), s) }

// labelCol returns a fixed-width "<glyph> <NAME>" badge for a grounded label,
// colored. Visible width is 7 ("? UNDEC") so columns align across rows.
func labelCol(label string) string {
	var badge string
	switch label {
	case "IN":
		badge = "✓ IN"
	case "OUT":
		badge = "✗ OUT"
	case "UNDEC":
		badge = "? UNDEC"
	default:
		return fmt.Sprintf("%-7s", label)
	}
	return labelColor(label, badge+strings.Repeat(" ", 7-len([]rune(badge))))
}

// labelInline returns "<glyph> <NAME>" with no padding, colored — for prose.
func labelInline(label string) string {
	switch label {
	case "IN":
		return labelColor(label, "✓ IN")
	case "OUT":
		return labelColor(label, "✗ OUT")
	case "UNDEC":
		return labelColor(label, "? UNDEC")
	}
	return label
}

// nid renders a kind-prefixed node id, colored as an id.
func nid(kind, id string) string { return cID(ibis.PrefixFor(ibis.Kind(kind)) + id) }

// pid renders a position id (the common case in status/explain).
func pid(id string) string { return cID(ibis.PrefixFor(ibis.Position) + id) }

// quote wraps text in double quotes (kept plain so wrapping math stays simple).
func quote(s string) string { return "\"" + s + "\"" }

var ansiRE = regexp.MustCompile("\033\\[[0-9;]*m")

// visLen is the visible (printable) rune count, ignoring ANSI escapes — the
// length that matters for column alignment and wrapping.
func visLen(s string) int { return runeLen(ansiRE.ReplaceAllString(s, "")) }

// para renders prefix followed by body, word-wrapping body to the configured
// width with continuation lines hanging-indented under the start of body. When
// wrapping is disabled it is a plain concatenation. The returned string has no
// trailing newline.
func para(prefix, body string) string {
	if wrapWidth <= 0 {
		return prefix + body
	}
	indent := visLen(prefix)
	avail := wrapWidth - indent
	if avail < 12 {
		return prefix + body
	}
	lines := wrapText(body, avail)
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	for i, ln := range lines {
		if i == 0 {
			b.WriteString(prefix + ln)
		} else {
			b.WriteString(pad + ln)
		}
		if i < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// suggestionCommand renders a move suggestion as a runnable dlktk command line,
// with placeholders for the free-text / basis a human must supply. Ids are left
// bare (the CLI accepts them with or without the i:/p:/a: prefix).
func suggestionCommand(m MoveSuggestion) string {
	parts := append([]string{"dlktk", m.Move}, m.Args...)
	switch m.Move {
	case "object", "support", "propose", "assume", "synthesize":
		parts = append(parts, `"<text>"`)
	case "prefer", "supersede":
		parts = append(parts, "--basis", "<label>")
	case "reframe":
		parts = append(parts, `"<text>"`, "--basis", "<label>")
	case "decide":
		parts = append(parts, "[--basis <label>]")
	}
	return strings.Join(parts, " ")
}

// relTime renders a compact "Nx ago" / "in Nx" relative to now.
func relTime(then, now time.Time) string {
	d := now.Sub(then)
	suffix := "ago"
	if d < 0 {
		d, suffix = -d, "from now"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm %s", int(d.Minutes()), suffix)
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %s", int(d.Hours()), suffix)
	default:
		return fmt.Sprintf("%dd %s", int(d.Hours()/24), suffix)
	}
}
