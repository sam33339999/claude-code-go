package api

import (
	"encoding/json"
	"testing"
)

// TestOpenAIConvert_ToolCallArgumentsAlwaysJSONObject verifies a replayed
// tool_use block always reaches an OpenAI-compatible provider with
// function.arguments set to a JSON object. Strict providers (e.g. DashScope)
// reject "null", "" or malformed JSON with HTTP 400, which happens when a
// model emitted a tool call with no or truncated arguments.
func TestOpenAIConvert_ToolCallArgumentsAlwaysJSONObject(t *testing.T) {
	cases := []struct {
		name  string
		input string // raw JSON for the "input" field; "" omits it
		want  string
	}{
		{"missing input", "", "{}"},
		{"null input", `null`, "{}"},
		{"empty string input", `""`, "{}"},
		{"non-object input", `[1,2]`, "{}"},
		{"valid object", `{"path":"a.go"}`, `{"path":"a.go"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			block := `{"type":"tool_use","id":"call_1","name":"MemoryRead"`
			if tc.input != "" {
				block += `,"input":` + tc.input
			}
			block += `}`
			client := &openaiClient{}
			msgs := client.convertMessage(MessageParam{Role: "assistant", Content: json.RawMessage(`[` + block + `]`)})
			if len(msgs) != 1 || len(msgs[0].ToolCalls) != 1 {
				t.Fatalf("got %+v, want one assistant message with one tool call", msgs)
			}
			if got := msgs[0].ToolCalls[0].Function.Arguments; got != tc.want {
				t.Errorf("arguments = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNormalizeToolArguments_Malformed covers input that cannot be decoded
// from a message (the accumulator stores raw streamed bytes verbatim, so a
// truncated stream leaves malformed JSON in the block).
func TestNormalizeToolArguments_Malformed(t *testing.T) {
	for _, in := range []json.RawMessage{nil, json.RawMessage(`{"a":`), json.RawMessage(`  `)} {
		if got := normalizeToolArguments(in); got != "{}" {
			t.Errorf("normalizeToolArguments(%q) = %q, want {}", in, got)
		}
	}
	if got := normalizeToolArguments(json.RawMessage(` {"a": 1} `)); got != `{"a": 1}` {
		t.Errorf("valid object should pass through trimmed, got %q", got)
	}
}
