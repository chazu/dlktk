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

// Version of the dlktk contract. 0.5.0 adds the check read (decision drift /
// stalemate / store-invariant verification, exit 5) on top of 0.4.0's
// supersede move (bare re-decide rejected, design §16 Q4) and
// decided.supersedes envelope field.
const Version = "0.5.0"

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
		Rels:    []string{"responds_to", "supports", "objects_to"},
		Labels:  []string{"IN", "OUT", "UNDEC"},
		Globals: []Flag{
			{"--format text|json", "output format (json gives the envelopes below)"},
			{"--discussion|-d id", "target discussion (else $DLKTK_DISC, else ./.dlktk/current)"},
			{"--role name", "author/role attribution for moves (default: OS user)"},
			{"--as-of T", "transaction-time travel: evaluate as of T (RFC3339 or Unix seconds)"},
			{"--valid-at T", "valid-time: which decisions were in force at T"},
			{"--store dir", "pudl store dir (default: repo .pudl/ else ~/.pudl)"},
		},
		Moves: []Move{
			{"raise", []string{"text", "[--parent issue]", "[--card select_one|open]"}, "parent must be an issue; cardinality fixed at creation, default select_one", true},
			{"propose", []string{"issue", "text"}, "target must be an issue", true},
			{"support", []string{"target", "text"}, "target in {position,argument}", true},
			{"object", []string{"target", "text"}, "target in {position,argument}", true},
			{"prefer", []string{"winner", "loser", "--basis label"}, "AF nodes; no preference cycle", true},
			{"decide", []string{"issue", "position", "[--basis label]"}, "position responds_to issue; issue not already decided (overturning requires supersede)", true},
			{"supersede", []string{"issue", "position", "--basis label"}, "issue already decided; basis required; new decision links the prior position", true},
			{"concede", []string{"node"}, "author owns the node", true},
			{"retract", []string{"node"}, "author owns the node", true},
		},
		Reads: []Read{
			{"status", []string{"[issue]"}, "[IssueStatus]", false},
			{"agenda", nil, "AgendaView", false},
			{"moves", []string{"issue"}, "MovesView", false},
			{"why", []string{"node"}, "WhyView", false},
			{"explain", []string{"issue"}, "ExplainView", false},
			{"tree", []string{"[issue]"}, "text", false},
			{"list", nil, "[Discussion]", false},
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
			"IssueStatus": "{issue, issue_text, cardinality, positions: [{id, text, label, attacked_by: [id], defeated_by: [id], reinstated: bool}], undecided: [id], stalemate: bool, advice, decided?: {position, basis, decider, override, supersedes?}}",
			"AgendaView":  "{undecided: [{id, kind, text, label}]}",
			"MovesView":   "{issue, moves: [{move, args: [string], effect}]}",
			"WhyView":     "{node, label, because: [{attacker, attacker_label, reason}], to_flip: [{move, args: [string], effect}]}",
			"ExplainView": "{issue, issue_text, cardinality, attacks: [{from, to, source, defeats, basis?}], preferences: [{winner, loser, basis?, derived}], steps: [{round, node, label, why, by: [id]}], outcome: [{id, text, label}], decided?: {position, basis, decider, override, supersedes?}, decision_is_in: bool}",
			"CheckView":   "{discussions, findings: [{kind: decision_drift|preference_cycle|store_invariant|stalemate, severity: error|warning, discussion, issue?, node?, detail}], ok: bool}",
			"Discussion":  "{id, title, subject, created_by}",
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
// (design §3.7). Install it under the pudl schema dir to register the package.
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
}

#Link: {
	id:     string
	disc:   string
	src:    string
	dst:    string
	rel:    "responds_to" | "supports" | "objects_to"
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
}

// relation -> schema binding
#byRelation: {
	"dlktk/discussion": #Discussion
	"dlktk/node":       #Node
	"dlktk/link":       #Link
	"dlktk/issue_card": #IssueCard
	"dlktk/preference": #Preference
	"dlktk/decision":   #Decision
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
