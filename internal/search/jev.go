package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/chazu/dlktk/internal/render"
)

const instructions = "Select the existing node that most nearly states the same claim as the query. Respect who does what, polarity, cause and scope. Related topics and contrary claims are not duplicates. Choose none if no candidate makes the claim."

type option struct{ key, text string }

// orderedCriteria preserves the option order evaluated in the pilot.
type orderedCriteria []option

func (opts orderedCriteria) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, opt := range opts {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(opt.key)
		text, _ := json.Marshal(opt.text)
		b.Write(key)
		b.WriteByte(':')
		b.Write(text)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

type choiceAnswer struct {
	Choice string              `json:"choice"`
	Scores map[string]*float64 `json:"probabilities"`
}

func rerank(ctx context.Context, query string, hits []render.SearchHit) (render.SearchView, error) {
	v := render.SearchView{Query: query, Semantic: true}
	if strings.TrimSpace(query) == "" {
		return v, fmt.Errorf("semantic search requires a claim to look up")
	}
	if len(hits) == 0 {
		return v, nil
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return v, fmt.Errorf("semantic search needs OPENROUTER_API_KEY; ordinary search runs offline")
	}
	counts := map[string]int{}
	for _, h := range hits {
		counts[h.ID]++
	}
	opts := orderedCriteria{}
	for _, h := range hits {
		opts = append(opts, option{labelFor(h, counts), h.Text})
	}
	opts = append(opts, option{"none", "No candidate makes the same claim."})
	question := map[string]any{"type": "choice", "instructions": instructions, "criteria": opts}
	payload := map[string]any{
		"state": query, "model": "typesafe/jev-1.14",
		"questions": map[string]any{"review": question},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return v, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/alpha/decisions", bytes.NewReader(body))
	if err != nil {
		return v, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client := http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return v, fmt.Errorf("semantic search request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return v, fmt.Errorf("semantic search provider returned %s; retry or use ordinary search", res.Status)
	}
	var result struct {
		Answers map[string]choiceAnswer `json:"answers"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return v, fmt.Errorf("semantic search response: %w", err)
	}
	answer := result.Answers["review"]
	v.Hits, err = selectHits(hits, opts, answer)
	return v, err
}

func selectHits(hits []render.SearchHit, opts orderedCriteria, a choiceAnswer) ([]render.SearchHit, error) {
	if _, ok := a.Scores[a.Choice]; !ok || len(a.Scores) != len(opts) {
		return nil, fmt.Errorf("semantic search provider returned an invalid choice")
	}
	for _, opt := range opts {
		p := a.Scores[opt.key]
		if p == nil || *p < 0 || *p > 1 {
			return nil, fmt.Errorf("semantic search provider returned an invalid probability")
		}
	}
	if a.Choice == "none" {
		return nil, nil
	}
	var selected []int
	for i := range hits {
		if *a.Scores[opts[i].key] > *a.Scores["none"] {
			selected = append(selected, i)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool { return *a.Scores[opts[selected[i]].key] > *a.Scores[opts[selected[j]].key] })
	var out []render.SearchHit
	for _, i := range selected[:min(len(selected), 3)] {
		out = append(out, hits[i])
	}
	return out, nil
}
