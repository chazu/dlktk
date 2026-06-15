package render

import (
	"strings"
	"testing"
)

// Color is off by default (library/pipe-safe) and only emits ANSI once enabled;
// disabling restores plain output. Tests restore the global so they don't leak.
func TestColorTogglesANSI(t *testing.T) {
	defer SetColor(false)

	SetColor(false)
	if got := cID("p:abc"); got != "p:abc" {
		t.Fatalf("color off should be plain, got %q", got)
	}
	SetColor(true)
	got := cID("p:abc")
	if !strings.Contains(got, "\033[") || !strings.Contains(got, "p:abc") {
		t.Fatalf("color on should wrap in ANSI, got %q", got)
	}
}

// visLen counts visible runes only, so column math is unaffected by color codes.
func TestVisLenIgnoresANSI(t *testing.T) {
	defer SetColor(false)
	SetColor(true)
	colored := labelCol("IN") // "✓ IN" padded to visible width 7, wrapped in ANSI
	if visLen(colored) != 7 {
		t.Fatalf("visLen want 7, got %d (%q)", visLen(colored), colored)
	}
	if strings.Contains(colored, "\033[") == false {
		t.Fatal("labelCol should be colored when color is on")
	}
}

// suggestionCommand renders runnable dlktk commands with placeholders.
func TestSuggestionCommand(t *testing.T) {
	cases := map[string]MoveSuggestion{
		`dlktk object p:x "<text>"`:          {Move: "object", Args: []string{"p:x"}},
		`dlktk prefer a b --basis <label>`:   {Move: "prefer", Args: []string{"a", "b"}},
		`dlktk decide i p [--basis <label>]`: {Move: "decide", Args: []string{"i", "p"}},
		`dlktk propose i "<text>"`:           {Move: "propose", Args: []string{"i"}},
	}
	for want, m := range cases {
		if got := suggestionCommand(m); got != want {
			t.Errorf("suggestionCommand(%+v) = %q, want %q", m, got, want)
		}
	}
}

// para wraps body under a hanging indent matching the prefix's visible width.
func TestParaWrapsWithHangingIndent(t *testing.T) {
	defer SetWidth(0)
	SetColor(false)
	SetWidth(20)
	out := para("  X ", "one two three four five")
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %q", out)
	}
	if !strings.HasPrefix(lines[0], "  X one") {
		t.Fatalf("first line wrong: %q", lines[0])
	}
	for _, ln := range lines[1:] {
		if !strings.HasPrefix(ln, "    ") { // 4 = visLen("  X ")
			t.Fatalf("continuation not indented to 4: %q", ln)
		}
	}
}
