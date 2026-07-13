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
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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
assume/prefer/decide) -> re-read. Use why/show/explain to understand a label,
whatif/crux to explore counterfactuals without writing, worlds to see the
coherent stances a contested issue admits, audiences to see which conclusions
survive every declared value ranking, and search to check whether a claim
already exists before adding a duplicate. An IN position that never faced a
substantive objection — one from another author that participates in the
defeat relation; select_one rival edges never count — is untested, not
vindicated: stress-test it before deciding. Stalemates have
three exits: prefer (a value call), synthesize (a recorded hybrid), or reframe
(replace a mis-framed question; basis required). A synthesis inherits its
parents' undefeated objections as open questions (shown by show/why/moves) —
discharge them with object/support --answers <objection-id> — and should say
what it drops (--drops; a synthesis that drops nothing is a bundle). Prefer
warns when it would bury a parent under its own hybrid with those questions
open. Decisions on decided issues must be overturned with supersede (basis
required), never re-decided; check verifies recorded decisions still stand
and their review horizons have not passed.`

type srv struct {
	s             *store.Store
	mu            sync.Mutex // serializes moves: legality check + write as one critical section
	defaultAuthor string
	defaultRole   string
}

// Serve runs the MCP server over the given transport until the client
// disconnects. defaultAuthor/defaultRole attribute moves that pass none of their
// own (author is the ownership identity, role the optional persona).
func Serve(ctx context.Context, s *store.Store, defaultAuthor, defaultRole string, t mcp.Transport) error {
	server := NewServer(s, defaultAuthor, defaultRole)
	return server.Run(ctx, t)
}

// NewServer builds the dlktk MCP server (exposed for in-memory tests).
func NewServer(s *store.Store, defaultAuthor, defaultRole string) *mcp.Server {
	x := &srv{s: s, defaultAuthor: defaultAuthor, defaultRole: defaultRole}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "dlktk", Version: discover.Version},
		&mcp.ServerOptions{Instructions: instructions},
	)

	// --- moves ---
	mcp.AddTool(server, &mcp.Tool{Name: "new", Description: "create a discussion"}, x.newDisc)
	mcp.AddTool(server, &mcp.Tool{Name: "raise", Description: "raise an issue (cardinality select_one|open is fixed at creation, default select_one); from records the position/argument that revealed it"}, x.raise)
	mcp.AddTool(server, &mcp.Tool{Name: "reframe", Description: "replace an issue's framing with a fresh issue (basis required; positions do not carry over; lineage recorded)"}, x.reframe)
	mcp.AddTool(server, &mcp.Tool{Name: "propose", Description: "propose a position on an issue"}, x.propose)
	mcp.AddTool(server, &mcp.Tool{Name: "synthesize", Description: "propose a hybrid position recombining two or more existing positions (lineage recorded); it joins the rivalry until parents are conceded or a preference/audience elevates it"}, x.synthesize)
	mcp.AddTool(server, &mcp.Tool{Name: "support", Description: "argue in support of a position or argument (rationale only; does not affect labels)"}, x.support)
	mcp.AddTool(server, &mcp.Tool{Name: "object", Description: "object to a position or argument (an attack; feeds the labelling)"}, x.object)
	mcp.AddTool(server, &mcp.Tool{Name: "assume", Description: "record an assumption the target rests on (a challengeable premise; inert in the labelling, tracked by agenda/check)"}, x.assume)
	mcp.AddTool(server, &mcp.Tool{Name: "prefer", Description: "state a preference (winner over loser, with a basis); neutralizes the loser's attack on the winner"}, x.prefer)
	mcp.AddTool(server, &mcp.Tool{Name: "promote", Description: "tag one of your own nodes with the value it promotes (audience lens input)"}, x.promote)
	mcp.AddTool(server, &mcp.Tool{Name: "audience", Description: "declare a named strict value ranking (re-declaring requires supersede=true and a basis)"}, x.audience)
	mcp.AddTool(server, &mcp.Tool{Name: "decide", Description: "close an issue by accepting a position; rejected if the issue is already decided (use supersede); review_by records a re-examination horizon"}, x.decide)
	mcp.AddTool(server, &mcp.Tool{Name: "supersede", Description: "overturn the standing decision on an issue; basis is required and the prior decision is linked"}, x.supersede)
	mcp.AddTool(server, &mcp.Tool{Name: "concede", Description: "withdraw one of your own nodes"}, x.concede)

	// --- reads ---
	mcp.AddTool(server, &mcp.Tool{Name: "list", Description: "list discussions"}, x.list)
	mcp.AddTool(server, &mcp.Tool{Name: "roster", Description: "list the author↔role bindings recorded for a discussion"}, x.roster)
	mcp.AddTool(server, &mcp.Tool{Name: "status", Description: "grounded labelling of an issue's positions (all issues if none given)"}, x.status)
	mcp.AddTool(server, &mcp.Tool{Name: "agenda", Description: "the worklist: UNDEC nodes, issues ready to decide, issues with no positions"}, x.agenda)
	mcp.AddTool(server, &mcp.Tool{Name: "moves", Description: "legal + useful next moves for an issue"}, x.moves)
	mcp.AddTool(server, &mcp.Tool{Name: "why", Description: "explain a node's label and how to flip it"}, x.why)
	mcp.AddTool(server, &mcp.Tool{Name: "show", Description: "one node in full: text, author, label, every incident link"}, x.show)
	mcp.AddTool(server, &mcp.Tool{Name: "search", Description: "find nodes whose text matches (check for an existing argument before duplicating it)"}, x.search)
	mcp.AddTool(server, &mcp.Tool{Name: "explain", Description: "full derivation of an issue's labelling: attacks, preferences, fixpoint rounds, outcome"}, x.explain)
	mcp.AddTool(server, &mcp.Tool{Name: "whatif", Description: "counterfactual: apply hypothetical moves (object/prefer/without) in memory and report the label diff — nothing is written"}, x.whatif)
	mcp.AddTool(server, &mcp.Tool{Name: "crux", Description: "the load-bearing arguments: which single argument's removal flips a position of the issue"}, x.crux)
	mcp.AddTool(server, &mcp.Tool{Name: "worlds", Description: "enumerate the coherent maximal stances (preferred extensions) on an issue; positions sorted robust/contingent/hopeless"}, x.worlds)
	mcp.AddTool(server, &mcp.Tool{Name: "audiences", Description: "cross-audience sensitivity report: which positions are justified under every declared value ranking vs audience-sensitive"}, x.audiences)
	mcp.AddTool(server, &mcp.Tool{Name: "check", Description: "verify standing decisions: drift, stalemates, untested/review-due decisions, store invariants"}, x.checkTool)

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

func (x *srv) mover(author, role string) *proto.Mover {
	if author == "" {
		author = x.defaultAuthor
	}
	if role == "" {
		role = x.defaultRole
	}
	return proto.New(x.s, author, role)
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
	From        string `json:"from,omitempty" jsonschema:"position or argument that revealed this question (mutually exclusive with parent)"`
	Cardinality string `json:"cardinality,omitempty" jsonschema:"select_one (default) or open; fixed at creation"`
	Author      string `json:"author,omitempty"`
	Role        string `json:"role,omitempty"`
}

func (x *srv) raise(ctx context.Context, req *mcp.CallToolRequest, a raiseArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	switch a.Cardinality {
	case "", string(ibis.SelectOne), string(ibis.Open):
	default:
		return bad(&ibis.IllegalMove{Detail: "cardinality must be select_one or open, got " + a.Cardinality})
	}
	nid, err := x.mover(a.Author, a.Role).Raise(a.Discussion, a.Text, a.Parent, a.From, ibis.Cardinality(a.Cardinality))
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type reframeArgs struct {
	Discussion  string `json:"discussion" jsonschema:"discussion id"`
	Issue       string `json:"issue" jsonschema:"the issue whose framing is replaced"`
	Text        string `json:"text" jsonschema:"the new framing of the question"`
	Basis       string `json:"basis" jsonschema:"why the framing is replaced (required)"`
	Cardinality string `json:"cardinality,omitempty" jsonschema:"select_one (default) or open for the new issue"`
	Author      string `json:"author,omitempty"`
	Role        string `json:"role,omitempty"`
}

func (x *srv) reframe(ctx context.Context, req *mcp.CallToolRequest, a reframeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	switch a.Cardinality {
	case "", string(ibis.SelectOne), string(ibis.Open):
	default:
		return bad(&ibis.IllegalMove{Detail: "cardinality must be select_one or open, got " + a.Cardinality})
	}
	nid, err := x.mover(a.Author, a.Role).Reframe(a.Discussion, a.Issue, a.Text, a.Basis, ibis.Cardinality(a.Cardinality))
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type proposeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Text       string `json:"text" jsonschema:"the candidate answer"`
	Promotes   string `json:"promotes,omitempty" jsonschema:"the value this position promotes (audience lens input)"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) propose(ctx context.Context, req *mcp.CallToolRequest, a proposeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author, a.Role).Propose(a.Discussion, a.Issue, a.Text, a.Promotes)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type synthesizeArgs struct {
	Discussion string   `json:"discussion" jsonschema:"discussion id"`
	Issue      string   `json:"issue" jsonschema:"issue id"`
	Text       string   `json:"text" jsonschema:"the hybrid position"`
	From       []string `json:"from" jsonschema:"two or more parent position ids on the same issue"`
	Drops      []string `json:"drops,omitempty" jsonschema:"what the hybrid excludes from its parents (ideally one per parent) — a synthesis that drops nothing is a bundle"`
	Promotes   string   `json:"promotes,omitempty" jsonschema:"the value the hybrid promotes"`
	Author     string   `json:"author,omitempty"`
	Role       string   `json:"role,omitempty"`
}

func (x *srv) synthesize(ctx context.Context, req *mcp.CallToolRequest, a synthesizeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, warnings, err := x.mover(a.Author, a.Role).Synthesize(a.Discussion, a.Issue, a.Text, a.From, a.Promotes, a.Drops)
	if err != nil {
		return bad(err)
	}
	return ok(moveResult(nid, warnings))
}

type attachArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Target     string `json:"target" jsonschema:"position or argument id"`
	Text       string `json:"text" jsonschema:"the claim"`
	Promotes   string `json:"promotes,omitempty" jsonschema:"the value this argument promotes (audience lens input)"`
	Answers    string `json:"answers,omitempty" jsonschema:"parent objection this move answers on a synthesis (records an evaluator-inert addresses link)"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) support(ctx context.Context, req *mcp.CallToolRequest, a attachArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author, a.Role).Support(a.Discussion, a.Target, a.Text, a.Promotes, a.Answers)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

func (x *srv) object(ctx context.Context, req *mcp.CallToolRequest, a attachArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author, a.Role).Object(a.Discussion, a.Target, a.Text, a.Promotes, a.Answers)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

// moveResult shapes a move's JSON result, carrying advisory warnings when the
// engine has any (same envelope the CLI prints).
func moveResult(id string, warnings []string) map[string]any {
	v := map[string]any{"id": id}
	if len(warnings) > 0 {
		v["warnings"] = warnings
	}
	return v
}

func (x *srv) assume(ctx context.Context, req *mcp.CallToolRequest, a attachArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	nid, err := x.mover(a.Author, a.Role).Assume(a.Discussion, a.Target, a.Text)
	if err != nil {
		return bad(err)
	}
	return ok(map[string]string{"id": nid})
}

type promoteArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Node       string `json:"node" jsonschema:"id of one of your own positions/arguments"`
	Value      string `json:"value" jsonschema:"the value it promotes (throughput, security, …)"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) promote(ctx context.Context, req *mcp.CallToolRequest, a promoteArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author, a.Role).Promote(a.Discussion, a.Node, a.Value); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"node": a.Node, "value": a.Value})
}

type audienceArgs struct {
	Discussion string   `json:"discussion" jsonschema:"discussion id"`
	Name       string   `json:"name" jsonschema:"audience name (ops, product, …)"`
	Ranking    []string `json:"ranking" jsonschema:"values, most important first (strict order, at least two)"`
	Supersede  bool     `json:"supersede,omitempty" jsonschema:"replace an existing declaration of this name (requires basis)"`
	Basis      string   `json:"basis,omitempty" jsonschema:"why the prior ranking is retired (required with supersede)"`
	Author     string   `json:"author,omitempty"`
	Role       string   `json:"role,omitempty"`
}

func (x *srv) audience(ctx context.Context, req *mcp.CallToolRequest, a audienceArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author, a.Role).DeclareAudience(a.Discussion, a.Name, a.Ranking, a.Supersede, a.Basis); err != nil {
		return bad(err)
	}
	return ok(map[string]any{"name": a.Name, "ranking": a.Ranking})
}

type preferArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Winner     string `json:"winner" jsonschema:"preferred node id"`
	Loser      string `json:"loser" jsonschema:"node id it beats"`
	Basis      string `json:"basis,omitempty" jsonschema:"why (security, velocity, throughput, …)"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) prefer(ctx context.Context, req *mcp.CallToolRequest, a preferArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	pid, warnings, err := x.mover(a.Author, a.Role).Prefer(a.Discussion, a.Winner, a.Loser, a.Basis)
	if err != nil {
		return bad(err)
	}
	return ok(moveResult(pid, warnings))
}

type decideArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Position   string `json:"position" jsonschema:"accepted position id"`
	Basis      string `json:"basis,omitempty" jsonschema:"why"`
	ReviewBy   int64  `json:"review_by,omitempty" jsonschema:"unix seconds; re-examination horizon check will enforce"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) decide(ctx context.Context, req *mcp.CallToolRequest, a decideArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author, a.Role).Decide(a.Discussion, a.Issue, a.Position, a.Basis, a.ReviewBy); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"issue": a.Issue, "position": a.Position})
}

type supersedeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Position   string `json:"position" jsonschema:"newly accepted position id"`
	Basis      string `json:"basis" jsonschema:"why the prior decision is overturned (required)"`
	ReviewBy   int64  `json:"review_by,omitempty" jsonschema:"unix seconds; re-examination horizon check will enforce"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) supersede(ctx context.Context, req *mcp.CallToolRequest, a supersedeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author, a.Role).Supersede(a.Discussion, a.Issue, a.Position, a.Basis, a.ReviewBy); err != nil {
		return bad(err)
	}
	return ok(map[string]string{"issue": a.Issue, "position": a.Position})
}

type concedeArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Node       string `json:"node" jsonschema:"id of one of your own nodes"`
	Author     string `json:"author,omitempty"`
	Role       string `json:"role,omitempty"`
}

func (x *srv) concede(ctx context.Context, req *mcp.CallToolRequest, a concedeArgs) (*mcp.CallToolResult, any, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	if err := x.mover(a.Author, a.Role).Concede(a.Discussion, a.Node); err != nil {
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

func (x *srv) roster(ctx context.Context, req *mcp.CallToolRequest, a discArgs) (*mcp.CallToolResult, any, error) {
	rs, err := x.s.Rosters(a.Discussion, store.Now())
	if err != nil {
		return bad(err)
	}
	return ok(map[string]any{"discussion": a.Discussion, "bindings": rs})
}

type statusArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue,omitempty" jsonschema:"issue id (all issues if omitted)"`
	Under      string `json:"under,omitempty" jsonschema:"audience name: evaluate under that value ranking"`
}

func (x *srv) status(ctx context.Context, req *mcp.CallToolRequest, a statusArgs) (*mcp.CallToolResult, any, error) {
	g, fw, labels, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if a.Under != "" {
		fw, labels, err = underAudience(g, a.Under)
		if err != nil {
			return bad(err)
		}
	}
	var issues []string
	if a.Issue != "" {
		if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
			return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
		}
		issues = []string{a.Issue}
	} else {
		// The all-issues sweep skips reframed (dead) framings.
		for _, iss := range g.Issues() {
			if _, reframed := g.ReframedTo(iss); !reframed {
				issues = append(issues, iss)
			}
		}
	}
	var views []render.IssueStatus
	for _, iss := range issues {
		st := render.Status(g, fw, labels, iss, decs)
		st.Under = a.Under
		views = append(views, st)
	}
	return ok(map[string]any{"issues": views})
}

// underAudience rebuilds the framework and labelling under a named audience.
func underAudience(g *ibis.Graph, name string) (*af.Framework, map[string]af.Label, error) {
	aud, found := g.Audiences[name]
	if !found {
		return nil, nil, fail.NotFound(name, "audience %q not declared", name)
	}
	fw, err := af.BuildUnder(g, aud)
	if err != nil {
		return nil, nil, err
	}
	return fw, fw.Grounded(), nil
}

type discArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
}

func (x *srv) agenda(ctx context.Context, req *mcp.CallToolRequest, a discArgs) (*mcp.CallToolResult, any, error) {
	g, fw, labels, decs, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	return ok(render.Agenda(g, fw, labels, decs))
}

type whatifArgs struct {
	Discussion string   `json:"discussion" jsonschema:"discussion id"`
	Issue      string   `json:"issue" jsonschema:"issue id"`
	Object     []string `json:"object,omitempty" jsonschema:"targets to hypothetically object to (each gets an undefeated synthetic attacker)"`
	Prefer     []string `json:"prefer,omitempty" jsonschema:"hypothetical preferences as winner:loser pairs"`
	Without    []string `json:"without,omitempty" jsonschema:"nodes to hypothetically remove (simulated concede)"`
}

func (x *srv) whatif(ctx context.Context, req *mcp.CallToolRequest, a whatifArgs) (*mcp.CallToolResult, any, error) {
	g, _, labels, _, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
		return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
	}
	hyps, err := parseHyps(a.Object, a.Prefer, a.Without)
	if err != nil {
		return bad(err)
	}
	v, err := render.WhatIf(g, labels, a.Issue, hyps)
	if err != nil {
		return bad(err)
	}
	return ok(v)
}

// parseHyps assembles hypotheticals from the flag-shaped inputs the CLI and
// MCP share ("winner:loser" preference pairs).
func parseHyps(objects, prefers, withouts []string) ([]render.Hypothetical, error) {
	var hyps []render.Hypothetical
	for _, t := range objects {
		hyps = append(hyps, render.HypObject(t))
	}
	for _, p := range prefers {
		w, l, found := strings.Cut(p, ":")
		if !found || w == "" || l == "" {
			return nil, &ibis.IllegalMove{Detail: fmt.Sprintf("whatif prefer %q must be winner:loser", p)}
		}
		hyps = append(hyps, render.HypPrefer(w, l))
	}
	for _, n := range withouts {
		hyps = append(hyps, render.HypWithout(n))
	}
	if len(hyps) == 0 {
		return nil, &ibis.IllegalMove{Detail: "whatif needs at least one hypothetical (object / prefer / without)"}
	}
	return hyps, nil
}

func (x *srv) crux(ctx context.Context, req *mcp.CallToolRequest, a issueArgs) (*mcp.CallToolResult, any, error) {
	g, _, labels, _, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
		return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
	}
	v, err := render.Crux(g, labels, a.Issue)
	if err != nil {
		return bad(err)
	}
	return ok(v)
}

type worldsArgs struct {
	Discussion string `json:"discussion" jsonschema:"discussion id"`
	Issue      string `json:"issue" jsonschema:"issue id"`
	Under      string `json:"under,omitempty" jsonschema:"audience name: enumerate under that value ranking"`
}

func (x *srv) worlds(ctx context.Context, req *mcp.CallToolRequest, a worldsArgs) (*mcp.CallToolResult, any, error) {
	g, fw, _, _, err := x.framework(a.Discussion)
	if err != nil {
		return bad(err)
	}
	if n, found := g.Nodes[a.Issue]; !found || n.Kind != ibis.Issue {
		return bad(fail.NotFound(a.Issue, "issue %q not found", a.Issue))
	}
	if a.Under != "" {
		fw, _, err = underAudience(g, a.Under)
		if err != nil {
			return bad(err)
		}
	}
	v := render.Worlds(g, fw, a.Issue)
	v.Under = a.Under
	return ok(v)
}

func (x *srv) audiences(ctx context.Context, req *mcp.CallToolRequest, a discArgs) (*mcp.CallToolResult, any, error) {
	g, err := x.s.Graph(a.Discussion, store.Now())
	if err != nil {
		return bad(err)
	}
	v, err := render.Audiences(g)
	if err != nil {
		return bad(err)
	}
	return ok(v)
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
	v, err := check.Run(x.s, discs, store.Now(), time.Now().Unix())
	if err != nil {
		return bad(err)
	}
	return ok(v)
}
