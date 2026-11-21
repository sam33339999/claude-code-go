package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// scriptedClient replays one canned assistant turn per Stream call and
// records each request.
type scriptedClient struct {
	mu    sync.Mutex
	turns [][]*api.StreamEvent
	reqs  []*api.MessageRequest
}

func (c *scriptedClient) Stream(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = append(c.reqs, req)
	if len(c.turns) == 0 {
		return nil, errors.New("scriptedClient: no more turns")
	}
	t := c.turns[0]
	c.turns = c.turns[1:]
	return &sliceReader{events: t}, nil
}

func (c *scriptedClient) Complete(context.Context, *api.MessageRequest) (*api.MessageResponse, error) {
	return nil, errors.New("scriptedClient: Complete not supported")
}

type sliceReader struct {
	events []*api.StreamEvent
	pos    int
}

func (r *sliceReader) Next() (*api.StreamEvent, error) {
	if r.pos >= len(r.events) {
		return nil, io.EOF
	}
	r.pos++
	return r.events[r.pos-1], nil
}

func (r *sliceReader) Close() error { return nil }

func rawJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// sseTurn builds an assistant turn: one tool_use block (toolName != "") or
// one text block, ending with the matching stop reason.
func sseTurn(toolID, toolName, payload string) []*api.StreamEvent {
	stop := "end_turn"
	block := map[string]any{"type": "text", "text": ""}
	delta := api.Delta{Type: "text_delta", Text: payload}
	deltaRaw := map[string]any{"type": "text_delta", "text": payload}
	if toolName != "" {
		stop = "tool_use"
		block = map[string]any{"type": "tool_use", "id": toolID, "name": toolName}
		delta = api.Delta{Type: "input_json_delta", PartialJSON: payload}
		deltaRaw = map[string]any{"type": "input_json_delta", "partial_json": payload}
	}
	msgDelta := &api.StreamEvent{
		Type:         api.EventMessageDelta,
		Data:         rawJSON(map[string]any{"delta": map[string]any{"stop_reason": stop}, "usage": map[string]any{"output_tokens": 3}}),
		MessageDelta: &api.MessageDeltaData{},
	}
	msgDelta.MessageDelta.Delta.StopReason = stop
	msgDelta.MessageDelta.Usage = api.Usage{OutputTokens: 3}
	return []*api.StreamEvent{
		{
			Type:         api.EventMessageStart,
			Data:         rawJSON(map[string]any{"message": map[string]any{"id": "m", "type": "message", "role": "assistant", "content": []any{}, "model": "claude-test", "usage": map[string]any{"input_tokens": 10, "output_tokens": 0}}}),
			MessageStart: &api.MessageStartData{Message: api.MessageResponse{ID: "m", Type: "message", Role: "assistant", Model: "claude-test", Usage: api.Usage{InputTokens: 10}}},
		},
		{Type: api.EventContentBlockStart, Data: rawJSON(map[string]any{"index": 0, "content_block": block})},
		{Type: api.EventContentBlockDelta, Data: rawJSON(map[string]any{"index": 0, "delta": deltaRaw}), ContentBlockDelta: &api.ContentBlockDeltaData{Index: 0, Delta: delta}},
		{Type: api.EventContentBlockStop, Data: rawJSON(map[string]any{"index": 0})},
		msgDelta,
		{Type: api.EventMessageStop},
	}
}

// toolResultsOf returns the text of every tool_result block in the request.
func toolResultsOf(req *api.MessageRequest) []string {
	b, _ := json.Marshal(req.Messages)
	var msgs []struct {
		Content []struct {
			Type    string          `json:"type"`
			Content json.RawMessage `json:"content"`
		} `json:"content"`
	}
	_ = json.Unmarshal(b, &msgs)
	var out []string
	for _, m := range msgs {
		for _, c := range m.Content {
			if c.Type == "tool_result" {
				out = append(out, string(c.Content))
			}
		}
	}
	return out
}

// TestACPIntegration_AgentToolDrivesExternalAgent runs the real wiring end to
// end: settings acpAgents → BuildContainerWithClient → main loop calls the
// Agent tool with agent_type=<external> → coordinator → ACP subprocess (this
// test binary as a scripted agent over real stdio) → tool_result back to the
// main model. It also checks the shared session, settings permission rules
// applied to the external agent, and process shutdown on Close.
func TestACPIntegration_AgentToolDrivesExternalAgent(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	home, project := t.TempDir(), t.TempDir()
	writeSettings(t, filepath.Join(home, ".glaude", "settings.json"), map[string]any{
		"acpAgents": map[string]any{
			"fakeacp": map[string]any{"command": exe, "env": map[string]string{"GLAUDE_ACP_FAKE_AGENT": "1"}, "description": "scripted test agent"},
		},
		"permissions": map[string]any{"allow": []string{"Agent", "Bash(echo ok:*)"}},
	})
	// Memory stop hooks run forked agents that race on the shared
	// ToolUseContext (pre-existing engine issue, unrelated to ACP).
	t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "1")

	agentCall := func(prompt string) string {
		return string(rawJSON(map[string]any{"prompt": prompt, "agent_type": "fakeacp"}))
	}
	client := &scriptedClient{turns: [][]*api.StreamEvent{
		sseTurn("tu1", "Agent", agentCall("echo:hello")),
		sseTurn("tu2", "Agent", agentCall("echo:again")),
		sseTurn("tu3", "Agent", agentCall("perm:echo ok now")),
		sseTurn("tu4", "Agent", agentCall("perm:make deploy")),
		sseTurn("", "", "all done"),
	}}
	c, err := BuildContainerWithClient(ContainerOptions{HomeDir: home, WorkingDir: project, Headless: true, Debug: os.Getenv("ACP_IT_DEBUG") != ""}, client)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if c.ACPAgents == nil || !c.ACPAgents.Has("fakeacp") {
		t.Fatalf("fakeacp not configured: %v", c.ACPAgents.Names())
	}
	agentTool, ok := c.ToolRegistry.Get("Agent")
	if !ok {
		t.Fatal("Agent tool not registered")
	}
	desc := agentTool.Description(nil, nil)
	if !strings.Contains(desc, `"fakeacp" — scripted test agent`) {
		t.Fatal("Agent tool description does not list the external agent")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	msgCh, err := c.QueryEngine.Query(ctx, engine.QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("go")}}}},
		ToolUseContext: &tools.UseContext{Ctx: ctx, Coordinator: c.AgentCoordinator, WorkingDir: project},
		QuerySource:    "foreground",
		MaxTurns:       10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	for m := range msgCh {
		if os.Getenv("ACP_IT_DEBUG") != "" {
			t.Logf("msg %v tool=%v err=%v", m.Type, m.ToolName, m.Err)
		}
		if m.Type == engine.MsgTypeError {
			t.Fatalf("engine error: %v", m.Err)
		}
	}

	client.mu.Lock()
	reqs := client.reqs
	client.mu.Unlock()
	if len(reqs) != 5 {
		t.Fatalf("model calls = %d, want 5", len(reqs))
	}
	results := toolResultsOf(reqs[4])
	want := []string{"turn 1: hello", "turn 2: again", "allowed", "denied"}
	if len(results) != len(want) {
		t.Fatalf("tool results = %v", results)
	}
	for i, w := range want {
		if !strings.Contains(results[i], w) {
			t.Errorf("tool result %d = %s, want it to contain %q", i, results[i], w)
		}
	}

	// Close terminates the agent process; a later prompt is refused.
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ACPAgents.Prompt(ctx, "fakeacp", project, "echo:x", nil); err == nil {
		t.Fatal("prompt after Close should fail")
	}
}

func writeSettings(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, rawJSON(v), 0o644); err != nil {
		t.Fatal(err)
	}
}
