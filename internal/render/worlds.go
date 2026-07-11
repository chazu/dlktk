// Worlds: the exploration lens over the same framework the grounded referee
// runs on. Grounded collapses a contested issue to "everything UNDEC"; the
// preferred extensions package that residue into its coherent maximal stances
// — the shape of the disagreement, not just the fact of it. Grounded remains
// the sole basis for status/decide/check.
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// World is one coherent maximal stance (a preferred extension), scoped to an
// issue's arguments.
type World struct {
	In []NodeRef `json:"in"`
	// Distinguishing lists the members not shared by every world — what you
	// must additionally accept to live in this one.
	Distinguishing []string `json:"distinguishing"`
}

// WorldsView enumerates the coherent stances on an issue and sorts its
// positions by how they fare across them: robust (IN in every world),
// contingent (IN in some), hopeless (IN in none).
type WorldsView struct {
	Issue        string    `json:"issue"`
	Under        string    `json:"under,omitempty"` // audience lens, when --under is set
	Worlds       []World   `json:"worlds"`
	Robust       []NodeRef `json:"robust"`
	Contingent   []NodeRef `json:"contingent"`
	Hopeless     []NodeRef `json:"hopeless"`
	TooContested bool      `json:"too_contested,omitempty"`
	Note         string    `json:"note,omitempty"`
}

// Worlds enumerates the preferred extensions of the issue's sub-framework.
func Worlds(g *ibis.Graph, fw *af.Framework, issue string) WorldsView {
	v := WorldsView{Issue: issue}
	scope := reachableAF(g, issue)
	sub := fw.Restrict(scope)
	exts, tooContested := sub.PreferredExtensions()
	if tooContested {
		v.TooContested = true
		v.Note = fmt.Sprintf("a contested component exceeds %d arguments; enumeration would not be interactive — resolve part of the deadlock first", af.WorldsMaxComponent)
		return v
	}

	// Membership counts, for distinguishing members and position sorting.
	inCount := map[string]int{}
	for _, ext := range exts {
		for _, id := range ext {
			inCount[id]++
		}
	}
	for _, ext := range exts {
		w := World{}
		for _, id := range ext {
			n := g.Nodes[id]
			w.In = append(w.In, NodeRef{ID: id, Kind: string(n.Kind), Text: n.Text, Label: "IN"})
			if inCount[id] < len(exts) {
				w.Distinguishing = append(w.Distinguishing, id)
			}
		}
		sort.Strings(w.Distinguishing)
		v.Worlds = append(v.Worlds, w)
	}

	for _, p := range positionsFor(g, issue) {
		n := g.Nodes[p]
		ref := NodeRef{ID: p, Kind: string(n.Kind), Text: n.Text}
		switch {
		case inCount[p] == len(exts) && len(exts) > 0:
			ref.Label = "IN"
			v.Robust = append(v.Robust, ref)
		case inCount[p] > 0:
			ref.Label = "UNDEC"
			v.Contingent = append(v.Contingent, ref)
		default:
			ref.Label = "OUT"
			v.Hopeless = append(v.Hopeless, ref)
		}
	}
	if len(v.Worlds) == 1 {
		v.Note = "one coherent stance: the grounded labelling already tells the whole story"
	}
	return v
}

// WorldsText renders a WorldsView.
func WorldsText(v WorldsView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s%s\n", cBold("worlds of"), cID(ibis.PrefixFor(ibis.Issue)+v.Issue),
		cDim(" — the coherent stances a reasonable participant can hold:"))
	if v.TooContested {
		b.WriteString("  " + cDim("too contested: "+v.Note) + "\n")
		return b.String()
	}
	name := 'A'
	for _, w := range v.Worlds {
		distinguishing := map[string]bool{}
		for _, id := range w.Distinguishing {
			distinguishing[id] = true
		}
		fmt.Fprintf(&b, "%s\n", cBold("world "+string(name)))
		for _, n := range w.In {
			mark := "  "
			if distinguishing[n.ID] {
				mark = cBold("* ")
			}
			b.WriteString(para(fmt.Sprintf("  %s%s %s  ", mark, labelInline("IN"), nid(n.Kind, n.ID)), quote(n.Text)) + "\n")
		}
		name++
	}
	section := func(header string, refs []NodeRef) {
		if len(refs) == 0 {
			return
		}
		b.WriteString(cBold(header) + "\n")
		for _, n := range refs {
			b.WriteString(para(fmt.Sprintf("  %s  ", nid(n.Kind, n.ID)), quote(n.Text)) + "\n")
		}
	}
	section("robust (IN in every world — safe to build on):", v.Robust)
	section("contingent (IN in some worlds — the live disagreement):", v.Contingent)
	section("hopeless (IN in none):", v.Hopeless)
	if v.Note != "" {
		b.WriteString(cDim(v.Note) + "\n")
	}
	if len(v.Worlds) > 1 {
		b.WriteString(cDim("* marks what a world accepts that not all worlds do") + "\n")
	}
	return b.String()
}
