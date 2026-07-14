package discover

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// TestCUESchemaCompiles guards that the published `dlktk schema` package is
// valid CUE — a stray edit to the raw string would otherwise only surface when
// pudl tried to load it.
func TestCUESchemaCompiles(t *testing.T) {
	if err := cuecontext.New().CompileString(CUESchema()).Err(); err != nil {
		t.Fatalf("CUESchema does not compile: %v", err)
	}
}

// TestCUESchemaValidatesDecisionFacts round-trips the exact args a decision
// fact carries through the published #Decision definition, selected the way
// pudl selects it (via #byRelation). The map-decision cases are the regression
// guard: item 7 writes kind:"map" / superseded_kind, and a #Decision that omits
// those closed-def fields rejects a valid value-map decision.
func TestCUESchemaValidatesDecisionFacts(t *testing.T) {
	ctx := cuecontext.New()
	schema := ctx.CompileString(CUESchema())
	if err := schema.Err(); err != nil {
		t.Fatalf("schema does not compile: %v", err)
	}

	decl := schema.
		LookupPath(cue.ParsePath("#byRelation")).
		LookupPath(cue.MakePath(cue.Str("dlktk/decision")))
	if err := decl.Err(); err != nil {
		t.Fatalf(`#byRelation["dlktk/decision"] missing: %v`, err)
	}

	cases := []struct {
		name string
		args map[string]any
		ok   bool
	}{
		{
			name: "conventional position decision",
			args: map[string]any{
				"disc": "d", "issue": "i1", "position": "p1",
				"basis": "b", "decider": "alice", "override": false,
			},
			ok: true,
		},
		{
			name: "map decision (item 7)",
			args: map[string]any{
				"disc": "d", "issue": "i1", "position": "",
				"basis": "values", "decider": "alice", "override": false,
				"kind": "map", "review_by": 123,
			},
			ok: true,
		},
		{
			name: "position->map supersession records superseded_kind",
			args: map[string]any{
				"disc": "d", "issue": "i1", "position": "",
				"basis": "values", "decider": "alice", "override": false,
				"kind": "map", "superseded_kind": "map", "review_by": 123,
			},
			ok: true,
		},
		{
			name: "unknown kind rejected",
			args: map[string]any{
				"disc": "d", "issue": "i1", "position": "",
				"basis": "b", "decider": "alice", "override": false,
				"kind": "nonsense",
			},
			ok: false,
		},
		{
			name: "unknown field rejected by closed def",
			args: map[string]any{
				"disc": "d", "issue": "i1", "position": "p1",
				"basis": "b", "decider": "alice", "override": false,
				"bogus": "x",
			},
			ok: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decl.Unify(ctx.Encode(tc.args)).Validate(cue.Concrete(true))
			if tc.ok && got != nil {
				t.Fatalf("expected args to validate, got error: %v", got)
			}
			if !tc.ok && got == nil {
				t.Fatalf("expected args to be rejected, but they validated")
			}
		})
	}
}
