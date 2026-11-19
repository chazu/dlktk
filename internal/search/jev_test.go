package search

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/chazu/dlktk/internal/render"
)

func TestBoundedShortlistRetainsLiteral(t *testing.T) {
	var hits []render.SearchHit
	for i := 0; i < 12; i++ {
		hits = append(hits, render.SearchHit{ID: fmt.Sprint(i), Text: "many readers may delay the writer"})
	}
	hits[11].Text = "some writers wait"
	short := shortlist("writers wait", hits)
	if len(short) != 8 || short[0].ID != "11" {
		t.Fatalf("unexpected shortlist: %v", short)
	}
}

func TestProviderSelection(t *testing.T) {
	hits := []render.SearchHit{{ID: "x", Discussion: "one"}, {ID: "x", Discussion: "two"}}
	counts := map[string]int{"x": 2}
	opts := orderedCriteria{{labelFor(hits[0], counts), "first"}, {labelFor(hits[1], counts), "second"}, {"none", "no match"}}
	var a choiceAnswer
	for _, tc := range []struct {
		body    string
		want    int
		invalid bool
	}{
		{`{"choice":"two/x","probabilities":{"one/x":0.1,"two/x":0.7,"none":0.2}}`, 1, false},
		{`{"choice":"none","probabilities":{"one/x":0.1,"two/x":0.2,"none":0.7}}`, 0, false},
		{`{"choice":"other","probabilities":{"one/x":0.1,"two/x":0.7,"none":0.2}}`, 0, true},
		{`{"choice":"two/x","probabilities":{"one/x":null,"two/x":0.7,"none":0.3}}`, 0, true},
	} {
		if err := json.Unmarshal([]byte(tc.body), &a); err != nil {
			t.Fatal(err)
		}
		got, err := selectHits(hits, opts, a)
		if (err != nil) != tc.invalid || len(got) != tc.want || (len(got) > 0 && got[0].Discussion != "two") {
			t.Fatalf("response %s: %v, %v", tc.body, got, err)
		}
	}
}
