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
