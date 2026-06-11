package fail

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chazu/dlktk/internal/af"
	"github.com/chazu/dlktk/internal/ibis"
)

// Classify must map error types to the exit codes the discover contract
// publishes; agent harnesses branch on these.
func TestClassifyExitCodes(t *testing.T) {
	cases := []struct {
		err      error
		code     int
		kind     string
		wantNode string
	}{
		{&ibis.IllegalMove{Detail: "d", Node: "n1"}, CodeIllegal, "illegal_move", "n1"},
		{&ibis.NotFound{Detail: "d", Node: "n2"}, CodeNotFound, "not_found", "n2"},
		{&af.PreferenceCycleError{Node: "n3"}, CodeStore, "store_error", "n3"},
		{Store("boom"), CodeStore, "store_error", ""},
		{NotFound("n4", "gone"), CodeNotFound, "not_found", "n4"},
		{errors.New("plain"), CodeGeneric, "error", ""},
		{fmt.Errorf("wrapped: %w", &ibis.NotFound{Detail: "d", Node: "n5"}), CodeNotFound, "not_found", "n5"},
	}
	for _, c := range cases {
		e := Classify(c.err)
		if e.Code() != c.code || e.ErrKind != c.kind || e.Node != c.wantNode {
			t.Errorf("Classify(%v) = code %d kind %q node %q; want %d %q %q",
				c.err, e.Code(), e.ErrKind, e.Node, c.code, c.kind, c.wantNode)
		}
	}
}
