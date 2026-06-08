// Package store is the only pudl-aware package: it binds dlktk's domain types to
// pudl's bitemporal fact store under the reserved dlktk/* relation namespace.
package store

import (
	"encoding/json"
	"fmt"

	"github.com/chazu/pudl/pkg/factstore"

	"github.com/chazu/dlktk/internal/ibis"
)

// Reserved dlktk relation namespace (see design §3.7).
const (
	relDiscussion = "dlktk/discussion"
	relNode       = "dlktk/node"
	relLink       = "dlktk/link"
	relIssueCard  = "dlktk/issue_card"
	relPreference = "dlktk/preference"
	relDecision   = "dlktk/decision"
)

// Store wraps a pudl fact store.
type Store struct {
	fs *factstore.Store
}

// Open opens (or creates) the pudl store at dir.
func Open(dir string) (*Store, error) {
	fs, err := factstore.Open(dir)
	if err != nil {
		return nil, fmt.Errorf("open pudl store: %w", err)
	}
	return &Store{fs: fs}, nil
}

// Close releases the store.
func (s *Store) Close() error { return s.fs.Close() }

// add marshals payload as a fact's args and appends it. source carries author.
func (s *Store) add(relation string, payload any, source string) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal %s args: %w", relation, err)
	}
	_, err = s.fs.AddFact(factstore.Fact{Relation: relation, Args: string(b), Source: source})
	if err != nil {
		return fmt.Errorf("add %s fact: %w", relation, err)
	}
	return nil
}

// scan returns the current (or as-of) facts for a relation, unmarshalling each
// args object into a fresh T. asOf is a Unix transaction time, or nil for now.
func scan[T any](s *Store, relation string, asOf *int64) ([]T, error) {
	facts, err := s.fs.QueryFacts(factstore.FactFilter{Relation: relation, TxAt: asOf})
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", relation, err)
	}
	out := make([]T, 0, len(facts))
	for _, f := range facts {
		var v T
		if err := json.Unmarshal([]byte(f.Args), &v); err != nil {
			return nil, fmt.Errorf("unmarshal %s fact %s: %w", relation, f.ID, err)
		}
		out = append(out, v)
	}
	return out, nil
}

// --- writes ---

func (s *Store) AddDiscussion(d ibis.Discussion) error {
	return s.add(relDiscussion, d, d.CreatedBy)
}
func (s *Store) AddNode(n ibis.Node) error { return s.add(relNode, n, n.Author) }
func (s *Store) AddLink(l ibis.Link) error { return s.add(relLink, l, l.Author) }
func (s *Store) AddPreference(p ibis.Preference) error {
	return s.add(relPreference, p, p.Author)
}
func (s *Store) AddDecision(d ibis.Decision) error { return s.add(relDecision, d, d.Decider) }
func (s *Store) SetIssueCard(c ibis.IssueCard) error {
	return s.add(relIssueCard, c, "")
}

// --- reads (filtered by discussion in Go; graphs are tiny) ---

func (s *Store) Discussions(asOf *int64) ([]ibis.Discussion, error) {
	return scan[ibis.Discussion](s, relDiscussion, asOf)
}

func (s *Store) Nodes(disc string, asOf *int64) ([]ibis.Node, error) {
	all, err := scan[ibis.Node](s, relNode, asOf)
	return filter(all, disc, func(n ibis.Node) string { return n.Disc }), err
}

func (s *Store) Links(disc string, asOf *int64) ([]ibis.Link, error) {
	all, err := scan[ibis.Link](s, relLink, asOf)
	return filter(all, disc, func(l ibis.Link) string { return l.Disc }), err
}

func (s *Store) Preferences(disc string, asOf *int64) ([]ibis.Preference, error) {
	all, err := scan[ibis.Preference](s, relPreference, asOf)
	return filter(all, disc, func(p ibis.Preference) string { return p.Disc }), err
}

func (s *Store) Decisions(disc string, asOf *int64) ([]ibis.Decision, error) {
	all, err := scan[ibis.Decision](s, relDecision, asOf)
	return filter(all, disc, func(d ibis.Decision) string { return d.Disc }), err
}

// IssueCards returns cards for issues in this discussion. Cards carry no disc of
// their own; they are matched against the discussion's issue nodes by the caller
// via the graph. Here we return all current cards; Graph filters by issue id.
func (s *Store) IssueCards(asOf *int64) ([]ibis.IssueCard, error) {
	return scan[ibis.IssueCard](s, relIssueCard, asOf)
}

// Graph loads and indexes a full discussion snapshot at asOf.
func (s *Store) Graph(disc string, asOf *int64) (*ibis.Graph, error) {
	nodes, err := s.Nodes(disc, asOf)
	if err != nil {
		return nil, err
	}
	links, err := s.Links(disc, asOf)
	if err != nil {
		return nil, err
	}
	prefs, err := s.Preferences(disc, asOf)
	if err != nil {
		return nil, err
	}
	cards, err := s.IssueCards(asOf)
	if err != nil {
		return nil, err
	}
	return ibis.NewGraph(nodes, links, prefs, cards), nil
}

// RetractNode closes the tt interval of the current dlktk/node fact whose
// args.id == nid (the concede/retract move). The current fact is the one pudl
// returns under AsOfNow; there must be at most one (design §3.1).
func (s *Store) RetractNode(nid string) error {
	facts, err := s.fs.QueryFacts(factstore.FactFilter{Relation: relNode})
	if err != nil {
		return fmt.Errorf("query nodes: %w", err)
	}
	var target string
	matches := 0
	for _, f := range facts {
		var n ibis.Node
		if err := json.Unmarshal([]byte(f.Args), &n); err != nil {
			continue
		}
		if n.ID == nid {
			target = f.ID
			matches++
		}
	}
	if matches == 0 {
		return fmt.Errorf("node %s not found", nid)
	}
	if matches > 1 {
		return fmt.Errorf("node %s is current in %d facts (store invariant violated)", nid, matches)
	}
	if err := s.fs.RetractFact(target); err != nil {
		return fmt.Errorf("retract node %s: %w", nid, err)
	}
	return nil
}

func filter[T any](all []T, disc string, discOf func(T) string) []T {
	out := make([]T, 0, len(all))
	for _, v := range all {
		if discOf(v) == disc {
			out = append(out, v)
		}
	}
	return out
}
