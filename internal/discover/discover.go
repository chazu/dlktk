// Package discover emits dlktk's machine-readable capability contract (design
// §8.2): the move/read vocabulary an agent needs to drive the tool cold. CUE by
// default (matching the AGENTS.md convention), JSON on request.
package discover

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Version of the dlktk contract. 0.17.0 is the arc-two repairs
// (wicked-problems-2.md item 10): roster reports one row per distinct
// (author, role) binding rather than one per move, show renders reframe lineage
// from both sides (reframed_to on the dead framing, reframed_from on the new
// one) and a value-map decision, and the skill declares a minimum contract
// version it checks via `discover` at session start. 0.16.0 makes warnings
// obligate something
// (wicked-problems-2.md item 9): unacknowledged_warning fires when a decided
// issue closed over another live warning that its basis does not acknowledge
// (resolve it before the convergent move, or name the override rationale in
// --basis), and premature_preference fires when a preference was recorded
// before its issue carried two positions from two authors — the process rules
// both batches wrote as prose, now computed from authorship and transaction
// time. 0.15.0 adds the single_author_convergence
// strict finding (wicked-problems-2.md item 8): a decided synthesis whose
// scrutiny or decision never left the synthesis author — the decider shares its
// author, or every objection against it does — caught regardless of how many
// --author strings the orchestrator wore, because it tests the shape of the
// scrutiny, not the names on it. 0.14.0 adds value-map closure
// (wicked-problems-2.md item 7): `decide <issue> --map --review-by T` closes a
// value-driven issue with its audience-conditional map instead of a single
// winner — legal only when the issue is audience-sensitive right now (>=2
// declared audiences, >=1 position whose verdict differs across them) and only
// with a mandatory review horizon. Verdicts are not snapshotted; check derives
// the decision-time map bitemporally and reports map_drift when the current map
// differs, review_due when the horizon passes, and the non-fatal note
// mapped_pending_governance until the deferred "whose ranking governs?"
// question is raised as its own issue. supersede --map / supersede to a
// position convert between the two kinds, recording the superseded kind. Bare
// YYYY-MM-DD review horizons are accepted. 0.13.0 opens the closure story for
// open-cardinality issues (wicked-problems-2.md item 6): decide records a
// standing decision per position (the winners compose, so a repeat decide is
// rejected only on the same position), supersede targets the decision on a
// given position, IssueStatus carries a decisions[] list, and the agenda's
// ready section lists each undecided-but-justified position on an open issue.
// 0.12.0 is the synthesis-discipline batch
// (wicked-problems-2.md items 2-4): the evaluator-inert `addresses` relation
// with --answers on object/support (discharging a synthesis's inherited
// questions), inherited_questions in NodeView/WhyView and the composite
// stress-test moves suggestion, --drops on synthesize with drops in
// NodeView, advisory warnings on synthesize/prefer move results
// ({id, warnings?}), and the self_elevated_synthesis / bundle_synthesis
// strict findings. 0.11.0 hardens tested-ness (wicked-problems-2.md
// item 1): everywhere the system asks "was this position examined?" — the
// untested flag in IssueStatus, the agenda's untested section, the moves
// stress-test suggestion, the untested_decision check finding — a position now
// counts as tested only with a substantive objection: authored by someone
// other than the position's author and participating in the defeat relation.
// select_one rival edges and preference-neutralized objections never count,
// and the agenda surfaces untested positions only when decide-adjacent (the
// issue is ready). 0.10.0 is the wicked-problems batch (see
// wicked-problems.md): the reframe/synthesize/assume/promote/audience moves,
// raise --from, --promotes on propose/object/support, --review-by on
// decide/supersede, the whatif/crux/worlds/audiences reads, --under on
// status/explain/worlds, untested/reframed_to in IssueStatus, the
// untested/assumptions agenda sections, and the untested_decision/review_due/
// defeated_assumption check findings. 0.9.0 documents the --color global and
// the --format auto-detect (text on a terminal, json when piped). 0.8.0 splits
// --author (ownership identity) from
// --role (persona), adds the dlktk/roster binding with auto-record on every
// move under a role plus the roster move/read, and makes concede/retract
// ownership ride on the author identity (design §16 Q6/Q8). 0.7.0 adds the
// search read. 0.6.0 added the
// show read, node text in the why envelope, decide suggestions in moves, and
// the ready/unpopulated agenda sections. 0.5.0 added the check read (decision
// drift / stalemate / store-invariant verification, exit 5); 0.4.0 the
// supersede move (bare re-decide rejected, design §16 Q4) and
// decided.supersedes.
const Version = "0.17.0"

// Move describes a state-mutating command.
type Move struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	Legality string   `json:"legality"`
	Mutates  bool     `json:"mutates"`
}

// Read describes a non-mutating command. Output names the JSON envelope returned
// under --format json (see Schema.Envelopes); "text" means human-only output.
type Read struct {
	Name    string   `json:"name"`
	Args    []string `json:"args"`
	Output  string   `json:"output"`
	Mutates bool     `json:"mutates"`
}

// Flag is a global flag accepted by every command.
type Flag struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// ErrCode maps an exit code to its error kind and meaning. The error envelope
// (ErrorEnvelope) carries the matching `error` (kind) string.
type ErrCode struct {
	Code    int    `json:"code"`
	Kind    string `json:"kind"`
	Meaning string `json:"meaning"`
}

// Schema is the full capability contract.
type Schema struct {
	Tool          string            `json:"tool"`
	Version       string            `json:"version"`
	IDs           string            `json:"ids"`
	Kinds         []string          `json:"kinds"`
	Rels          []string          `json:"rels"`
	Labels        []string          `json:"labels"`
	Globals       []Flag            `json:"globals"`
	Moves         []Move            `json:"moves"`
	Reads         []Read            `json:"reads"`
	Errors        []ErrCode         `json:"errors"`
	ErrorEnvelope string            `json:"error_envelope"`
	Envelopes     map[string]string `json:"envelopes"`
}

// Current returns the capability schema for this build.
func Current() Schema {
	return Schema{
		Tool:    "dlktk",
		Version: Version,
		IDs:     "proquint",
		Kinds:   []string{"issue", "position", "argument"},
		Rels:    []string{"responds_to", "supports", "objects_to", "synthesizes", "raised_from", "addresses"},
		Labels:  []string{"IN", "OUT", "UNDEC"},
		Globals: []Flag{
			{"--format text|json", "output format; default auto: text on a terminal, json when piped (json gives the envelopes below)"},
			{"--color auto|always|never", "colorize text output (auto = on for a terminal, off when piped or NO_COLOR is set)"},
			{"--discussion|-d id", "target discussion (else $DLKTK_DISC, else ./.dlktk/current)"},
			{"--author name", "stable identity attributed to moves and checked for ownership (default: OS user)"},
			{"--role name", "persona a move is made under; auto-records an author↔role roster binding (metadata only)"},
			{"--as-of T", "transaction-time travel: evaluate as of T (RFC3339 or Unix seconds)"},
			{"--valid-at T", "valid-time: which decisions were in force at T"},
			{"--store dir", "pudl store dir (default: repo .pudl/ else ~/.pudl)"},
		},
		Moves: []Move{
			{"raise", []string{"text", "[--parent issue]", "[--from node]", "[--card select_one|open]"}, "parent must be an issue; from must be a position/argument (mutually exclusive with parent); cardinality fixed at creation, default select_one", true},
			{"reframe", []string{"issue", "text", "--basis label", "[--card select_one|open]"}, "issue exists, not already reframed, and has no standing decision (supersede it first); basis required; positions do not carry over, lineage recorded", true},
			{"propose", []string{"issue", "text", "[--promotes value]"}, "target must be an issue", true},
			{"synthesize", []string{"issue", "text", "--from position", "--from position", "[--drops text]...", "[--promotes value]"}, "at least two distinct parent positions, each responding to the issue; the hybrid joins the rivalry until parents are conceded or a preference/audience elevates it; a synthesis that drops nothing is a bundle — result warns at >=3 parents with no --drops", true},
			{"support", []string{"target", "text", "[--promotes value]", "[--answers objection-id]"}, "target in {position,argument}; --answers dismisses a parent objection on a synthesis target (inert addresses link)", true},
			{"object", []string{"target", "text", "[--promotes value]", "[--answers objection-id]"}, "target in {position,argument}; --answers re-aims a parent objection at a synthesis target (inert addresses link)", true},
			{"assume", []string{"target", "text"}, "target in {position,argument}; records a challengeable premise (supports link, tag=assumption; inert in the labelling)", true},
			{"prefer", []string{"winner", "loser", "--basis label"}, "AF nodes; no preference cycle; result warns (self-elevated synthesis) when the winner subsumes the loser without answering its objections", true},
			{"promote", []string{"node", "value"}, "AF node owned by the author; one value per node (to change, concede and restate)", true},
			{"audience", []string{"name", "value...", "[--supersede --basis label]"}, "at least two distinct values, most important first; re-declaring a name requires --supersede with a basis", true},
			{"decide", []string{"issue", "position", "[--basis label]", "[--review-by T]", "[--map]"}, "position responds_to issue; select_one: issue not already decided (overturn via supersede); open: one standing decision per position — the winners compose — so a repeat decide is rejected only on the same position; --map (no position) closes the issue as its audience-conditional map — legal only with >=2 declared audiences and >=1 position whose verdict differs across them, --review-by then mandatory; review-by must be in the future", true},
			{"supersede", []string{"issue", "position", "--basis label", "[--review-by T]", "[--map]"}, "basis required; select_one: replaces the issue's single decision; open: revises the standing decision on <position> (siblings stand); --map (no position) converts the issue to a value-map (same map preconditions, --review-by mandatory); new decision records the superseded decision's kind", true},
			{"concede", []string{"node"}, "author (identity, not persona) owns the node", true},
			{"retract", []string{"node"}, "author (identity, not persona) owns the node", true},
			{"roster", []string{"[author]", "[role]"}, "no args lists bindings; author+role pre-declares one (moves auto-record otherwise)", true},
		},
		Reads: []Read{
			{"status", []string{"[issue]", "[--under audience]"}, "[IssueStatus]", false},
			{"agenda", nil, "AgendaView", false},
			{"moves", []string{"issue"}, "MovesView", false},
			{"show", []string{"node"}, "NodeView", false},
			{"search", []string{"query", "[--all]"}, "SearchView", false},
			{"why", []string{"node"}, "WhyView", false},
			{"explain", []string{"issue", "[--under audience]"}, "ExplainView", false},
			{"whatif", []string{"issue", "[--object node]...", "[--prefer winner:loser]...", "[--without node]..."}, "WhatIfView", false},
			{"crux", []string{"issue"}, "CruxView", false},
			{"worlds", []string{"issue", "[--under audience]"}, "WorldsView", false},
			{"audiences", nil, "AudiencesView", false},
			{"audience", nil, "[Audience]", false},
			{"tree", []string{"[issue]"}, "text", false},
			{"list", nil, "[Discussion]", false},
			{"roster", nil, "RosterView", false},
			{"check", []string{"[--all]", "[--strict]"}, "CheckView", false},
			{"discover", nil, "Schema (this document)", false},
		},
		Errors: []ErrCode{
			{1, "generic", "unspecified failure"},
			{2, "illegal_move", "ill-formed or illegal move; nothing was written"},
			{3, "not_found", "a referenced discussion/issue/node id does not exist"},
			{4, "store_error", "storage or engine failure"},
			{5, "check_failed", "check found decision drift or invariant violations (warnings too, under --strict)"},
		},
		ErrorEnvelope: "{error: kind, detail: string, node?: id}",
		Envelopes: map[string]string{
			"IssueStatus":       "{issue, issue_text, cardinality, under?, positions: [{id, text, label, attacked_by: [id], defeated_by: [id], reinstated: bool, untested?: bool}], undecided: [id], stalemate: bool, advice, reframed_to?, decided?: {position, basis, decider, override, supersedes?, review_by?} (select_one), decisions?: [{position, basis, decider, override, supersedes?, review_by?}] (all standing decisions; open issues record one per position), map_decided?: {basis, decider, review_by, kind: map, superseded_kind?} (issue closed as a value-map, no single winner)}",
			"AgendaView":        "{undecided: [{id, kind, text, label}], ready: [{issue, text, position, position_text}], unpopulated: [{issue, text}], untested: [{issue, text, position, position_text}], assumptions: [{id, kind, text, label}]}",
			"MovesView":         "{issue, moves: [{move, args: [string], effect}]}",
			"WhyView":           "{node, text, label, because: [{attacker, attacker_text, attacker_label, reason}], inherited_questions?: [InheritedQuestion], to_flip: [{move, args: [string], effect}]}",
			"InheritedQuestion": "{objection, objection_text, author?, parent, parent_text, addressed_by?: [id], open: bool} — a synthesis parent's undefeated objection; discharge with object/support --answers",
			"MoveResult":        "{id, warnings?: [string]} — synthesize and prefer may carry advisory warnings; the move stands",
			"NodeView":          "{id, kind, text, author?, tag?, promotes?, label?, drops?: [string], inherited_questions?: [InheritedQuestion], links: [{rel, dir: in|out, peer, peer_kind, peer_text, peer_label?}], decided?: {position, basis, decider, override, supersedes?, review_by?}}",
			"SearchView":        "{query, hits: [{discussion, id, kind, text, label?}]}",
			"ExplainView":       "{issue, issue_text, cardinality, attacks: [{from, to, source, defeats, basis?, audience_blocked?}], preferences: [{winner, loser, basis?, derived}], steps: [{round, node, label, why, by: [id]}], outcome: [{id, text, label}], decided?: {position, basis, decider, override, supersedes?, review_by?}, decision_is_in: bool}",
			"WhatIfView":        "{issue, hypotheticals: [{kind: object|prefer|without, target?, winner?, loser?, node?, summary}], flipped: [{node, text, from, to}], result: IssueStatus}",
			"CruxView":          "{issue, cruxes: [{node, text, author?, flips: [{node, text, from, to}]}], note}",
			"WorldsView":        "{issue, under?, worlds: [{in: [{id, kind, text, label}], distinguishing: [id]}], robust: [ref], contingent: [ref], hopeless: [ref], too_contested?: bool, note?}",
			"AudiencesView":     "{audiences: [{name, ranking: [value], author?}], issues: [{issue, text, baseline: {position: label}, by_audience: {name: {position: label}}, robust: [id], sensitive: [{position, text, verdicts: {audience|baseline: label}}]}]}",
			"Audience":          "{disc, name, ranking: [value], basis?, author}",
			"CheckView":         "{discussions, findings: [{kind: decision_drift|preference_cycle|store_invariant|stalemate|untested_decision|review_due|defeated_assumption|self_elevated_synthesis|bundle_synthesis|map_drift|mapped_pending_governance|single_author_convergence|premature_preference|unacknowledged_warning, severity: error|warning|note, discussion, issue?, node?, detail}], ok: bool}",
			"Discussion":        "{id, title, subject, created_by}",
			"RosterView":        "{discussion, bindings: [{disc, author, role}]}",
		},
	}
}

// JSON renders the schema as indented JSON.
func JSON() (string, error) {
	b, err := json.MarshalIndent(Current(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CUE renders the schema as a CUE document matching the #Dlktk shape (§8.2).
func CUE() string {
	s := Current()
	var b strings.Builder
	b.WriteString("#Dlktk: {\n")
	fmt.Fprintf(&b, "\ttool:    %q\n", s.Tool)
	fmt.Fprintf(&b, "\tversion: %q\n", s.Version)
	fmt.Fprintf(&b, "\tids:     %q\n", s.IDs)
	fmt.Fprintf(&b, "\tkinds:   %s\n", cueList(s.Kinds))
	fmt.Fprintf(&b, "\trels:    %s\n", cueList(s.Rels))
	fmt.Fprintf(&b, "\tlabels:  %s\n", cueList(s.Labels))
	b.WriteString("\tglobals: [\n")
	for _, f := range s.Globals {
		fmt.Fprintf(&b, "\t\t{name: %q, desc: %q},\n", f.Name, f.Desc)
	}
	b.WriteString("\t]\n")
	b.WriteString("\tmoves: [\n")
	for _, m := range s.Moves {
		fmt.Fprintf(&b, "\t\t{name: %q, args: %s, legality: %q, mutates: true},\n", m.Name, cueList(m.Args), m.Legality)
	}
	b.WriteString("\t]\n")
	b.WriteString("\treads: [\n")
	for _, r := range s.Reads {
		fmt.Fprintf(&b, "\t\t{name: %q, args: %s, output: %q, mutates: false},\n", r.Name, cueList(r.Args), r.Output)
	}
	b.WriteString("\t]\n")
	b.WriteString("\terrors: [\n")
	for _, e := range s.Errors {
		fmt.Fprintf(&b, "\t\t{code: %d, kind: %q, meaning: %q},\n", e.Code, e.Kind, e.Meaning)
	}
	b.WriteString("\t]\n")
	fmt.Fprintf(&b, "\terror_envelope: %q\n", s.ErrorEnvelope)
	b.WriteString("\tenvelopes: {\n")
	for _, name := range sortedKeys(s.Envelopes) {
		fmt.Fprintf(&b, "\t\t%q: %q\n", name, s.Envelopes[name])
	}
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// CUESchema renders the `pudl/dlktk` CUE package: the typed args shape of every
// dlktk/* relation, so pudl and other tools can validate/interpret dlktk facts
// (design §3.7). pudl >= v0.1.3 ships this package as a built-in bootstrap
// schema (registered automatically); on older pudl versions, install the output
// under the pudl schema dir to register it. Keep this copy and pudl's
// internal/importer/bootstrap/pudl/dlktk/dlktk.cue in lockstep.
func CUESchema() string {
	return `package dlktk

// Args schemas for the dlktk/* fact relations. The fact's relation name selects
// the schema; the fact's args object must unify with it.

#Discussion: {
	id:         string
	title:      string
	subject:    string // file:… | pkg:… | commit:… | q:…
	created_by: string
}

#Node: {
	id:     string
	disc:   string
	kind:   "issue" | "position" | "argument"
	text:   string
	author: string
	tag?:   "assumption" // a challengeable premise; bookkeeping only, never reaches the evaluator
	drops?: [...string] // syntheses: what the hybrid explicitly excludes from its parents (metadata)
}

#Link: {
	id:     string
	disc:   string
	src:    string
	dst:    string
	rel:    "responds_to" | "supports" | "objects_to" | "synthesizes" | "raised_from" | "addresses"
	author: string
}

#IssueCard: {
	issue:       string
	cardinality: "select_one" | "open"
}

#Preference: {
	id:     string
	disc:   string
	winner: string
	loser:  string
	basis:  string
	author: string
}

#Decision: {
	disc:        string
	issue:       string
	position:    string
	basis:       string
	decider:     string
	override:    bool
	supersedes?: string // prior decided position, when made via supersede
	review_by?:  int    // unix seconds; re-examination horizon check enforces
}

#Roster: {
	disc:   string
	author: string // stable ownership identity
	role:   string // persona; metadata only, never reaches the evaluator
}

#Reframe: {
	disc:   string
	old:    string // the issue whose framing was replaced
	new:    string // the issue that replaced it
	basis:  string // why — mandatory, the Q4 force-capture ethos
	author: string
}

#Value: {
	disc:   string
	node:   string // the position/argument
	value:  string // the value it promotes (audience lens input)
	author: string
}

#Audience: {
	disc:    string
	name:    string
	ranking: [...string] // values, most important first (strict order)
	basis?:  string      // recorded when a supersession retires a prior ranking
	author:  string
}

// relation -> schema binding
#byRelation: {
	"dlktk/discussion": #Discussion
	"dlktk/node":       #Node
	"dlktk/link":       #Link
	"dlktk/issue_card": #IssueCard
	"dlktk/preference": #Preference
	"dlktk/decision":   #Decision
	"dlktk/roster":     #Roster
	"dlktk/reframe":    #Reframe
	"dlktk/value":      #Value
	"dlktk/audience":   #Audience
}
`
}

func cueList(xs []string) string {
	if len(xs) == 0 {
		return "[]"
	}
	q := make([]string, len(xs))
	for i, x := range xs {
		q[i] = fmt.Sprintf("%q", x)
	}
	return "[" + strings.Join(q, ", ") + "]"
}
