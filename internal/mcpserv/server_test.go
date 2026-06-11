package mcpserv

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chazu/dlktk/internal/store"
)

// harness connects an in-memory MCP client to the dlktk server over a temp
// store and returns a call helper.
func harness(t *testing.T) func(tool string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	ctx := context.Background()
	serverT, clientT := mcp.NewInMemoryTransports()
	server := NewServer(s, "tester")
	ss, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })

	return func(tool string, args map[string]any) (map[string]any, bool) {
		t.Helper()
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatalf("%s: protocol error: %v", tool, err)
		}
		if len(res.Content) != 1 {
			t.Fatalf("%s: want 1 content item, got %d", tool, len(res.Content))
		}
		text := res.Content[0].(*mcp.TextContent).Text
		var v map[string]any
		if err := json.Unmarshal([]byte(text), &v); err != nil {
			t.Fatalf("%s: result is not a JSON envelope: %v\n%s", tool, err, text)
		}
		return v, res.IsError
	}
}

// Drive the full agent loop over MCP: new -> raise -> propose x2 -> object ->
// status -> prefer -> agenda (ready) -> decide -> re-decide rejected with the
// CLI's structured error envelope.
func TestDeliberationLoop(t *testing.T) {
	call := harness(t)

	v, _ := call("new", map[string]any{"title": "lock choice"})
	disc := v["id"].(string)
	v, _ = call("raise", map[string]any{"discussion": disc, "text": "which lock?"})
	issue := v["id"].(string)
	v, _ = call("propose", map[string]any{"discussion": disc, "issue": issue, "text": "mutex"})
	mutex := v["id"].(string)
	v, _ = call("propose", map[string]any{"discussion": disc, "issue": issue, "text": "rwlock"})
	rwlock := v["id"].(string)
	if _, isErr := call("object", map[string]any{"discussion": disc, "target": mutex, "text": "coarse lock serializes reads", "author": "carol"}); isErr {
		t.Fatal("object failed")
	}

	v, _ = call("status", map[string]any{"discussion": disc, "issue": issue})
	issues := v["issues"].([]any)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue status, got %d", len(issues))
	}

	// rwlock should be the unique IN position (mutex defeated by the objection).
	v, _ = call("agenda", map[string]any{"discussion": disc})
	ready, _ := v["ready"].([]any)
	if len(ready) != 1 || ready[0].(map[string]any)["position"] != rwlock {
		t.Fatalf("agenda not ready on rwlock: %v", v)
	}

	if _, isErr := call("decide", map[string]any{"discussion": disc, "issue": issue, "position": rwlock, "basis": "read-heavy"}); isErr {
		t.Fatal("decide failed")
	}

	// Bare re-decide must surface the structured illegal_move envelope.
	v, isErr := call("decide", map[string]any{"discussion": disc, "issue": issue, "position": mutex})
	if !isErr || v["error"] != "illegal_move" {
		t.Fatalf("re-decide: want illegal_move error envelope, got isErr=%v %v", isErr, v)
	}

	// search finds the objection text.
	v, _ = call("search", map[string]any{"query": "SERIALIZES", "discussion": disc})
	if hits := v["hits"].([]any); len(hits) != 1 {
		t.Fatalf("search: want 1 hit, got %v", v)
	}

	// check is clean (decision matches the justified position).
	v, _ = call("check", map[string]any{})
	if v["ok"] != true {
		t.Fatalf("check not ok: %v", v)
	}
}

func TestNotFoundEnvelope(t *testing.T) {
	call := harness(t)
	v, _ := call("new", map[string]any{"title": "t"})
	disc := v["id"].(string)
	v, isErr := call("show", map[string]any{"discussion": disc, "node": "zzzzz-zzzzz"})
	if !isErr || v["error"] != "not_found" {
		t.Fatalf("want not_found envelope, got isErr=%v %v", isErr, v)
	}
}
