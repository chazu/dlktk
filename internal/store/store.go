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
	relRoster     = "dlktk/roster"
	relReframe    = "dlktk/reframe"
	relValue      = "dlktk/value"
	relAudience   = "dlktk/audience"
)

// factOps is the slice of pudl's API that store operations run against. Both
// *factstore.Store (standalone operations) and *factstore.Tx (operations
// inside a Move transaction) satisfy it.
type factOps interface {
	AddFact(f factstore.Fact) (factstore.Fact, error)
	QueryFacts(filter factstore.FactFilter) ([]factstore.Fact, error)
	RetractFact(id string) error
	InvalidateFact(id string) error
	FactHistory(relation string) ([]factstore.Fact, error)
}

// Store wraps a pudl fact store — or, inside a Move callback, a single pudl
// transaction presenting the same read/write surface.
type Store struct {
	fs  *factstore.Store // nil on the transactional view passed to a Move callback
	ops factOps
}

// Open opens (or creates) the pudl store at dir.
func Open(dir string) (*Store, error) {
	fs, err := factstore.Open(dir)
	if err != nil {
		return nil, fail.Store("open pudl store: %v", err)
	}
	return &Store{fs: fs, ops: fs}, nil
}

// Close releases the store.
func (s *Store) Close() error { return s.fs.Close() }

// Move runs fn against a transactional view of the store: every read and
// write fn performs happens inside one pudl transaction that holds the store
// write lock from the start, so a move's legality check cannot interleave
// with another agent's write — in this process or another (closes the
// check-then-write TOCTOU race, ANALYSIS §1.4). An error from fn rolls back
// every write fn made.
func (s *Store) Move(fn func(txs *Store) error) error {
	if s.fs == nil {
		return fail.Store("nested move transaction")
	}
	var fnErr error
	err := s.fs.Transact(func(tx *factstore.Tx) error {
		fnErr = fn(&Store{ops: tx})
		return fnErr
	})
	if err != nil && err != fnErr {
		return fail.Store("move transaction: %v", err)
	}
	return err
}

// add marshals payload as a fact's args and appends it. source carries author.
func (s *Store) add(relation string, payload any, source string) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fail.Store("marshal %s args: %v", relation, err)
	}
	_, err = s.ops.AddFact(factstore.Fact{Relation: relation, Args: string(b), Source: source})
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
	facts, err := s.ops.QueryFacts(factstore.FactFilter{Relation: relation, TxAt: w.Tx, ValidAt: w.Valid})
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

// AddRoster records an author↔role binding for a discussion. Content-addressing
// makes it idempotent: re-asserting the same binding dedups, so the auto-record
// on every move costs one row per distinct (disc, author, role) (design §16 Q8).
func (s *Store) AddRoster(r ibis.Roster) error { return s.add(relRoster, r, r.Author) }

// AddReframe records that an issue's framing was replaced.
func (s *Store) AddReframe(r ibis.Reframe) error { return s.add(relReframe, r, r.Author) }

// AddValue records the value a node promotes.
func (s *Store) AddValue(v ibis.ValueTag) error { return s.add(relValue, v, v.Author) }

// AddAudience records a named value ranking.
func (s *Store) AddAudience(a ibis.Audience) error { return s.add(relAudience, a, a.Author) }

// SupersedeAudience invalidates (closes the vt interval of) the current
// audience fact(s) with the given name, so a fresh declaration supersedes it —
// the SupersedeDecision pattern. No-op if none stands.
func (s *Store) SupersedeAudience(disc, name string) error {
	facts, err := s.ops.QueryFacts(factstore.FactFilter{Relation: relAudience})
	if err != nil {
		return fail.Store("query audiences: %v", err)
	}
	for _, f := range facts {
		var a ibis.Audience
		if err := json.Unmarshal([]byte(f.Args), &a); err != nil {
			continue
		}
		if a.Disc == disc && a.Name == name {
			if err := s.ops.InvalidateFact(f.ID); err != nil {
				return fail.Store("invalidate prior audience %s: %v", name, err)
			}
		}
	}
	return nil
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

// Reframes returns a discussion's framing supersessions.
func (s *Store) Reframes(disc string, w When) ([]ibis.Reframe, error) {
	all, err := scan[ibis.Reframe](s, relReframe, w)
	return filter(all, disc, func(r ibis.Reframe) string { return r.Disc }), err
}

// Values returns a discussion's node→value tags.
func (s *Store) Values(disc string, w When) ([]ibis.ValueTag, error) {
	all, err := scan[ibis.ValueTag](s, relValue, w)
	return filter(all, disc, func(v ibis.ValueTag) string { return v.Disc }), err
}

// Audiences returns a discussion's currently declared value rankings
// (superseded ones are vt-closed and excluded by the default viewpoint).
func (s *Store) Audiences(disc string, w When) ([]ibis.Audience, error) {
	all, err := scan[ibis.Audience](s, relAudience, w)
	return filter(all, disc, func(a ibis.Audience) string { return a.Disc }), err
}

// Rosters returns the distinct author↔role bindings recorded for a discussion,
// in canonical (author, role) order so output is deterministic. Every move
// under a role re-records the binding, so the raw facts hold one row per move;
// this collapses them to one row per binding — the roster is the attribution
// audit, not a move log (wicked-problems-2.md item 10).
func (s *Store) Rosters(disc string, w When) ([]ibis.Roster, error) {
	all, err := scan[ibis.Roster](s, relRoster, w)
	if err != nil {
		return nil, err
	}
	filtered := filter(all, disc, func(r ibis.Roster) string { return r.Disc })
	seen := map[[2]string]bool{}
	out := filtered[:0]
	for _, r := range filtered {
		key := [2]string{r.Author, r.Role}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Author != out[j].Author {
			return out[i].Author < out[j].Author
		}
		return out[i].Role < out[j].Role
	})
	return out, nil
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
	reframes, err := s.Reframes(disc, gw)
	if err != nil {
		return nil, err
	}
	values, err := s.Values(disc, gw)
	if err != nil {
		return nil, err
	}
	// Audiences ride the full viewpoint: supersession closes the vt interval,
	// so --valid-at answers "which rankings governed at T".
	audiences, err := s.Audiences(disc, w)
	if err != nil {
		return nil, err
	}
	return ibis.NewGraph(nodes, links, prefs, cards, reframes, values, audiences), nil
}

// SupersedeDecision invalidates (closes the vt interval of) any current decision
// on the given issue, so a fresh decide supersedes it. No-op if none stands.
// DecisionTxStart returns the transaction time at which the current standing
// decision on an issue was recorded, for bitemporally deriving the state as of
// that moment (a map decision's decision-time verdict map is computed as of
// this time rather than snapshotted; wicked-problems-2.md item 7). ok is false
// when the issue carries no standing decision.
func (s *Store) DecisionTxStart(disc, issue string) (int64, bool, error) {
	facts, err := s.ops.QueryFacts(factstore.FactFilter{Relation: relDecision})
	if err != nil {
		return 0, false, fail.Store("query decisions: %v", err)
	}
	for _, f := range facts {
		var d ibis.Decision
		if err := json.Unmarshal([]byte(f.Args), &d); err != nil {
			continue
		}
		if d.Disc == disc && d.Issue == issue {
			return f.TxStart, true, nil
		}
	}
	return 0, false, nil
}

// SupersedeDecision closes the tt interval of the standing decision(s) on an
// issue. A non-empty position closes only the decision on that position — the
// unit of supersession on an open issue, where sibling per-position decisions
// must stand; an empty position closes every decision on the issue (select_one,
// which has at most one).
func (s *Store) SupersedeDecision(disc, issue, position string) error {
	facts, err := s.ops.QueryFacts(factstore.FactFilter{Relation: relDecision})
	if err != nil {
		return fail.Store("query decisions: %v", err)
	}
	for _, f := range facts {
		var d ibis.Decision
		if err := json.Unmarshal([]byte(f.Args), &d); err != nil {
			continue
		}
		if d.Disc == disc && d.Issue == issue && (position == "" || d.Position == position) {
			if err := s.ops.InvalidateFact(f.ID); err != nil {
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
	facts, err := s.ops.QueryFacts(factstore.FactFilter{Relation: relNode})
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
	if err := s.ops.RetractFact(target); err != nil {
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
		facts, err := s.ops.QueryFacts(factstore.FactFilter{Relation: rel})
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
	if err := emit(relRoster, discIs); err != nil {
		return nil, err
	}
	if err := emit(relReframe, discIs); err != nil {
		return nil, err
	}
	if err := emit(relValue, discIs); err != nil {
		return nil, err
	}
	if err := emit(relAudience, discIs); err != nil {
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
		facts, err := s.ops.FactHistory(rel)
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
	for _, rel := range []string{relLink, relPreference, relDecision, relRoster, relReframe, relValue, relAudience} {
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
	facts, err := s.ops.FactHistory(rec.Relation)
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
			if err := s.ops.RetractFact(f.ID); err != nil {
				return fail.Store("replay retract %s: %v", f.ID, err)
			}
		case EventInvalidate:
			if f.ValidEnd != nil {
				return nil // already invalidated
			}
			if err := s.ops.InvalidateFact(f.ID); err != nil {
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
	_, err := s.ops.AddFact(factstore.Fact{
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
// errors are IllegalMove (exit 2) and nothing is written. The store-level
// check and the writes run in one Move transaction, so a concurrent writer
// cannot slip a conflicting preference between the acyclicity check and the
// batch landing, and a mid-batch failure rolls back the whole batch.
func (s *Store) ImportAll(recs []ExportRecord) (int, error) {
	prefsByDisc := map[string][]ibis.Preference{}
	for i, rec := range recs {
		if err := validateRecord(rec, i+1, prefsByDisc); err != nil {
			return 0, err
		}
	}
	err := s.Move(func(txs *Store) error {
		for disc, incoming := range prefsByDisc {
			existing, err := txs.Preferences(disc, Now())
			if err != nil {
				return err
			}
			if node, cyclic := af.PreferenceCycle(append(existing, incoming...)); cyclic {
				return &ibis.IllegalMove{Node: node, Detail: fmt.Sprintf(
					"import would create a preference cycle through %s in discussion %s; nothing imported", node, disc)}
			}
		}
		for _, rec := range recs {
			if rec.Event != "" {
				if err := txs.applyEvent(rec); err != nil {
					return err
				}
				continue
			}
			if err := txs.Import(rec); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
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
		if v.Tag != "" && v.Tag != ibis.TagAssumption {
			return bad("node tag %q invalid (only %q)", v.Tag, ibis.TagAssumption)
		}
		return firstErr(require("id", v.ID), require("disc", v.Disc))
	case relLink:
		var v ibis.Link
		if err := parse(&v); err != nil {
			return err
		}
		switch v.Rel {
		case ibis.RespondsTo, ibis.Supports, ibis.ObjectsTo, ibis.Synthesizes, ibis.RaisedFrom, ibis.Addresses:
		default:
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
	case relRoster:
		var v ibis.Roster
		if err := parse(&v); err != nil {
			return err
		}
		return firstErr(require("disc", v.Disc), require("author", v.Author), require("role", v.Role))
	case relReframe:
		var v ibis.Reframe
		if err := parse(&v); err != nil {
			return err
		}
		return firstErr(require("disc", v.Disc), require("old", v.Old), require("new", v.New), require("basis", v.Basis))
	case relValue:
		var v ibis.ValueTag
		if err := parse(&v); err != nil {
			return err
		}
		return firstErr(require("disc", v.Disc), require("node", v.Node), require("value", v.Value))
	case relAudience:
		var v ibis.Audience
		if err := parse(&v); err != nil {
			return err
		}
		if len(v.Ranking) < 2 {
			return bad("audience %q ranking must list at least two values", v.Name)
		}
		return firstErr(require("disc", v.Disc), require("name", v.Name))
	default:
		return bad("unknown relation %q", rec.Relation)
	}
}

func knownRelation(rel string) bool {
	switch rel {
	case relDiscussion, relNode, relLink, relIssueCard, relPreference, relDecision, relRoster,
		relReframe, relValue, relAudience:
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
	for _, rel := range []string{relNode, relLink, relPreference, relDecision, relReframe, relValue, relAudience} {
		facts, err := s.ops.FactHistory(rel)
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
	case relReframe:
		return fmt.Sprintf("reframe %s→%s basis=%s", str("old"), str("new"), str("basis"))
	case relValue:
		return fmt.Sprintf("promote %s value=%s", str("node"), str("value"))
	case relAudience:
		return fmt.Sprintf("audience %s", str("name"))
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
