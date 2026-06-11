// Package store is the only pudl-aware package: it binds dlktk's domain types to
// pudl's bitemporal fact store under the reserved dlktk/* relation namespace.
package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/chazu/pudl/pkg/factstore"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/fail"
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
		return nil, fail.Store("open pudl store: %v", err)
	}
	return &Store{fs: fs}, nil
}

// Close releases the store.
func (s *Store) Close() error { return s.fs.Close() }

// add marshals payload as a fact's args and appends it. source carries author.
func (s *Store) add(relation string, payload any, source string) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fail.Store("marshal %s args: %v", relation, err)
	}
	_, err = s.fs.AddFact(factstore.Fact{Relation: relation, Args: string(b), Source: source})
	if err != nil {
		return fail.Store("add %s fact: %v", relation, err)
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
		return nil, fail.Store("query %s: %v", relation, err)
	}
	out := make([]T, 0, len(facts))
	for _, f := range facts {
		var v T
		if err := json.Unmarshal([]byte(f.Args), &v); err != nil {
			return nil, fail.Store("unmarshal %s fact %s: %v", relation, f.ID, err)
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
		return fail.Store("query decisions: %v", err)
	}
	for _, f := range facts {
		var d ibis.Decision
		if err := json.Unmarshal([]byte(f.Args), &d); err != nil {
			continue
		}
		if d.Disc == disc && d.Issue == issue {
			if err := s.fs.InvalidateFact(f.ID); err != nil {
				return fail.Store("invalidate prior decision on %s: %v", issue, err)
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
		return fail.Store("query nodes: %v", err)
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
		return fail.NotFound(nid, "node %s not found", nid)
	}
	if matches > 1 {
		return fail.Store("node %s is current in %d facts (store invariant violated)", nid, matches)
	}
	if err := s.fs.RetractFact(target); err != nil {
		return fail.Store("retract node %s: %v", nid, err)
	}
	return nil
}

// ExportRecord is one record in the git-native NDJSON move log. An assert
// record (Event == "") carries a fact; ValidStart and Source are preserved so
// re-import recomputes the same content-addressed id (making import
// idempotent). A history export additionally interleaves event records
// (Event == retract|invalidate) that close a previously asserted fact,
// referenced by its content-addressed id.
type ExportRecord struct {
	Relation   string          `json:"relation"`
	Args       json.RawMessage `json:"args,omitempty"`
	Source     string          `json:"source,omitempty"`
	ValidStart int64           `json:"valid_start,omitempty"`
	Event      string          `json:"event,omitempty"` // "" (assert) | "retract" | "invalidate"
	Ref        string          `json:"ref,omitempty"`   // content-addressed fact id the event applies to
}

// Export gathers the current (live) dlktk/* facts for a discussion as an
// ordered record stream suitable for NDJSON dump and idempotent re-import.
func (s *Store) Export(disc string) ([]ExportRecord, error) {
	var recs []ExportRecord
	issueIDs := map[string]bool{}

	emit := func(rel string, keep func(map[string]any) bool) error {
		facts, err := s.fs.QueryFacts(factstore.FactFilter{Relation: rel})
		if err != nil {
			return fail.Store("export %s: %v", rel, err)
		}
		for _, f := range facts {
			var m map[string]any
			if err := json.Unmarshal([]byte(f.Args), &m); err != nil {
				continue
			}
			if !keep(m) {
				continue
			}
			if rel == relNode {
				if k, _ := m["kind"].(string); k == string(ibis.Issue) {
					if id, _ := m["id"].(string); id != "" {
						issueIDs[id] = true
					}
				}
			}
			recs = append(recs, ExportRecord{
				Relation:   rel,
				Args:       json.RawMessage(f.Args),
				Source:     f.Source,
				ValidStart: f.ValidStart,
			})
		}
		return nil
	}

	discIs := func(m map[string]any) bool { d, _ := m["disc"].(string); return d == disc }

	if err := emit(relDiscussion, func(m map[string]any) bool { id, _ := m["id"].(string); return id == disc }); err != nil {
		return nil, err
	}
	if err := emit(relNode, discIs); err != nil { // populates issueIDs
		return nil, err
	}
	if err := emit(relLink, discIs); err != nil {
		return nil, err
	}
	if err := emit(relPreference, discIs); err != nil {
		return nil, err
	}
	if err := emit(relDecision, discIs); err != nil {
		return nil, err
	}
	if err := emit(relIssueCard, func(m map[string]any) bool { i, _ := m["issue"].(string); return issueIDs[i] }); err != nil {
		return nil, err
	}
	return recs, nil
}

// Event kinds in a history export.
const (
	EventRetract    = "retract"
	EventInvalidate = "invalidate"
)

// ExportHistory gathers a discussion's full transaction-time history as an
// ordered, replayable event stream: every fact ever asserted (including
// later-retracted ones) followed, at its place in time, by the retract /
// invalidate events that closed it. Importing the stream into a fresh store
// reproduces both the current state and the audit trail. Event *order* is
// preserved exactly; pudl stamps transaction times at import, so original
// wall-clock tt is not reproduced (its API takes no explicit tx time).
func (s *Store) ExportHistory(disc string) ([]ExportRecord, error) {
	type histRec struct {
		rec  ExportRecord
		at   int64
		rank int // assert < invalidate < retract at equal times
		tie  string
	}
	var all []histRec
	issueIDs := map[string]bool{}

	collect := func(rel string, keep func(map[string]any) bool) error {
		facts, err := s.fs.FactHistory(rel)
		if err != nil {
			return fail.Store("history export %s: %v", rel, err)
		}
		for _, f := range facts {
			var m map[string]any
			if err := json.Unmarshal([]byte(f.Args), &m); err != nil {
				continue
			}
			if !keep(m) {
				continue
			}
			if rel == relNode {
				if k, _ := m["kind"].(string); k == string(ibis.Issue) {
					if id, _ := m["id"].(string); id != "" {
						issueIDs[id] = true
					}
				}
			}
			all = append(all, histRec{
				rec: ExportRecord{Relation: rel, Args: json.RawMessage(f.Args), Source: f.Source, ValidStart: f.ValidStart},
				at:  f.TxStart, rank: 0, tie: f.ID,
			})
			if f.ValidEnd != nil {
				all = append(all, histRec{
					rec: ExportRecord{Relation: rel, Event: EventInvalidate, Ref: f.ID},
					at:  *f.ValidEnd, rank: 1, tie: f.ID,
				})
			}
			if f.TxEnd != nil {
				all = append(all, histRec{
					rec: ExportRecord{Relation: rel, Event: EventRetract, Ref: f.ID},
					at:  *f.TxEnd, rank: 2, tie: f.ID,
				})
			}
		}
		return nil
	}

	discIs := func(m map[string]any) bool { d, _ := m["disc"].(string); return d == disc }
	if err := collect(relDiscussion, func(m map[string]any) bool { id, _ := m["id"].(string); return id == disc }); err != nil {
		return nil, err
	}
	if err := collect(relNode, discIs); err != nil { // populates issueIDs
		return nil, err
	}
	for _, rel := range []string{relLink, relPreference, relDecision} {
		if err := collect(rel, discIs); err != nil {
			return nil, err
		}
	}
	if err := collect(relIssueCard, func(m map[string]any) bool { i, _ := m["issue"].(string); return issueIDs[i] }); err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.at != b.at {
			return a.at < b.at
		}
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		return a.tie < b.tie
	})
	recs := make([]ExportRecord, len(all))
	for i, h := range all {
		recs[i] = h.rec
	}
	return recs, nil
}

// applyEvent replays one retract/invalidate event. Already-applied events are
// skipped, so re-importing a history stream converges.
func (s *Store) applyEvent(rec ExportRecord) error {
	facts, err := s.fs.FactHistory(rec.Relation)
	if err != nil {
		return fail.Store("replay %s: %v", rec.Event, err)
	}
	for _, f := range facts {
		if f.ID != rec.Ref {
			continue
		}
		switch rec.Event {
		case EventRetract:
			if f.TxEnd != nil {
				return nil // already retracted
			}
			if err := s.fs.RetractFact(f.ID); err != nil {
				return fail.Store("replay retract %s: %v", f.ID, err)
			}
		case EventInvalidate:
			if f.ValidEnd != nil {
				return nil // already invalidated
			}
			if err := s.fs.InvalidateFact(f.ID); err != nil {
				return fail.Store("replay invalidate %s: %v", f.ID, err)
			}
		}
		return nil
	}
	return &ibis.IllegalMove{Detail: fmt.Sprintf(
		"history event %s references unknown %s fact %s (stream not self-contained)", rec.Event, rec.Relation, rec.Ref)}
}

// Import re-asserts one exported fact. Idempotent: pudl content-addresses by
// (relation, canonical args, valid_start, source), so re-importing dedups.
func (s *Store) Import(rec ExportRecord) error {
	if !strings.HasPrefix(rec.Relation, "dlktk/") {
		return &ibis.IllegalMove{Detail: fmt.Sprintf("refusing to import non-dlktk relation %q", rec.Relation)}
	}
	_, err := s.fs.AddFact(factstore.Fact{
		Relation:   rec.Relation,
		Args:       string(rec.Args),
		Source:     rec.Source,
		ValidStart: rec.ValidStart,
	})
	if err != nil {
		return fail.Store("import %s: %v", rec.Relation, err)
	}
	return nil
}

// ImportAll validates a batch of exported facts and then writes them. Import is
// a first-class write path (the NDJSON log can be the system of record, design
// §12), so it must not bypass the invariants the move layer enforces: every
// record must be a known dlktk relation with well-formed args, and the batch's
// preferences — combined with what the store already holds — must stay acyclic
// (a cycle would collapse the closure to all-prefer-all, §16 Q2). Validation
// errors are IllegalMove (exit 2) and nothing is written.
func (s *Store) ImportAll(recs []ExportRecord) (int, error) {
	prefsByDisc := map[string][]ibis.Preference{}
	for i, rec := range recs {
		if err := validateRecord(rec, i+1, prefsByDisc); err != nil {
			return 0, err
		}
	}
	for disc, incoming := range prefsByDisc {
		existing, err := s.Preferences(disc, Now())
		if err != nil {
			return 0, err
		}
		if node, cyclic := af.PreferenceCycle(append(existing, incoming...)); cyclic {
			return 0, &ibis.IllegalMove{Node: node, Detail: fmt.Sprintf(
				"import would create a preference cycle through %s in discussion %s; nothing imported", node, disc)}
		}
	}
	for _, rec := range recs {
		if rec.Event != "" {
			if err := s.applyEvent(rec); err != nil {
				return 0, err
			}
			continue
		}
		if err := s.Import(rec); err != nil {
			return 0, err
		}
	}
	return len(recs), nil
}

// validateRecord checks one record's relation and args shape, collecting
// preferences for the batch-level acyclicity check.
func validateRecord(rec ExportRecord, line int, prefsByDisc map[string][]ibis.Preference) error {
	bad := func(format string, a ...any) error {
		return &ibis.IllegalMove{Detail: fmt.Sprintf("import record %d: ", line) + fmt.Sprintf(format, a...)}
	}
	parse := func(v any) error {
		if err := json.Unmarshal(rec.Args, v); err != nil {
			return bad("malformed %s args: %v", rec.Relation, err)
		}
		return nil
	}
	require := func(field, val string) error {
		if val == "" {
			return bad("%s args missing %q", rec.Relation, field)
		}
		return nil
	}

	if rec.Event != "" {
		if rec.Event != EventRetract && rec.Event != EventInvalidate {
			return bad("event %q invalid (retract or invalidate)", rec.Event)
		}
		if rec.Ref == "" {
			return bad("%s event missing ref", rec.Event)
		}
		if !knownRelation(rec.Relation) {
			return bad("unknown relation %q", rec.Relation)
		}
		return nil
	}

	switch rec.Relation {
	case relDiscussion:
		var v ibis.Discussion
		if err := parse(&v); err != nil {
			return err
		}
		return require("id", v.ID)
	case relNode:
		var v ibis.Node
		if err := parse(&v); err != nil {
			return err
		}
		if v.Kind != ibis.Issue && v.Kind != ibis.Position && v.Kind != ibis.Argument {
			return bad("node kind %q invalid", v.Kind)
		}
		return firstErr(require("id", v.ID), require("disc", v.Disc))
	case relLink:
		var v ibis.Link
		if err := parse(&v); err != nil {
			return err
		}
		if v.Rel != ibis.RespondsTo && v.Rel != ibis.Supports && v.Rel != ibis.ObjectsTo {
			return bad("link rel %q invalid", v.Rel)
		}
		return firstErr(require("id", v.ID), require("disc", v.Disc), require("src", v.Src), require("dst", v.Dst))
	case relIssueCard:
		var v ibis.IssueCard
		if err := parse(&v); err != nil {
			return err
		}
		if v.Cardinality != ibis.SelectOne && v.Cardinality != ibis.Open {
			return bad("cardinality %q invalid", v.Cardinality)
		}
		return require("issue", v.Issue)
	case relPreference:
		var v ibis.Preference
		if err := parse(&v); err != nil {
			return err
		}
		if err := firstErr(require("disc", v.Disc), require("winner", v.Winner), require("loser", v.Loser)); err != nil {
			return err
		}
		prefsByDisc[v.Disc] = append(prefsByDisc[v.Disc], v)
		return nil
	case relDecision:
		var v ibis.Decision
		if err := parse(&v); err != nil {
			return err
		}
		return firstErr(require("disc", v.Disc), require("issue", v.Issue), require("position", v.Position))
	default:
		return bad("unknown relation %q", rec.Relation)
	}
}

func knownRelation(rel string) bool {
	switch rel {
	case relDiscussion, relNode, relLink, relIssueCard, relPreference, relDecision:
		return true
	}
	return false
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
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
			return nil, fail.Store("history %s: %v", rel, err)
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
