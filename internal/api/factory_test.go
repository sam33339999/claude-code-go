package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNormalizeBaseURL verifies that a trailing version segment is trimmed so
// clients can append their own versioned path without producing ".../v1/v1/...".
func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already versioned", "https://host/compatible-mode/v1", "https://host/compatible-mode"},
		{"versioned with trailing slash", "https://host/compatible-mode/v1/", "https://host/compatible-mode"},
		{"plain host", "https://api.anthropic.com", "https://api.anthropic.com"},
		{"host with trailing slash", "https://api.openai.com/", "https://api.openai.com"},
		{"custom gateway without version", "https://gw.example.com/openai", "https://gw.example.com/openai"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBaseURL(tt.input); got != tt.want {
				t.Errorf("normalizeBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestOpenAIClient_VersionedBaseURL_NoDuplicateV1 verifies end-to-end that an
// OpenAI-compatible base URL that already ends in /v1 is not doubled.
func TestOpenAIClient_VersionedBaseURL_NoDuplicateV1(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openaiChatCompletionResponse{
			ID:     "chatcmpl-1",
			Object: "chat.completion",
			Model:  "qwen3.7-max",
			Choices: []openaiChoice{{
				Index:        0,
				Message:      openaiResponseMessage{Role: "assistant", Content: "Hi"},
				FinishReason: "stop",
			}},
		})
	}))
	defer server.Close()

	c, err := NewClient(ClientConfig{
		Provider: ProviderOpenAI,
		APIKey:   "key",
		BaseURL:  server.URL + "/compatible-mode/v1",
	}, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Complete(context.Background(), &MessageRequest{
		Model:     "qwen3.7-max",
		MaxTokens: 16,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := "/compatible-mode/v1/chat/completions"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
}

// TestOpenAIURL verifies that a base URL already ending in a version segment
// other than /v1 (e.g. z.ai's ".../paas/v4") is used as-is, while plain and
// /v1 bases keep the /v1 prefix.
func TestOpenAIURL(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{"https://api.z.ai/api/coding/paas/v4", "/chat/completions", "https://api.z.ai/api/coding/paas/v4/chat/completions"},
		{"https://host/api/v2", "/models", "https://host/api/v2/models"},
		{"https://api.openai.com", "/models", "https://api.openai.com/v1/models"},
		{"https://host/compatible-mode", "/chat/completions", "https://host/compatible-mode/v1/chat/completions"},
		{"https://host/openai/vendor", "/models", "https://host/openai/vendor/v1/models"},
		{"https://host/v4beta", "/models", "https://host/v4beta/v1/models"},
	}
	for _, tt := range tests {
		if got := openAIURL(tt.base, tt.path); got != tt.want {
			t.Errorf("openAIURL(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
		}
	}
}

// TestOpenAIClient_V4BaseURL verifies a z.ai-style ".../v4" base posts to
// ".../v4/chat/completions", not ".../v4/v1/chat/completions" (404).
func TestOpenAIClient_V4BaseURL(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openaiChatCompletionResponse{
			ID: "chatcmpl-1", Object: "chat.completion", Model: "glm-5",
			Choices: []openaiChoice{{Message: openaiResponseMessage{Role: "assistant", Content: "Hi"}, FinishReason: "stop"}},
		})
	}))
	defer server.Close()

	c, err := NewClient(ClientConfig{Provider: ProviderOpenAI, APIKey: "key", BaseURL: server.URL + "/api/coding/paas/v4"}, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.Complete(context.Background(), &MessageRequest{
		Model: "glm-5", MaxTokens: 16,
		Messages: []MessageParam{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if want := "/api/coding/paas/v4/chat/completions"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
}
