// Package ibis holds the IBIS domain types (the L1 capture ontology) and the
// move-legality rules (part of L4). It knows nothing of storage or the CLI.
package ibis

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
	RespondsTo Rel = "responds_to"
	Supports   Rel = "supports"
	ObjectsTo  Rel = "objects_to"
)

// Cardinality of an issue: select_one positions are mutually exclusive; open
// positions may be independently acceptable.
type Cardinality string

const (
	SelectOne Cardinality = "select_one"
	Open      Cardinality = "open"
)

// Node is an IBIS node. Stored as a dlktk/node fact (args = these fields).
type Node struct {
	ID     string `json:"id"`
	Disc   string `json:"disc"`
	Kind   Kind   `json:"kind"`
	Text   string `json:"text"`
	Author string `json:"author"`
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
	Position   string `json:"position"`
	Basis      string `json:"basis"`
	Decider    string `json:"decider"`
	Override   bool   `json:"override"`
	Supersedes string `json:"supersedes,omitempty"`
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
}

// NewGraph builds an indexed graph from slices.
func NewGraph(nodes []Node, links []Link, prefs []Preference, cards []IssueCard) *Graph {
	g := &Graph{
		Nodes:       make(map[string]Node, len(nodes)),
		Links:       links,
		Preferences: prefs,
		IssueCards:  make(map[string]Cardinality, len(cards)),
	}
	for _, n := range nodes {
		g.Nodes[n.ID] = n
	}
	for _, c := range cards {
		g.IssueCards[c.Issue] = c.Cardinality
	}
	return g
}

// IsAFNode reports whether a node participates in the argumentation framework
// (positions and arguments are AF nodes; issues are not).
func (g *Graph) IsAFNode(nodeID string) bool {
	n, ok := g.Nodes[nodeID]
	return ok && (n.Kind == Position || n.Kind == Argument)
}
