// Package store is the only pudl-aware package: it binds dlktk's domain types to
// pudl's bitemporal fact store under the reserved dlktk/* relation namespace.
package store

import (
	"encoding/json"
	"fmt"
	"sort"

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

// When selects the temporal viewpoint for a read: Tx is transaction-time
// (--as-of, "what did we know when"), Valid is valid-time (--valid-at, "what was
// in effect when"). The zero value means now.
type When struct {
	Tx    *int64
	Valid *int64
}

// Now is the present-time viewpoint.
func Now() When { return When{} }

// scan returns the facts for a relation at the given temporal viewpoint,
// unmarshalling each args object into a fresh T.
func scan[T any](s *Store, relation string, w When) ([]T, error) {
	facts, err := s.fs.QueryFacts(factstore.FactFilter{Relation: relation, TxAt: w.Tx, ValidAt: w.Valid})
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

func (s *Store) Discussions(w When) ([]ibis.Discussion, error) {
	return scan[ibis.Discussion](s, relDiscussion, w)
}

func (s *Store) Nodes(disc string, w When) ([]ibis.Node, error) {
	all, err := scan[ibis.Node](s, relNode, w)
	return filter(all, disc, func(n ibis.Node) string { return n.Disc }), err
}

func (s *Store) Links(disc string, w When) ([]ibis.Link, error) {
	all, err := scan[ibis.Link](s, relLink, w)
	return filter(all, disc, func(l ibis.Link) string { return l.Disc }), err
}

func (s *Store) Preferences(disc string, w When) ([]ibis.Preference, error) {
	all, err := scan[ibis.Preference](s, relPreference, w)
	return filter(all, disc, func(p ibis.Preference) string { return p.Disc }), err
}

func (s *Store) Decisions(disc string, w When) ([]ibis.Decision, error) {
	all, err := scan[ibis.Decision](s, relDecision, w)
	return filter(all, disc, func(d ibis.Decision) string { return d.Disc }), err
}

// IssueCards returns cards at the given viewpoint. Cards carry no disc of their
// own; Graph matches them to this discussion's issue ids by key.
func (s *Store) IssueCards(w When) ([]ibis.IssueCard, error) {
	return scan[ibis.IssueCard](s, relIssueCard, w)
}

// Graph loads and indexes a full discussion snapshot at the given viewpoint.
// The graph uses the transaction-time axis (structure as it stood); valid-time
// is for decisions and is read separately.
func (s *Store) Graph(disc string, w When) (*ibis.Graph, error) {
	gw := When{Tx: w.Tx}
	nodes, err := s.Nodes(disc, gw)
	if err != nil {
		return nil, err
	}
	links, err := s.Links(disc, gw)
	if err != nil {
		return nil, err
	}
	prefs, err := s.Preferences(disc, gw)
	if err != nil {
		return nil, err
	}
	cards, err := s.IssueCards(gw)
	if err != nil {
		return nil, err
	}
	return ibis.NewGraph(nodes, links, prefs, cards), nil
}

// SupersedeDecision invalidates (closes the vt interval of) any current decision
// on the given issue, so a fresh decide supersedes it. No-op if none stands.
func (s *Store) SupersedeDecision(disc, issue string) error {
	facts, err := s.fs.QueryFacts(factstore.FactFilter{Relation: relDecision})
	if err != nil {
		return fmt.Errorf("query decisions: %w", err)
	}
	for _, f := range facts {
		var d ibis.Decision
		if err := json.Unmarshal([]byte(f.Args), &d); err != nil {
			continue
		}
		if d.Disc == disc && d.Issue == issue {
			if err := s.fs.InvalidateFact(f.ID); err != nil {
				return fmt.Errorf("invalidate prior decision on %s: %w", issue, err)
			}
		}
	}
	return nil
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

// HistoryEntry is one fact's transaction-time record for the audit trail.
type HistoryEntry struct {
	Relation  string `json:"relation"`
	ID        string `json:"id,omitempty"`
	Summary   string `json:"summary"`
	Author    string `json:"author,omitempty"`
	TxStart   int64  `json:"tx_start"`
	TxEnd     *int64 `json:"tx_end,omitempty"`
	Retracted bool   `json:"retracted"`
}

// History returns the transaction-time audit trail for a discussion: every
// node/link/preference/decision fact ever recorded (including retracted), in tt
// order. If nodeID is non-empty, only facts carrying that args.id are returned.
func (s *Store) History(disc, nodeID string) ([]HistoryEntry, error) {
	var out []HistoryEntry
	for _, rel := range []string{relNode, relLink, relPreference, relDecision} {
		facts, err := s.fs.FactHistory(rel)
		if err != nil {
			return nil, fmt.Errorf("history %s: %w", rel, err)
		}
		for _, f := range facts {
			var m map[string]any
			if err := json.Unmarshal([]byte(f.Args), &m); err != nil {
				continue
			}
			if d, _ := m["disc"].(string); d != disc {
				continue
			}
			fid, _ := m["id"].(string)
			if nodeID != "" && fid != nodeID {
				continue
			}
			out = append(out, HistoryEntry{
				Relation:  rel,
				ID:        fid,
				Summary:   summarize(rel, m),
				Author:    f.Source,
				TxStart:   f.TxStart,
				TxEnd:     f.TxEnd,
				Retracted: f.TxEnd != nil,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TxStart < out[j].TxStart })
	return out, nil
}

// summarize produces a human label for a fact's args by relation.
func summarize(rel string, m map[string]any) string {
	str := func(k string) string { s, _ := m[k].(string); return s }
	switch rel {
	case relNode:
		return fmt.Sprintf("%s %q", str("kind"), str("text"))
	case relLink:
		return fmt.Sprintf("%s %s→%s", str("rel"), str("src"), str("dst"))
	case relPreference:
		return fmt.Sprintf("prefer %s>%s basis=%s", str("winner"), str("loser"), str("basis"))
	case relDecision:
		return fmt.Sprintf("decide %s→%s", str("issue"), str("position"))
	}
	return rel
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
