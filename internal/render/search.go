package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// SearchHit is one node matching a search query.
type SearchHit struct {
	Discussion string `json:"discussion"`
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Text       string `json:"text"`
	Label      string `json:"label,omitempty"` // AF nodes only; empty if unlabellable
}

// SearchView is the search envelope. Search exists so an agent rejoining a
// discussion can check whether a claim has already been made before piling on
// a duplicate — the predictable multi-agent failure mode.
type SearchView struct {
	Query string      `json:"query"`
	Hits  []SearchHit `json:"hits"`
}

// Search matches query (case-insensitive substring) against node text in one
// discussion's graph. labels may be nil (e.g. the graph is unlabellable).
func Search(disc string, g *ibis.Graph, labels map[string]af.Label, query string) []SearchHit {
	q := strings.ToLower(query)
	var hits []SearchHit
	for id, n := range g.Nodes {
		if !strings.Contains(strings.ToLower(n.Text), q) {
			continue
		}
		h := SearchHit{Discussion: disc, ID: id, Kind: string(n.Kind), Text: n.Text}
		if labels != nil && g.IsAFNode(id) {
			h.Label = string(labels[id])
		}
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].ID < hits[j].ID })
	return hits
}

// SearchText renders a SearchView as human text.
func SearchText(v SearchView) string {
	if len(v.Hits) == 0 {
		return cDim(fmt.Sprintf("no nodes match %q", v.Query)) + "\n"
	}
	var b strings.Builder
	for _, h := range v.Hits {
		label := ""
		if h.Label != "" {
			label = labelInline(h.Label) + " "
		}
		prefix := fmt.Sprintf("%s  %s%s  ", cDim(h.Discussion), label, nid(h.Kind, h.ID))
		b.WriteString(para(prefix, quote(h.Text)) + "\n")
	}
	return b.String()
}
