package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

func TestToolResultContent_PreservesImage(t *testing.T) {
	raw := json.RawMessage(`{"type":"tool_result","tool_use_id":"x","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUJD"}}]}`)
	blocks, display := toolResultContent(raw)
	if len(blocks) != 1 || blocks[0].Type != types.ContentTypeImage || blocks[0].Source == nil || blocks[0].Source.Data != "QUJD" {
		t.Fatalf("image block not preserved: %+v", blocks)
	}
	if strings.Contains(display, "QUJD") || !strings.Contains(display, "image/png") {
		t.Errorf("display = %q, want short image label without base64", display)
	}
}

func TestToolResultContent_LegacyShapesUnchanged(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"string content", `{"type":"tool_result","content":"hello"}`, "hello"},
		{"bare string", `"ok"`, `"ok"`},
		{"text array stays raw", `{"content":[{"type":"text","text":"a"}]}`, `{"content":[{"type":"text","text":"a"}]}`},
		{"missing content", `{"type":"tool_result"}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks, display := toolResultContent(json.RawMessage(tc.raw))
			if len(blocks) != 1 || blocks[0].Type != types.ContentTypeText || *blocks[0].Text != tc.want || display != tc.want {
				t.Errorf("got blocks=%+v display=%q, want single text %q", blocks, display, tc.want)
			}
		})
	}
}

func TestStripImages(t *testing.T) {
	img := types.ContentBlock{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "QUJD"}}
	id := "t1"
	msgs := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("look")}, img}},
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeToolResult, ToolUseID: &id, Content: []types.ContentBlock{img}}}},
	}
	if !hasImages(msgs) {
		t.Fatal("hasImages = false, want true")
	}
	out := stripImages(msgs)
	if hasImages(out) {
		t.Fatal("stripImages left an image")
	}
	if !hasImages(msgs) {
		t.Fatal("stripImages mutated its input")
	}
	if out[0].Content[1].Type != types.ContentTypeText || !strings.Contains(*out[0].Content[1].Text, "image") {
		t.Errorf("user image not replaced by placeholder: %+v", out[0].Content[1])
	}
	inner := out[1].Content[0].Content
	if len(inner) != 1 || inner[0].Type != types.ContentTypeText {
		t.Errorf("tool_result image not replaced by placeholder: %+v", inner)
	}
}

// TestQueryLoop_ImageRejectedRetriesWithoutImages: a provider rejecting image
// input must not wedge the conversation. The turn is retried once with
// placeholders, and later turns on the same model skip images up front.
func TestQueryLoop_ImageRejectedRetriesWithoutImages(t *testing.T) {
	var mu sync.Mutex
	var sawImage []bool
	client := &mockClient{streamFn: func(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
		withImage := false
		for _, m := range req.Messages {
			if strings.Contains(string(m.Content), `"type":"image"`) {
				withImage = true
			}
		}
		mu.Lock()
		sawImage = append(sawImage, withImage)
		mu.Unlock()
		if withImage {
			return nil, &api.APIError{StatusCode: 400, Message: "messages.content.type is invalid, allowed values: ['text']", Headers: http.Header{}}
		}
		return newStaticReader(buildEndTurnEvents("fine")...), nil
	}}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "text-only"})
	history := []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{
		{Type: types.ContentTypeText, Text: strPtr("look")},
		{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "QUJD"}},
	}}}

	for turn := 0; turn < 2; turn++ {
		ch, err := eng.Query(context.Background(), QueryParams{
			Messages:       history,
			QuerySource:    "foreground",
			ToolUseContext: &tools.UseContext{Ctx: context.Background()},
		})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		msgs := drainMsgs(ch)
		if m, ok := findMsg(msgs, MsgTypeError); ok {
			t.Fatalf("turn %d: unexpected error %v", turn, m.Err)
		}
		if turn == 0 {
			if _, ok := findMsg(msgs, MsgTypeSystemMessage); !ok {
				t.Error("turn 0: want a system notice that images are omitted")
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	want := []bool{true, false, false}
	if len(sawImage) != len(want) || sawImage[0] != want[0] || sawImage[1] != want[1] || sawImage[2] != want[2] {
		t.Errorf("requests carried images = %v, want %v (reject, retry stripped, next turn stripped up front)", sawImage, want)
	}
	if !hasImages(history) {
		t.Error("caller's history must not be mutated")
	}
}
