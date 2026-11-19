// Package search finds existing claims in the discussions selected by the caller.
package search

import (
	"context"
	"math"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/render"
	"github.com/chazu/dlktk/internal/store"
)

// Read preserves substring search by default. Semantic search sends a local
// shortlist to Jev only after the caller explicitly selects that mode.
func Read(ctx context.Context, s *store.Store, w store.When, discs []string, query string, semantic bool) (render.SearchView, error) {
	v := render.SearchView{Query: query}
	filter := query
	if semantic {
		filter = ""
	}
	for _, disc := range discs {
		g, err := s.Graph(disc, w)
		if err != nil {
			return v, err
		}
		var labels map[string]af.Label
		if fw, err := af.Build(g); err == nil {
			labels = fw.Grounded()
		}
		v.Hits = append(v.Hits, render.Search(disc, g, labels, filter)...)
	}
	if semantic {
		return rerank(ctx, query, shortlist(query, v.Hits))
	}
	return v, nil
}

const stopWords = " a an and are as at be by can for from if in is it of on or should that the to when with "

func terms(text string) map[string]bool {
	out := make(map[string]bool)
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if !strings.Contains(stopWords, " "+word+" ") {
			out[word] = true
		}
	}
	return out
}

func shortlist(query string, hits []render.SearchHit) []render.SearchHit {
	type candidate struct {
		hit     render.SearchHit
		literal bool
		score   float64
	}
	query = strings.ToLower(query)
	var tokens []string
	for t := range terms(query) {
		tokens = append(tokens, t)
	}
	sort.Strings(tokens)
	df := map[string]int{}
	seen := make([]map[string]bool, len(hits))
	for i, h := range hits {
		seen[i] = terms(h.Text)
		for _, t := range tokens {
			if seen[i][t] {
				df[t]++
			}
		}
	}
	ranked := make([]candidate, len(hits))
	for i, h := range hits {
		c := candidate{hit: h, literal: strings.Contains(strings.ToLower(h.Text), query)}
		for _, t := range tokens {
			if seen[i][t] {
				c.score += math.Log(float64(len(hits)+1) / float64(df[t]))
			}
		}
		ranked[i] = c
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.literal != b.literal {
			return a.literal
		}
		if a.score != b.score {
			return a.score > b.score
		}
		if a.hit.ID != b.hit.ID {
			return a.hit.ID < b.hit.ID
		}
		return a.hit.Discussion < b.hit.Discussion
	})
	var out []render.SearchHit
	for _, c := range ranked[:min(len(ranked), 8)] {
		out = append(out, c.hit)
	}
	return out
}

// labelFor disambiguates IDs shared by imported discussions.
func labelFor(h render.SearchHit, counts map[string]int) string {
	if counts[h.ID] > 1 || h.ID == "none" || strings.Contains(h.ID, "/") {
		return url.PathEscape(h.Discussion) + "/" + url.PathEscape(h.ID)
	}
	return h.ID
}
