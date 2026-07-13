// Package ibis holds the IBIS domain types (the L1 capture ontology) and the
// move-legality rules (part of L4). It knows nothing of storage or the CLI.
package ibis

import "sort"

// Kind enumerates node kinds.
type Kind string

const (
	Issue    Kind = "issue"
	Position Kind = "position"
	Argument Kind = "argument"
)

// Rel enumerates link relations.
type Rel string

const (
	RespondsTo  Rel = "responds_to"
	Supports    Rel = "supports"
	ObjectsTo   Rel = "objects_to"
	Synthesizes Rel = "synthesizes" // hybrid position -> parent position (lineage; never reaches the evaluator)
	RaisedFrom  Rel = "raised_from" // issue -> the position/argument that revealed it (provenance; never reaches the evaluator)
	Addresses   Rel = "addresses"   // hybrid-side argument -> a parent's objection it answers (discharge; never reaches the evaluator)
)

// Cardinality of an issue: select_one positions are mutually exclusive; open
// positions may be independently acceptable.
type Cardinality string

const (
	SelectOne Cardinality = "select_one"
	Open      Cardinality = "open"
)

// TagAssumption marks an argument as a challengeable premise its target rests
// on. Tags are bookkeeping only: they never reach the evaluator (§3.5 holds);
// agenda/check use them to surface unexamined or defeated premises.
const TagAssumption = "assumption"

// Node is an IBIS node. Stored as a dlktk/node fact (args = these fields).
// Drops records what a synthesis explicitly excludes from its parents (one
// entry per exclusion) — pure metadata, rendered by show/tree; a synthesis
// that drops nothing is a bundle (wicked-problems-2.md item 4).
type Node struct {
	ID     string   `json:"id"`
	Disc   string   `json:"disc"`
	Kind   Kind     `json:"kind"`
	Text   string   `json:"text"`
	Author string   `json:"author"`
	Tag    string   `json:"tag,omitempty"`   // "assumption" only, for now
	Drops  []string `json:"drops,omitempty"` // syntheses only
}

// Link is an IBIS link. Stored as a dlktk/link fact.
type Link struct {
	ID     string `json:"id"`
	Disc   string `json:"disc"`
	Src    string `json:"src"`
	Dst    string `json:"dst"`
	Rel    Rel    `json:"rel"`
	Author string `json:"author"`
}

// Preference records that Winner is preferred to Loser, with a basis label.
type Preference struct {
	ID     string `json:"id"`
	Disc   string `json:"disc"`
	Winner string `json:"winner"`
	Loser  string `json:"loser"`
	Basis  string `json:"basis"`
	Author string `json:"author"`
}

// Decision records the act of closing an issue by accepting a position.
// Supersedes names the previously decided position when this decision was made
// with the supersede move (design §16 Q4: overturning requires a recorded basis
// and a link back; a bare re-decide is rejected).
type Decision struct {
	Disc       string `json:"disc"`
	Issue      string `json:"issue"`
	Position   string `json:"position"` // empty for a map decision (the object is the whole issue's audience map)
	Basis      string `json:"basis"`
	Decider    string `json:"decider"`
	Override   bool   `json:"override"`
	Supersedes string `json:"supersedes,omitempty"`
	ReviewBy   int64  `json:"review_by,omitempty"` // unix seconds; the recorded re-examination horizon (0 = none)
	// Kind distinguishes a conventional position decision (empty/"position")
	// from a "map" decision, whose object is the issue's audience-conditional
	// verdict map rather than a single winner (wicked-problems-2.md item 7). A
	// map decision snapshots no verdicts — the decision-time map is derived
	// bitemporally as of this fact's transaction time.
	Kind string `json:"kind,omitempty"`
	// SupersededKind records the kind of the decision this one overturned, so a
	// map->position or position->map transition is legible on the record.
	SupersededKind string `json:"superseded_kind,omitempty"`
}

// Decision kinds. The empty string is a conventional position decision.
const (
	MapDecision = "map"
)

// Reframe records that Old's framing was replaced by New, and why. The old
// issue's graph is untouched (append-only ethos) but it leaves the live agenda:
// the question has moved. Basis is mandatory — reframing is the most
// consequential move a deliberation makes, so its reasoning must be captured
// (the Q4 ethos applied to framings).
type Reframe struct {
	Disc   string `json:"disc"`
	Old    string `json:"old"`
	New    string `json:"new"`
	Basis  string `json:"basis"`
	Author string `json:"author"`
}

// ValueTag records that a node promotes a value (throughput, security, …), for
// audience-relative evaluation (value-based AF). One value per live node; a
// dangling tag (node conceded) is ignored on load.
type ValueTag struct {
	Disc   string `json:"disc"`
	Node   string `json:"node"`
	Value  string `json:"value"`
	Author string `json:"author"`
}

// Audience is a named strict ranking over values (Ranking[0] is the most
// important). Under an audience, an attack on a node promoting a strictly
// higher-ranked value fails. Re-declaring a name requires supersession with a
// recorded basis (the Q4 pattern).
type Audience struct {
	Disc    string   `json:"disc"`
	Name    string   `json:"name"`
	Ranking []string `json:"ranking"`
	Basis   string   `json:"basis,omitempty"`
	Author  string   `json:"author"`
}

// Discussion is the unit of scope.
type Discussion struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Subject   string `json:"subject"`
	CreatedBy string `json:"created_by"`
}

// IssueCard carries an issue's cardinality.
type IssueCard struct {
	Issue       string      `json:"issue"`
	Cardinality Cardinality `json:"cardinality"`
}

// Roster binds an author (stable identity) to a role (persona) within a
// discussion. Pure metadata: it never reaches the evaluator (design §11). The
// binding is auto-recorded the first time an author moves under a --role, and
// can also be pre-declared with the `roster` move (design §16 Q8). Author is the
// ownership identity that concede/retract check, distinct from the persona.
type Roster struct {
	Disc   string `json:"disc"`
	Author string `json:"author"`
	Role   string `json:"role"`
}

// PrefixFor returns the presentational one-char kind prefix for a node id.
func PrefixFor(k Kind) string {
	switch k {
	case Issue:
		return "i:"
	case Position:
		return "p:"
	case Argument:
		return "a:"
	}
	return ""
}

// Graph is an in-memory snapshot of one discussion, indexed for legality checks
// and AF construction. Build it from store reads.
type Graph struct {
	Nodes       map[string]Node
	Links       []Link
	Preferences []Preference
	IssueCards  map[string]Cardinality // issue id -> cardinality
	Reframes    []Reframe
	Values      map[string]string   // node id -> promoted value (live nodes only)
	Audiences   map[string]Audience // name -> currently declared audience
}

// NewGraph builds an indexed graph from slices. Value tags whose node is absent
// (conceded and restated) are dropped: they are the expected residue of the
// change-a-value path and must not influence any audience lens.
func NewGraph(nodes []Node, links []Link, prefs []Preference, cards []IssueCard, reframes []Reframe, values []ValueTag, audiences []Audience) *Graph {
	g := &Graph{
		Nodes:       make(map[string]Node, len(nodes)),
		Links:       links,
		Preferences: prefs,
		IssueCards:  make(map[string]Cardinality, len(cards)),
		Reframes:    reframes,
		Values:      make(map[string]string, len(values)),
		Audiences:   make(map[string]Audience, len(audiences)),
	}
	for _, n := range nodes {
		g.Nodes[n.ID] = n
	}
	for _, c := range cards {
		g.IssueCards[c.Issue] = c.Cardinality
	}
	for _, v := range values {
		if _, ok := g.Nodes[v.Node]; ok {
			g.Values[v.Node] = v.Value
		}
	}
	for _, a := range audiences {
		g.Audiences[a.Name] = a
	}
	return g
}

// ReframedTo returns the issue that replaced this one's framing, if any.
func (g *Graph) ReframedTo(issue string) (string, bool) {
	for _, r := range g.Reframes {
		if r.Old == issue {
			return r.New, true
		}
	}
	return "", false
}

// SynthesisParents returns the direct parents of a synthesis (its synthesizes
// out-links), sorted; nil for a non-synthesis.
func (g *Graph) SynthesisParents(node string) []string {
	var out []string
	for _, l := range g.Links {
		if l.Rel == Synthesizes && l.Src == node {
			out = append(out, l.Dst)
		}
	}
	sort.Strings(out)
	return out
}

// SynthesisAncestors returns the transitive closure of a node's synthesizes
// lineage, sorted — chained syntheses (H2 = H1+C, H1 = A+B) reach A and B, so
// chaining cannot launder a parent's critics (wicked-problems-2.md item 2).
func (g *Graph) SynthesisAncestors(node string) []string {
	seen := map[string]bool{}
	stack := g.SynthesisParents(node)
	var out []string
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		out = append(out, cur)
		stack = append(stack, g.SynthesisParents(cur)...)
	}
	sort.Strings(out)
	return out
}

// IsAFNode reports whether a node participates in the argumentation framework
// (positions and arguments are AF nodes; issues are not).
func (g *Graph) IsAFNode(nodeID string) bool {
	n, ok := g.Nodes[nodeID]
	return ok && (n.Kind == Position || n.Kind == Argument)
}

// Issues returns the graph's issue ids in canonical (sorted) order.
func (g *Graph) Issues() []string {
	var out []string
	for id, n := range g.Nodes {
		if n.Kind == Issue {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
