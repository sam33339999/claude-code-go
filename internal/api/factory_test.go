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
