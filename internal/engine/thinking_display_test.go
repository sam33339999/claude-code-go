package engine

import (
	"context"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// TestQueryLoop_ThinkingDeltaEmitted guards the display chain: thinking
// content blocks (e.g. translated from OpenAI reasoning_content) must surface
// as MsgTypeThinkingDelta for the TUI and accumulate into the assistant
// message's thinking block.
func TestQueryLoop_ThinkingDeltaEmitted(t *testing.T) {
	events := []*api.StreamEvent{
		{
			Type: api.EventMessageStart,
			Data: mustMarshalRaw(map[string]any{
				"message": map[string]any{
					"id": "msg_th", "type": "message", "role": "assistant",
					"content": []any{}, "model": "claude-test",
					"usage": map[string]any{"input_tokens": 10, "output_tokens": 0},
				},
			}),
			MessageStart: &api.MessageStartData{
				Message: api.MessageResponse{ID: "msg_th", Usage: api.Usage{InputTokens: 10}},
			},
		},
		{
			Type: api.EventContentBlockStart,
			Data: mustMarshalRaw(map[string]any{
				"index":         0,
				"content_block": map[string]any{"type": "thinking", "thinking": ""},
			}),
		},
		{
			Type: api.EventContentBlockDelta,
			Data: mustMarshalRaw(map[string]any{
				"index": 0,
				"delta": map[string]any{"type": "thinking_delta", "thinking": "reasoning step"},
			}),
			ContentBlockDelta: &api.ContentBlockDeltaData{
				Index: 0,
				Delta: api.Delta{Type: "thinking_delta", Thinking: "reasoning step"},
			},
		},
		{
			Type: api.EventContentBlockStop,
			Data: mustMarshalRaw(map[string]any{"index": 0}),
		},
		{
			Type: api.EventContentBlockStart,
			Data: mustMarshalRaw(map[string]any{
				"index":         1,
				"content_block": map[string]any{"type": "text", "text": ""},
			}),
		},
		{
			Type: api.EventContentBlockDelta,
			Data: mustMarshalRaw(map[string]any{
				"index": 1,
				"delta": map[string]any{"type": "text_delta", "text": "answer"},
			}),
			ContentBlockDelta: &api.ContentBlockDeltaData{
				Index: 1,
				Delta: api.Delta{Type: "text_delta", Text: "answer"},
			},
		},
		{
			Type: api.EventContentBlockStop,
			Data: mustMarshalRaw(map[string]any{"index": 1}),
		},
		{
			Type: api.EventMessageDelta,
			Data: mustMarshalRaw(map[string]any{
				"delta": map[string]any{"stop_reason": "end_turn"},
				"usage": map[string]any{"output_tokens": 5},
			}),
			MessageDelta: &api.MessageDeltaData{},
		},
		{Type: api.EventMessageStop},
	}
	events[7].MessageDelta.Delta.StopReason = "end_turn"
	events[7].MessageDelta.Usage = api.Usage{OutputTokens: 5}

	client := &mockClient{streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
		return newStaticReader(events...), nil
	}}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	ch, err := eng.Query(context.Background(), QueryParams{
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hi")}}},
		},
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := drainMsgs(ch)

	th, ok := findMsg(msgs, MsgTypeThinkingDelta)
	if !ok || th.TextDelta != "reasoning step" {
		t.Fatalf("thinking delta = %+v (found=%v), want 'reasoning step'", th, ok)
	}
	am, ok := findMsg(msgs, MsgTypeAssistantMessage)
	if !ok || am.AssistantMsg == nil {
		t.Fatal("assistant message missing")
	}
	var thinking, text string
	for _, b := range am.AssistantMsg.Content {
		if b.Type == types.ContentTypeThinking && b.Thinking != nil {
			thinking += *b.Thinking
		}
		if b.Type == types.ContentTypeText && b.Text != nil {
			text += *b.Text
		}
	}
	if thinking != "reasoning step" || text != "answer" {
		t.Errorf("assistant content: thinking=%q text=%q, want 'reasoning step'/'answer'", thinking, text)
	}
}
