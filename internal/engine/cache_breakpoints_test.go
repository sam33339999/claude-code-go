package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// ─────────────────────────────────────────────────────────────────────────────
// Cache breakpoint placement (cache-first guard tests)
// ─────────────────────────────────────────────────────────────────────────────

func TestBuildRequest_CacheBreakpoints(t *testing.T) {
	reg := newTestRegistry(&stubTool{name: "Read"}, &stubTool{name: "Bash"})
	eng := New(Config{Client: &mockClient{}, Registry: reg}).(*engineImpl)

	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("first")}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("reply")}}},
		{Role: types.RoleUser, Content: []types.ContentBlock{
			{Type: types.ContentTypeText, Text: strPtr("a")},
			{Type: types.ContentTypeText, Text: strPtr("b")},
		}},
	}
	params := QueryParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
			{Text: "stable prefix"},
			{Text: "dynamic tail"},
		}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}

	req, err := eng.buildRequestWithModel(params, messages, "m")
	if err != nil {
		t.Fatalf("buildRequestWithModel: %v", err)
	}

	// System: no explicit markers → fallback breakpoint on the last part only.
	if len(req.SystemBlocks) != 2 {
		t.Fatalf("SystemBlocks len = %d, want 2", len(req.SystemBlocks))
	}
	if req.SystemBlocks[0].CacheControl != nil {
		t.Error("system part 0 must not carry a breakpoint")
	}
	if req.SystemBlocks[1].CacheControl == nil || req.SystemBlocks[1].CacheControl.Type != "ephemeral" {
		t.Error("last system part must carry the ephemeral breakpoint")
	}

	// Tools: breakpoint on the last tool only.
	if len(req.Tools) != 2 {
		t.Fatalf("Tools len = %d, want 2", len(req.Tools))
	}
	if req.Tools[0].CacheControl != nil {
		t.Error("tool 0 must not carry a breakpoint")
	}
	if req.Tools[1].CacheControl == nil {
		t.Error("last tool must carry the breakpoint")
	}

	// Messages: rolling breakpoint on the final block of the last user message.
	var lastUser []types.ContentBlock
	if err := json.Unmarshal(req.Messages[2].Content, &lastUser); err != nil {
		t.Fatalf("unmarshal last user content: %v", err)
	}
	if lastUser[0].CacheControl != nil {
		t.Error("non-final block must not carry a breakpoint")
	}
	if lastUser[1].CacheControl == nil || lastUser[1].CacheControl.Type != "ephemeral" {
		t.Error("final block of last user message must carry the breakpoint")
	}
	var firstUser []types.ContentBlock
	if err := json.Unmarshal(req.Messages[0].Content, &firstUser); err != nil {
		t.Fatalf("unmarshal first user content: %v", err)
	}
	if firstUser[0].CacheControl != nil {
		t.Error("earlier user message must not carry a breakpoint")
	}

	// History must not be mutated: markers are request-side only.
	for i, m := range messages {
		for j, b := range m.Content {
			if b.CacheControl != nil {
				t.Errorf("history mutated: messages[%d].Content[%d] carries a marker", i, j)
			}
		}
	}
}

func TestBuildRequest_ExplicitSystemMarkerHonoured(t *testing.T) {
	eng := New(Config{Client: &mockClient{}, Registry: tools.NewRegistry()}).(*engineImpl)
	params := QueryParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
			{Text: "stable", CacheControl: "ephemeral"},
			{Text: "dynamic tail"},
		}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}
	req, err := eng.buildRequestWithModel(params, nil, "m")
	if err != nil {
		t.Fatalf("buildRequestWithModel: %v", err)
	}
	if req.SystemBlocks[0].CacheControl == nil {
		t.Error("explicit marker on part 0 must be honoured")
	}
	if req.SystemBlocks[1].CacheControl != nil {
		t.Error("fallback marker must not be added when an explicit marker exists")
	}
}

func TestBuildRequest_SkipCacheWrite(t *testing.T) {
	reg := newTestRegistry(&stubTool{name: "Read"})
	eng := New(Config{Client: &mockClient{}, Registry: reg}).(*engineImpl)
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hi")}}},
	}
	params := QueryParams{
		SystemPrompt:   SystemPrompt{Parts: []SystemPromptPart{{Text: "sys", CacheControl: "ephemeral"}}},
		SkipCacheWrite: true,
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}
	req, err := eng.buildRequestWithModel(params, messages, "m")
	if err != nil {
		t.Fatalf("buildRequestWithModel: %v", err)
	}
	if req.SystemBlocks[0].CacheControl != nil {
		t.Error("SkipCacheWrite must suppress system markers")
	}
	if req.Tools[0].CacheControl != nil {
		t.Error("SkipCacheWrite must suppress tool markers")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "cache_control") {
		t.Error("SkipCacheWrite must suppress all cache_control on the wire")
	}
}

func TestBuildRequest_WireFormatSystemBlocks(t *testing.T) {
	eng := New(Config{Client: &mockClient{}, Registry: tools.NewRegistry()}).(*engineImpl)
	params := QueryParams{
		SystemPrompt:   SystemPrompt{Parts: []SystemPromptPart{{Text: "sys"}}},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}
	req, err := eng.buildRequestWithModel(params, nil, "m")
	if err != nil {
		t.Fatalf("buildRequestWithModel: %v", err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body struct {
		System []struct {
			Type         string `json:"type"`
			Text         string `json:"text"`
			CacheControl *struct {
				Type string `json:"type"`
			} `json:"cache_control"`
		} `json:"system"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("system must serialise as a block array: %v", err)
	}
	if len(body.System) != 1 || body.System[0].Text != "sys" {
		t.Fatalf("unexpected system blocks: %+v", body.System)
	}
	if body.System[0].CacheControl == nil || body.System[0].CacheControl.Type != "ephemeral" {
		t.Error("system block must carry the ephemeral breakpoint on the wire")
	}
	if req.SystemText() != "sys" {
		t.Errorf("SystemText() = %q, want sys", req.SystemText())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Turn-boundary snapshot + inbox drain
// ─────────────────────────────────────────────────────────────────────────────

func TestQueryLoop_SnapshotCacheParams(t *testing.T) {
	client := &mockClient{streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
		return newStaticReader(buildEndTurnEvents("done")...), nil
	}}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	uctx := &tools.UseContext{Ctx: context.Background()}
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hello")}}},
	}
	params := QueryParams{
		Messages:       messages,
		SystemPrompt:   SystemPrompt{Parts: []SystemPromptPart{{Text: "sys"}}},
		ToolUseContext: uctx,
	}
	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	drainMsgs(ch)

	snap, ok := uctx.CacheParams.(*CacheSafeParams)
	if !ok || snap == nil {
		t.Fatal("uctx.CacheParams must hold a *CacheSafeParams after the query")
	}
	if snap.Model != "claude-test" {
		t.Errorf("snapshot Model = %q, want claude-test", snap.Model)
	}
	if len(snap.ContextMessages) != 1 {
		t.Errorf("snapshot ContextMessages len = %d, want 1", len(snap.ContextMessages))
	}
	if len(snap.SystemPrompt.Parts) != 1 || snap.SystemPrompt.Parts[0].Text != "sys" {
		t.Error("snapshot must carry the parent's system prompt")
	}
}

func TestQueryLoop_InboxDrainAtTurnBoundary(t *testing.T) {
	var mu sync.Mutex
	var requests []*api.MessageRequest
	call := 0
	client := &mockClient{streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
		mu.Lock()
		requests = append(requests, req)
		call++
		n := call
		mu.Unlock()
		if n == 1 {
			// First turn: call a tool so the loop continues to a second turn.
			return newStaticReader(buildToolUseEvents("tu1", "Read", `{}`)...), nil
		}
		return newStaticReader(buildEndTurnEvents("done")...), nil
	}}
	reg := newTestRegistry(&stubTool{name: "Read", safe: true})
	eng := New(Config{Client: client, Registry: reg, Model: "claude-test"})

	inbox := make(chan string, 4)
	inbox <- "follow-up from coordinator"

	uctx := &tools.UseContext{Ctx: context.Background()}
	params := QueryParams{
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("task")}}},
		},
		ToolUseContext: uctx,
		InboxCh:        inbox,
		NoWriteBack:    true,
	}
	ch, err := eng.Query(context.Background(), params)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := drainMsgs(ch)

	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 2 {
		t.Fatalf("expected >= 2 requests, got %d", len(requests))
	}
	// The inbox message is drained at the first turn boundary (before the
	// first API call), so it appears in the first request.
	found := false
	for _, m := range requests[0].Messages {
		var blocks []types.ContentBlock
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Text != nil && strings.Contains(*b.Text, "<coordinator-message>") &&
				strings.Contains(*b.Text, "follow-up from coordinator") {
				found = true
			}
		}
	}
	if !found {
		t.Error("first request must contain the drained coordinator message")
	}
	// A user-message event must surface the injection.
	if _, ok := findMsg(msgs, MsgTypeUserMessage); !ok {
		t.Error("inbox injection must emit MsgTypeUserMessage")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Fork prefix byte-identity (cache-first guard)
// ─────────────────────────────────────────────────────────────────────────────

// TestForkPrefix_ByteIdentity verifies that a forked agent's request shares a
// byte-identical prefix (tools + system + inherited messages) with the
// parent's request, so the provider's prompt cache hits. This is the guard
// test for the cache-first review gate: any change that moves the fork prefix
// must fail here.
func TestForkPrefix_ByteIdentity(t *testing.T) {
	reg := newTestRegistry(&stubTool{name: "Read"}, &stubTool{name: "Agent"})
	var mu sync.Mutex
	var requests []*api.MessageRequest
	client := &mockClient{streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		return newStaticReader(buildEndTurnEvents("ok")...), nil
	}}
	eng := New(Config{Client: client, Registry: reg, Model: "claude-test"})

	// Parent query with a stable system prompt and two-turn history.
	uctx := &tools.UseContext{Ctx: context.Background()}
	parentMsgs := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("question")}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("answer")}}},
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("follow-up")}}},
	}
	parentParams := QueryParams{
		Messages:       parentMsgs,
		SystemPrompt:   SystemPrompt{Parts: []SystemPromptPart{{Text: "parent system prompt"}}},
		ToolUseContext: uctx,
	}
	ch, err := eng.Query(context.Background(), parentParams)
	if err != nil {
		t.Fatalf("parent Query: %v", err)
	}
	drainMsgs(ch)

	snap, ok := uctx.CacheParams.(*CacheSafeParams)
	if !ok || snap == nil {
		t.Fatal("parent snapshot missing")
	}

	// Fork via RunForkedAgent with the parent's cache-safe params.
	directive := types.Message{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("side directive")}},
	}
	_, err = RunForkedAgent(context.Background(), eng, ForkedAgentConfig{
		PromptMessages:  []types.Message{directive},
		CacheSafeParams: snap,
		QuerySource:     "background",
	})
	if err != nil {
		t.Fatalf("RunForkedAgent: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests (parent + fork), got %d", len(requests))
	}
	parent, fork := requests[0], requests[1]

	// Tools: byte-identical schemas (fork must not filter the tool list).
	pt, err := json.Marshal(parent.Tools)
	if err != nil {
		t.Fatalf("marshal parent tools: %v", err)
	}
	ft, err := json.Marshal(fork.Tools)
	if err != nil {
		t.Fatalf("marshal fork tools: %v", err)
	}
	if string(pt) != string(ft) {
		t.Error("fork tools must be byte-identical to parent tools (cache prefix)")
	}

	// System: identical text and block structure.
	if parent.System != fork.System {
		t.Errorf("fork system differs: %q vs %q", parent.System, fork.System)
	}
	ps, err := json.Marshal(parent.SystemBlocks)
	if err != nil {
		t.Fatalf("marshal parent system blocks: %v", err)
	}
	fs, err := json.Marshal(fork.SystemBlocks)
	if err != nil {
		t.Fatalf("marshal fork system blocks: %v", err)
	}
	if string(ps) != string(fs) {
		t.Error("fork system blocks must be byte-identical to parent (cache prefix)")
	}

	// Messages: the parent's full message list is a content-identical prefix
	// of the fork's message list. cache_control markers are excluded from the
	// comparison: they are write-position metadata, not cached content (the
	// parent's rolling marker sits on its last user message, the fork's on
	// the directive; provider prefix matching ignores marker placement).
	if len(fork.Messages) != len(parent.Messages)+1 {
		t.Fatalf("fork messages len = %d, want %d", len(fork.Messages), len(parent.Messages)+1)
	}
	for i := range parent.Messages {
		pm := stripBlockMarkers(t, parent.Messages[i].Content)
		fm := stripBlockMarkers(t, fork.Messages[i].Content)
		if pm != fm {
			t.Errorf("fork message %d differs from parent (cache prefix broken)", i)
		}
	}

	// Model: fork inherits the parent's active model.
	if fork.Model != parent.Model {
		t.Errorf("fork model = %q, want parent model %q", fork.Model, parent.Model)
	}
}

// stripBlockMarkers normalises message content for prefix comparison: it
// removes request-side cache_control markers and re-marshals the blocks.
func stripBlockMarkers(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var blocks []types.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("unmarshal content blocks: %v", err)
	}
	for i := range blocks {
		blocks[i].CacheControl = nil
	}
	out, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal content blocks: %v", err)
	}
	return string(out)
}
