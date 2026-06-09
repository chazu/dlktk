// Package discover emits dlktk's machine-readable capability contract (design
// §8.2): the move/read vocabulary an agent needs to drive the tool cold. CUE by
// default (matching the AGENTS.md convention), JSON on request.
package discover

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Version of the dlktk contract.
const Version = "0.2.0"

// Move describes a state-mutating command.
type Move struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	Legality string   `json:"legality"`
	Mutates  bool     `json:"mutates"`
}

// Read describes a non-mutating command.
type Read struct {
	Name    string   `json:"name"`
	Args    []string `json:"args"`
	Mutates bool     `json:"mutates"`
}

// Schema is the full capability contract.
type Schema struct {
	Tool    string   `json:"tool"`
	Version string   `json:"version"`
	IDs     string   `json:"ids"`
	Kinds   []string `json:"kinds"`
	Rels    []string `json:"rels"`
	Labels  []string `json:"labels"`
	Moves   []Move   `json:"moves"`
	Reads   []Read   `json:"reads"`
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
		Moves: []Move{
			{"raise", []string{"text", "[--parent issue]"}, "parent must be an issue", true},
			{"propose", []string{"issue", "text"}, "target must be an issue", true},
			{"support", []string{"target", "text"}, "target in {position,argument}", true},
			{"object", []string{"target", "text"}, "target in {position,argument}", true},
			{"prefer", []string{"winner", "loser", "--basis label"}, "AF nodes; no preference cycle", true},
			{"decide", []string{"issue", "position", "[--basis label]"}, "position responds_to issue", true},
			{"concede", []string{"node"}, "author owns the node", true},
			{"retract", []string{"node"}, "author owns the node", true},
		},
		Reads: []Read{
			{"status", []string{"[issue]"}, false},
			{"agenda", nil, false},
			{"moves", []string{"issue"}, false},
			{"why", []string{"node"}, false},
			{"explain", []string{"issue"}, false},
			{"tree", []string{"[issue]"}, false},
			{"list", nil, false},
			{"discover", nil, false},
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
	b.WriteString("\tmoves: [\n")
	for _, m := range s.Moves {
		fmt.Fprintf(&b, "\t\t{name: %q, args: %s, legality: %q, mutates: true},\n", m.Name, cueList(m.Args), m.Legality)
	}
	b.WriteString("\t]\n")
	b.WriteString("\treads: [\n")
	for _, r := range s.Reads {
		fmt.Fprintf(&b, "\t\t{name: %q, args: %s, mutates: false},\n", r.Name, cueList(r.Args))
	}
	b.WriteString("\t]\n")
	b.WriteString("}\n")
	return b.String()
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
	disc:     string
	issue:    string
	position: string
	basis:    string
	decider:  string
	override: bool
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
