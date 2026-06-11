// Package mcpserv serves dlktk over the Model Context Protocol (stdio), so any
// MCP-capable agent harness can drive a dialectic without shelling out to the
// CLI. The tool surface mirrors the CLI verbs one-to-one and returns the same
// JSON envelopes the CLI emits under --format json; errors come back as the
// same structured envelope (error kind, detail, node) with IsError set.
//
// Moves are serialized under one mutex, which closes the check-then-write race
// the per-process CLI cannot (two concurrent prefers forming a cycle).
package mcpserv

import (
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/check"
	"github.com/chazu/dlktk/internal/discover"
	"github.com/chazu/dlktk/internal/fail"
	"github.com/chazu/dlktk/internal/ibis"
	"github.com/chazu/dlktk/internal/id"
	"github.com/chazu/dlktk/internal/proto"
	"github.com/chazu/dlktk/internal/render"
	"github.com/chazu/dlktk/internal/store"
)

const instructions = `dlktk records design discussions as an IBIS argument graph and computes which
positions currently stand (IN), are defeated (OUT), or are genuinely contested
(UNDEC) via Dung grounded semantics. The loop: agenda (the worklist) -> moves
(legal next moves for an issue) -> make a move (raise/propose/support/object/
prefer/decide) -> re-read. Use why/show/explain to understand a label, search
to check whether a claim already exists before adding a duplicate, and check to
verify recorded decisions still stand. Decisions on decided issues must be
overturned with supersede (basis required), never re-decided.`

type srv struct {
	s             *store.Store
	mu            sync.Mutex // serializes moves: legality check + write as one critical section
	defaultAuthor string
}

// Serve runs the MCP server over the given transport until the client
// disconnects. defaultAuthor attributes moves that pass no author.
func Serve(ctx context.Context, s *store.Store, defaultAuthor string, t mcp.Transport) error {
	server := NewServer(s, defaultAuthor)
	return server.Run(ctx, t)
}

// NewServer builds the dlktk MCP server (exposed for in-memory tests).
func NewServer(s *store.Store, defaultAuthor string) *mcp.Server {
	x := &srv{s: s, defaultAuthor: defaultAuthor}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "dlktk", Version: discover.Version},
		&mcp.ServerOptions{Instructions: instructions},
	)

	// --- moves ---
	mcp.AddTool(server, &mcp.Tool{Name: "new", Description: "create a discussion"}, x.newDisc)
	mcp.AddTool(server, &mcp.Tool{Name: "raise", Description: "raise an issue (cardinality select_one|open is fixed at creation, default select_one)"}, x.raise)
	mcp.AddTool(server, &mcp.Tool{Name: "propose", Description: "propose a position on an issue"}, x.propose)
	mcp.AddTool(server, &mcp.Tool{Name: "support", Description: "argue in support of a position or argument (rationale only; does not affect labels)"}, x.support)
	mcp.AddTool(server, &mcp.Tool{Name: "object", Description: "object to a position or argument (an attack; feeds the labelling)"}, x.object)
	mcp.AddTool(server, &mcp.Tool{Name: "prefer", Description: "state a preference (winner over loser, with a basis); neutralizes the loser's attack on the winner"}, x.prefer)
	mcp.AddTool(server, &mcp.Tool{Name: "decide", Description: "close an issue by accepting a position; rejected if the issue is already decided (use supersede)"}, x.decide)
	mcp.AddTool(server, &mcp.Tool{Name: "supersede", Description: "overturn the standing decision on an issue; basis is required and the prior decision is linked"}, x.supersede)
	mcp.AddTool(server, &mcp.Tool{Name: "concede", Description: "withdraw one of your own nodes"}, x.concede)

	// --- reads ---
	mcp.AddTool(server, &mcp.Tool{Name: "list", Description: "list discussions"}, x.list)
	mcp.AddTool(server, &mcp.Tool{Name: "status", Description: "grounded labelling of an issue's positions (all issues if none given)"}, x.status)
	mcp.AddTool(server, &mcp.Tool{Name: "agenda", Description: "the worklist: UNDEC nodes, issues ready to decide, issues with no positions"}, x.agenda)
	mcp.AddTool(server, &mcp.Tool{Name: "moves", Description: "legal + useful next moves for an issue"}, x.moves)
	mcp.AddTool(server, &mcp.Tool{Name: "why", Description: "explain a node's label and how to flip it"}, x.why)
	mcp.AddTool(server, &mcp.Tool{Name: "show", Description: "one node in full: text, author, label, every incident link"}, x.show)
	mcp.AddTool(server, &mcp.Tool{Name: "search", Description: "find nodes whose text matches (check for an existing argument before duplicating it)"}, x.search)
	mcp.AddTool(server, &mcp.Tool{Name: "explain", Description: "full derivation of an issue's labelling: attacks, preferences, fixpoint rounds, outcome"}, x.explain)
	mcp.AddTool(server, &mcp.Tool{Name: "check", Description: "verify standing decisions: drift, stalemates, store invariants"}, x.checkTool)

	return server
}

// ok wraps a view as a JSON text result — the same envelope the CLI prints.
func ok(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return bad(err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

// bad wraps an error as the CLI's structured error envelope with IsError set.
func bad(err error) (*mcp.CallToolResult, any, error) {
	e := fail.Classify(err)
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: e.JSON()}},
	}, nil, nil
}

func (x *srv) mover(author string) *proto.Mover {
	if author == "" {
		author = x.defaultAuthor
	}
	return proto.New(x.s, author)
}

// framework loads the present-time graph, labelling, and decisions.
func (x *srv) framework(disc string) (*ibis.Graph, *af.Framework, map[string]af.Label, []ibis.Decision, error) {
	g, err := x.s.Graph(disc, store.Now())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	decs, err := x.s.Decisions(disc, store.Now())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	fw, err := af.Build(g)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return g, fw, fw.Grounded(), decs, nil
}

// --- move handlers ---

type newArgs struct {
	Title   string `json:"title" jsonschema:"discussion title"`
	Subject string `json:"subject,omitempty" jsonschema:"subject ref anchoring the discussion to code (file:… pkg:… commit:… q:…)"`
	Author  string `json:"author,omitempty" jsonschema:"author attribution (defaults to the server's author)"`
}

func (x *srv) newDisc(ctx context.Context, req *mcp.CallToolRequest, a newArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	author := a.Author
	if author == "" {
		author = x.defaultAuthor
	}
	disc := id.New()
	if err := x.s.AddDiscussion(ibis.Discussion{ID: disc, Title: a.Title, Subject: a.Subject, CreatedBy: author}); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": disc})
}

type raiseArgs struct {
	Discussion  string `json:"discussion" jsonschema:"discussion id"`
	Text        string `json:"text" jsonschema:"the question at stake"`
	Parent      string `json:"parent,omitempty" jsonschema:"parent issue id"`
	Cardinality string `json:"cardinality,omitempty" jsonschema:"select_one (default) or open; fixed at creation"`
	Author      string `json:"author,omitempty"`
}

func (x *srv) raise(ctx context.Context, req *mcp.CallToolRequest, a raiseArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	switch a.Cardinality {
	case "", string(ibis.SelectOne), string(ibis.Open):
	default:
		return bad(&ibis.IllegalMove{Detail: "cardinality must be select_one or open, got " + a.Cardinality})
	}
	nid, err := x.mover(a.Author).Raise(a.Discussion, a.Text, a.Parent, ibis.Cardinality(a.Cardinality))
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type proposeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Text       string `json:"text" jsonschema:"the candidate answer"`
	Author     string `json:"author,omitempty"`
}

func (x *srv) propose(ctx context.Context, req *mcp.CallToolRequest, a proposeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author).Propose(a.Discussion, a.Issue, a.Text)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type attachArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Target     string `json:"target" jsonschema:"position or argument id"`
	Text       string `json:"text" jsonschema:"the claim"`
	Author     string `json:"author,omitempty"`
}

func (x *srv) support(ctx context.Context, req *mcp.CallToolRequest, a attachArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author).Support(a.Discussion, a.Target, a.Text)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

func (x *srv) object(ctx context.Context, req *mcp.CallToolRequest, a attachArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author).Object(a.Discussion, a.Target, a.Text)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type preferArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Winner     string `json:"winner" jsonschema:"preferred node id"`
	Loser      string `json:"loser" jsonschema:"node id it beats"`
	Basis      string `json:"basis,omitempty" jsonschema:"why (security, velocity, throughput, …)"`
	Author     string `json:"author,omitempty"`
}

func (x *srv) prefer(ctx context.Context, req *mcp.CallToolRequest, a preferArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	pid, err := x.mover(a.Author).Prefer(a.Discussion, a.Winner, a.Loser, a.Basis)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": pid})
}

type decideArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Position   string `json:"position" jsonschema:"accepted position id"`
	Basis      string `json:"basis,omitempty" jsonschema:"why"`
	Author     string `json:"author,omitempty"`
}

func (x *srv) decide(ctx context.Context, req *mcp.CallToolRequest, a decideArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author).Decide(a.Discussion, a.Issue, a.Position, a.Basis); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"issue": a.Issue, "position": a.Position})
}

type supersedeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Position   string `json:"position" jsonschema:"newly accepted position id"`
	Basis      string `json:"basis" jsonschema:"why the prior decision is overturned (required)"`
	Author     string `json:"author,omitempty"`
}

func (x *srv) supersede(ctx context.Context, req *mcp.CallToolRequest, a supersedeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author).Supersede(a.Discussion, a.Issue, a.Position, a.Basis); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"issue": a.Issue, "position": a.Position})
}

type concedeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Node       string `json:"node" jsonschema:"id of one of your own nodes"`
	Author     string `json:"author,omitempty"`
}

func (x *srv) concede(ctx context.Context, req *mcp.CallToolRequest, a concedeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author).Concede(a.Discussion, a.Node); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"conceded": a.Node})
}

// --- read handlers ---

type noArgs struct{}

func (x *srv) list(ctx context.Context, req *mcp.CallToolRequest, _ noArgs) (*mcp.CallToolResult, any, error) {
	ds, err := x.s.Discussions(store.Now())
	if err != nil {
		return bad(err)
	}
	return ok(map[string]any{"discussions": ds})
}

type statusArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue,omitempty" jsonschema:"issue id (all issues if omitted)"`
}

func (x *srv) status(ctx context.Context, req *mcp.CallToolRequest, a statusArgs) (*mcp.CallToolResult, any, error) {
	g, fw, labels, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	issues := g.Issues()
	if a.Issue != "" {
		if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
			return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
		}
		issues = []string{a.Issue}
	}
	var views []render.IssueStatus
	for _, iss := range issues {
		views = append(views, render.Status(g, fw, labels, iss, decs))
	}
	return ok(map[string]any{"issues": views})
}

type discArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
}

func (x *srv) agenda(ctx context.Context, req *mcp.CallToolRequest, a discArgs) (*mcp.CallToolResult, any, error) {
	g, _, labels, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	return ok(render.Agenda(g, labels, decs))
}

type issueArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
}

func (x *srv) moves(ctx context.Context, req *mcp.CallToolRequest, a issueArgs) (*mcp.CallToolResult, any, error) {
	g, fw, labels, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
		return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
	}
	return ok(render.Moves(g, fw, labels, a.Issue, decs))
}

type nodeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Node       string `json:"node" jsonschema:"node id"`
}

func (x *srv) why(ctx context.Context, req *mcp.CallToolRequest, a nodeArgs) (*mcp.CallToolResult, any, error) {
	g, fw, labels, _, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if !g.IsAFNode(a.Node) {
		return bad(fail.NotFound(a.Node, "%q is not a position or argument", a.Node))
	}
	return ok(render.Why(g, fw, labels, a.Node))
}

func (x *srv) show(ctx context.Context, req *mcp.CallToolRequest, a nodeArgs) (*mcp.CallToolResult, any, error) {
	g, _, labels, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if _, found := g.Nodes[a.Node]; !found {
		return bad(fail.NotFound(a.Node, "node %q not found", a.Node))
	}
	return ok(render.Show(g, labels, a.Node, decs))
}

type searchArgs struct {
	Query      string `json:"query" jsonschema:"case-insensitive substring to match against node text"`
	Discussion string `json:"discussion,omitempty" jsonschema:"discussion id (all discussions if omitted)"`
}

func (x *srv) search(ctx context.Context, req *mcp.CallToolRequest, a searchArgs) (*mcp.CallToolResult, any, error) {
	discs := []string{a.Discussion}
	if a.Discussion == "" {
		ds, err := x.s.Discussions(store.Now())
		if err != nil {
			return bad(err)
		}
		discs = discs[:0]
		for _, d := range ds {
			discs = append(discs, d.ID)
		}
		sort.Strings(discs)
	}
	v := render.SearchView{Query: a.Query}
	for _, disc := range discs {
		g, err := x.s.Graph(disc, store.Now())
		if err != nil {
			return bad(err)
		}
		var labels map[string]af.Label
		if fw, err := af.Build(g); err == nil {
			labels = fw.Grounded()
		}
		v.Hits = append(v.Hits, render.Search(disc, g, labels, a.Query)...)
	}
	return ok(v)
}

func (x *srv) explain(ctx context.Context, req *mcp.CallToolRequest, a issueArgs) (*mcp.CallToolResult, any, error) {
	g, fw, _, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
		return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
	}
	return ok(render.Explain(g, fw, a.Issue, decs))
}

type checkArgs struct {
	Discussion string `json:"discussion,omitempty" jsonschema:"discussion id (all discussions if omitted)"`
}

func (x *srv) checkTool(ctx context.Context, req *mcp.CallToolRequest, a checkArgs) (*mcp.CallToolResult, any, error) {
	discs := []string{a.Discussion}
	if a.Discussion == "" {
		ds, err := x.s.Discussions(store.Now())
		if err != nil {
			return bad(err)
		}
		discs = discs[:0]
		for _, d := range ds {
			discs = append(discs, d.ID)
		}
	}
	v, err := check.Run(x.s, discs, store.Now())
	if err != nil {
		return bad(err)
	}
	return ok(v)
}
