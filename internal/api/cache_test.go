package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Prompt-cache wire format
// ─────────────────────────────────────────────────────────────────────────────

func TestMessageRequest_MarshalSystemString(t *testing.T) {
	req := MessageRequest{Model: "m", MaxTokens: 10, System: "plain system"}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body struct {
		System string `json:"system"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("system must stay a string without SystemBlocks: %v", err)
	}
	if body.System != "plain system" {
		t.Errorf("system = %q, want plain system", body.System)
	}
}

func TestMessageRequest_MarshalSystemBlocks(t *testing.T) {
	req := MessageRequest{
		Model:     "m",
		MaxTokens: 10,
		System:    "stable\n\ndynamic",
		SystemBlocks: []SystemBlock{
			{Type: "text", Text: "stable", CacheControl: EphemeralCache},
			{Type: "text", Text: "dynamic"},
		},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var body struct {
		System []struct {
			Type         string        `json:"type"`
			Text         string        `json:"text"`
			CacheControl *CacheControl `json:"cache_control"`
		} `json:"system"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("system must serialise as blocks: %v", err)
	}
	if len(body.System) != 2 {
		t.Fatalf("system blocks len = %d, want 2", len(body.System))
	}
	if body.System[0].CacheControl == nil || body.System[0].CacheControl.Type != "ephemeral" {
		t.Error("first block must carry the ephemeral breakpoint")
	}
	if body.System[1].CacheControl != nil {
		t.Error("second block must not carry a breakpoint")
	}
	// The plain System field must not leak as a duplicate key.
	if strings.Count(string(raw), `"system"`) != 1 {
		t.Errorf("system key must appear exactly once: %s", raw)
	}
}

func TestMessageRequest_SystemText(t *testing.T) {
	req := MessageRequest{System: "fallback"}
	if req.SystemText() != "fallback" {
		t.Errorf("SystemText without blocks = %q, want fallback", req.SystemText())
	}
	req.SystemBlocks = []SystemBlock{
		{Type: "text", Text: "a"},
		{Type: "text", Text: ""},
		{Type: "text", Text: "b"},
	}
	if req.SystemText() != "a\n\nb" {
		t.Errorf("SystemText with blocks = %q, want a\\n\\nb", req.SystemText())
	}
}

func TestToolSchema_CacheControlWire(t *testing.T) {
	tools := []ToolSchema{
		{Name: "a", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "b", InputSchema: json.RawMessage(`{"type":"object"}`), CacheControl: EphemeralCache},
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `"name":"a","description":"","input_schema":{"type":"object"},"cache_control"`) {
		t.Error("tool a must not carry cache_control")
	}
	if !strings.Contains(s, `"cache_control":{"type":"ephemeral"}`) {
		t.Errorf("tool b must carry the ephemeral breakpoint: %s", s)
	}
}

func TestOpenAIConvert_SystemBlocksBecomePlainText(t *testing.T) {
	c := &openaiClient{}
	req := &MessageRequest{
		Model:     "m",
		MaxTokens: 10,
		System:    "ignored-when-blocks-set",
		SystemBlocks: []SystemBlock{
			{Type: "text", Text: "stable", CacheControl: EphemeralCache},
			{Type: "text", Text: "dynamic"},
		},
	}
	out := c.convertToOpenAIRequest(req)
	if len(out.Messages) == 0 || out.Messages[0].Role != "system" {
		t.Fatal("expected a system message")
	}
	if out.Messages[0].Content == nil || *out.Messages[0].Content != "stable\n\ndynamic" {
		t.Errorf("OpenAI system content must be the joined block text, got %v", out.Messages[0].Content)
	}
}
