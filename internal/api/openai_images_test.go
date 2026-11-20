package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const testPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC"

// TestOpenAIConvert_ToolResultImage verifies an image returned by a tool
// reaches an OpenAI-compatible provider as an image_url part (in a user
// message following the tool messages) instead of base64 text in the tool
// message.
func TestOpenAIConvert_ToolResultImage(t *testing.T) {
	client := &openaiClient{}
	content := fmt.Sprintf(`[
		{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":%q}}]},
		{"type":"tool_result","tool_use_id":"call_2","content":[{"type":"text","text":"ok"}]}
	]`, testPNGBase64)
	msgs := client.convertMessage(MessageParam{Role: "user", Content: json.RawMessage(content)})

	if len(msgs) != 3 {
		t.Fatalf("got %d messages, want 3 (tool, tool, user-with-image)", len(msgs))
	}
	if msgs[0].Role != "tool" || msgs[0].ToolCallID != "call_1" || msgs[1].Role != "tool" || msgs[1].ToolCallID != "call_2" {
		t.Fatalf("tool messages must stay contiguous and first: %+v", msgs)
	}
	if strings.Contains(*msgs[0].Content, testPNGBase64) {
		t.Errorf("tool message must not carry base64 as text: %q", *msgs[0].Content)
	}
	raw, err := json.Marshal(msgs[2])
	if err != nil {
		t.Fatal(err)
	}
	want := `"image_url":{"url":"data:image/png;base64,` + testPNGBase64 + `"}`
	if msgs[2].Role != "user" || !strings.Contains(string(raw), want) {
		t.Errorf("image user message = %s, want role user containing %s", raw, want)
	}
}

// TestOpenAIConvert_UserImage verifies an image block in a user message is
// sent as an image_url part.
func TestOpenAIConvert_UserImage(t *testing.T) {
	client := &openaiClient{}
	content := fmt.Sprintf(`[{"type":"text","text":"what is this?"},{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":%q}}]`, testPNGBase64)
	msgs := client.convertMessage(MessageParam{Role: "user", Content: json.RawMessage(content)})
	raw, err := json.Marshal(msgs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"text":"what is this?"`) && !strings.Contains(string(raw), `"content":"what is this?"`) {
		t.Errorf("text lost: %s", raw)
	}
	if !strings.Contains(string(raw), `data:image/jpeg;base64,`+testPNGBase64) {
		t.Errorf("image not sent as image_url: %s", raw)
	}
}

// TestOpenAIConvert_NoImageBytesUnchanged guards the cache-first rule:
// image-free content must serialise exactly as before.
func TestOpenAIConvert_NoImageBytesUnchanged(t *testing.T) {
	client := &openaiClient{}
	content := `[{"type":"tool_result","tool_use_id":"c","content":[{"type":"text","text":"out"}]},{"type":"text","text":"next"}]`
	raw, err := json.Marshal(client.convertMessage(MessageParam{Role: "user", Content: json.RawMessage(content)}))
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"role":"tool","content":"out","tool_call_id":"c"},{"role":"user","content":"next"}]`
	if string(raw) != want {
		t.Errorf("bytes changed:\n got %s\nwant %s", raw, want)
	}
}

func TestIsImageUnsupportedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"zai", &APIError{StatusCode: 400, Message: `{"error":{"code":"1210","message":"messages.content.type is invalid, allowed values: ['text']"}}`}, true},
		{"openai", &APIError{StatusCode: 400, Message: "Invalid content type. image_url is only supported by certain models."}, true},
		{"deepseek", &APIError{StatusCode: 400, Message: "Failed to deserialize the JSON body into the target type: messages[3]: unknown variant `image_url`, expected `text`"}, true},
		{"anthropic-ish", &APIError{StatusCode: 400, Message: "This model does not support image input."}, true},
		{"wrapped", fmt.Errorf("stream: %w", &CannotRetryError{Cause: &APIError{StatusCode: 400, Message: "image input not supported"}}), true},
		{"other 400", &APIError{StatusCode: 400, Message: "max_tokens must be positive"}, false},
		{"500 mentioning image", &APIError{StatusCode: 500, Message: "image service down"}, false},
		{"plain", errors.New("image"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsImageUnsupportedError(tc.err); got != tc.want {
				t.Errorf("IsImageUnsupportedError = %v, want %v", got, tc.want)
			}
		})
	}
}
