package bootstrap

import (
	"context"
	"testing"

	"github.com/tunsuy/glaude/internal/coordinator"
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// TestSubAgentResult_FallsBackToAssistantText guards the empty-result fix:
// when streamed text deltas are absent (tool-only final turn, max_turns
// cutoff), the result must still carry the assistant message's text rather
// than returning empty to the coordinator.
func TestSubAgentResult_FallsBackToAssistantText(t *testing.T) {
	assistant := textMsg(types.RoleAssistant, "the final summary")
	eng := &stubQueryEngine{events: []engine.Msg{
		// No MsgTypeStreamText — only the authoritative assistant message.
		{Type: engine.MsgTypeAssistantMessage, AssistantMsg: &assistant},
	}}
	fn := buildRunAgentFn(eng, tools.NewRegistry(), forkTestState(), nil, forkTestProfiles())

	result, err := fn(context.Background(), "a-worker-r1", coordinator.SpawnRequest{
		SubagentType: "worker",
		Prompt:       "summarize",
		MaxTurns:     coordinator.DefaultMaxTurns,
	}, make(chan string, 1))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result != "the final summary" {
		t.Errorf("result = %q, want assistant text fallback 'the final summary'", result)
	}
}

// TestSubAgentResult_PrefersLastAssistantText verifies the result is the last
// text-bearing assistant message (the final report), not intermediate turns.
func TestSubAgentResult_PrefersLastAssistantText(t *testing.T) {
	first := textMsg(types.RoleAssistant, "let me read the file")
	final := textMsg(types.RoleAssistant, "FINAL ANSWER")
	eng := &stubQueryEngine{events: []engine.Msg{
		{Type: engine.MsgTypeStreamText, TextDelta: "let me read the file"},
		{Type: engine.MsgTypeAssistantMessage, AssistantMsg: &first},
		{Type: engine.MsgTypeStreamText, TextDelta: "FINAL ANSWER"},
		{Type: engine.MsgTypeAssistantMessage, AssistantMsg: &final},
	}}
	fn := buildRunAgentFn(eng, tools.NewRegistry(), forkTestState(), nil, forkTestProfiles())

	result, err := fn(context.Background(), "a-worker-r2", coordinator.SpawnRequest{
		SubagentType: "worker",
		Prompt:       "task",
		MaxTurns:     coordinator.DefaultMaxTurns,
	}, make(chan string, 1))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result != "FINAL ANSWER" {
		t.Errorf("result = %q, want last assistant text 'FINAL ANSWER'", result)
	}
}

func TestSubAgentResult_Helper(t *testing.T) {
	if got := subAgentResult("stream", "assistant"); got != "assistant" {
		t.Errorf("prefer assistant: got %q", got)
	}
	if got := subAgentResult("stream", ""); got != "stream" {
		t.Errorf("fallback stream: got %q", got)
	}
	if got := subAgentResult("", ""); got != "" {
		t.Errorf("both empty: got %q", got)
	}
}
