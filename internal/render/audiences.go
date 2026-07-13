// Audiences: sensitivity analysis over value rankings. Stakeholders judge a
// wicked problem against different value orders and there is no ultimate test;
// this read computes which conclusions survive every declared ranking (act on
// those) and which hinge on whose values govern (that is the real decision —
// name it and argue it).
package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// AudienceRef is one declared value ranking.
type AudienceRef struct {
	Name    string   `json:"name"`
	Ranking []string `json:"ranking"`
	Author  string   `json:"author,omitempty"`
}

// SensitivePosition is a position whose verdict depends on the audience.
type SensitivePosition struct {
	Position string            `json:"position"`
	Text     string            `json:"text"`
	Verdicts map[string]string `json:"verdicts"` // audience name (or "baseline") -> label
}

// AudienceIssue is one issue's labelling across every audience.
type AudienceIssue struct {
	Issue      string                       `json:"issue"`
	Text       string                       `json:"text"`
	Baseline   map[string]string            `json:"baseline"`    // position -> label with no audience lens
	ByAudience map[string]map[string]string `json:"by_audience"` // audience -> position -> label
	Robust     []string                     `json:"robust"`      // IN under baseline and every audience
	Sensitive  []SensitivePosition          `json:"sensitive"`
}

// AudiencesView is the cross-audience robustness report.
type AudiencesView struct {
	Audiences []AudienceRef   `json:"audiences"`
	Issues    []AudienceIssue `json:"issues"`
}

// Audiences computes the labelling under every declared audience and reports
// robust vs audience-sensitive positions per issue. Reframed issues are
// excluded (the question moved).
func Audiences(g *ibis.Graph) (AudiencesView, error) {
	var v AudiencesView
	names := make([]string, 0, len(g.Audiences))
	for name := range g.Audiences {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		a := g.Audiences[name]
		v.Audiences = append(v.Audiences, AudienceRef{Name: a.Name, Ranking: a.Ranking, Author: a.Author})
	}

	baseFw, err := af.Build(g)
	if err != nil {
		return AudiencesView{}, err
	}
	baseline := baseFw.Grounded()

	byAudience := map[string]map[string]af.Label{}
	for _, name := range names {
		fw, err := af.BuildUnder(g, g.Audiences[name])
		if err != nil {
			return AudiencesView{}, err
		}
		byAudience[name] = fw.Grounded()
	}

	for _, issue := range g.Issues() {
		if _, ok := g.ReframedTo(issue); ok {
			continue
		}
		positions := positionsFor(g, issue)
		if len(positions) == 0 {
			continue
		}
		ai := AudienceIssue{
			Issue: issue, Text: g.Nodes[issue].Text,
			Baseline:   map[string]string{},
			ByAudience: map[string]map[string]string{},
		}
		for _, name := range names {
			ai.ByAudience[name] = map[string]string{}
		}
		for _, p := range positions {
			ai.Baseline[p] = string(baseline[p])
			robust := baseline[p] == af.IN
			uniform := true
			verdicts := map[string]string{"baseline": string(baseline[p])}
			for _, name := range names {
				l := byAudience[name][p]
				ai.ByAudience[name][p] = string(l)
				verdicts[name] = string(l)
				if l != af.IN {
					robust = false
				}
				if l != baseline[p] {
					uniform = false
				}
			}
			switch {
			case robust:
				ai.Robust = append(ai.Robust, p)
			case !uniform:
				ai.Sensitive = append(ai.Sensitive, SensitivePosition{Position: p, Text: g.Nodes[p].Text, Verdicts: verdicts})
			}
		}
		v.Issues = append(v.Issues, ai)
	}
	return v, nil
}

// IssueMap computes one issue's audience-conditional verdict map: every
// position's label under baseline and under each declared audience, in a
// deterministic canonical string. sensitive reports whether the issue is
// audience-sensitive *right now* — at least two declared audiences and at least
// one position whose verdict differs across them (or from baseline) — which is
// the precondition for closing the issue with a value-map decision
// (wicked-problems-2.md item 7). The canonical string is what map_drift
// compares across time; no verdicts are stored, so it is recomputed on demand.
func IssueMap(g *ibis.Graph, issue string) (canonical string, sensitive bool, err error) {
	names := make([]string, 0, len(g.Audiences))
	for name := range g.Audiences {
		names = append(names, name)
	}
	sort.Strings(names)

	baseFw, err := af.Build(g)
	if err != nil {
		return "", false, err
	}
	baseline := baseFw.Grounded()
	byAudience := map[string]map[string]af.Label{}
	for _, name := range names {
		fw, e := af.BuildUnder(g, g.Audiences[name])
		if e != nil {
			return "", false, e
		}
		byAudience[name] = fw.Grounded()
	}

	positions := positionsFor(g, issue)
	sort.Strings(positions)
	var b strings.Builder
	differs := false
	for _, p := range positions {
		fmt.Fprintf(&b, "%s:baseline=%s", p, baseline[p])
		for _, name := range names {
			l := byAudience[name][p]
			fmt.Fprintf(&b, ",%s=%s", name, l)
			if l != baseline[p] {
				differs = true
			}
		}
		b.WriteByte(';')
	}
	return b.String(), len(names) >= 2 && differs, nil
}

// AudiencesText renders an AudiencesView.
func AudiencesText(v AudiencesView) string {
	var b strings.Builder
	if len(v.Audiences) == 0 {
		return cDim("no audiences declared — `dlktk audience <name> <value>...` records a stakeholder's value ranking") + "\n"
	}
	b.WriteString(cBold("audiences:") + "\n")
	for _, a := range v.Audiences {
		fmt.Fprintf(&b, "  %s  %s\n", cID(a.Name), cDim(strings.Join(a.Ranking, " ≻ ")))
	}
	for _, ai := range v.Issues {
		b.WriteString(para(fmt.Sprintf("%s %s  ", cBold("issue"), cID(ibis.PrefixFor(ibis.Issue)+ai.Issue)), quote(ai.Text)) + "\n")
		if len(ai.Robust) > 0 {
			ids := make([]string, len(ai.Robust))
			for i, p := range ai.Robust {
				ids[i] = pid(p)
			}
			fmt.Fprintf(&b, "  %s %s  %s\n", labelInline("IN"), strings.Join(ids, ", "), cDim("robust — justified under every declared ranking"))
		}
		for _, s := range ai.Sensitive {
			b.WriteString(para(fmt.Sprintf("  %s %s  ", labelInline("UNDEC"), pid(s.Position)), quote(s.Text)) + "\n")
			names := make([]string, 0, len(s.Verdicts))
			for name := range s.Verdicts {
				names = append(names, name)
			}
			sort.Strings(names)
			parts := make([]string, 0, len(names))
			for _, name := range names {
				parts = append(parts, name+"→"+s.Verdicts[name])
			}
			fmt.Fprintf(&b, "      %s\n", cDim("audience-sensitive: "+strings.Join(parts, "  ")))
		}
		if len(ai.Robust) == 0 && len(ai.Sensitive) == 0 {
			b.WriteString("  " + cDim("no position is robust; none flips with the audience either (contested on the arguments themselves)") + "\n")
		}
	}
	return b.String()
}
