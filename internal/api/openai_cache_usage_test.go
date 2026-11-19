package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Prompt-cache usage mapping (OpenAI-compatible providers)
// ─────────────────────────────────────────────────────────────────────────────

func TestOpenAIUsage_CachedTokens(t *testing.T) {
	tests := []struct {
		name string
		u    *openaiUsage
		want int
	}{
		{"nil", nil, 0},
		{"no cache fields", &openaiUsage{PromptTokens: 100}, 0},
		{"openai details", &openaiUsage{PromptTokens: 100, PromptTokensDetails: &openaiPromptTokensDetails{CachedTokens: 80}}, 80},
		{"deepseek style", &openaiUsage{PromptTokens: 100, PromptCacheHitTokens: 64}, 64},
		{"details preferred", &openaiUsage{PromptTokens: 100, PromptTokensDetails: &openaiPromptTokensDetails{CachedTokens: 80}, PromptCacheHitTokens: 64}, 80},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.u.cachedTokens(); got != tc.want {
				t.Errorf("cachedTokens() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestOpenAIStream_CacheUsageFlows verifies cache hit counts reported by the
// provider in the final usage chunk reach the accumulated MessageResponse,
// and that streaming requests ask for usage (stream_options.include_usage).
func TestOpenAIStream_CacheUsageFlows(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-c1","object":"chat.completion.chunk","created":1677652288,"model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"},"finish_reason":null}]}

data: {"id":"chatcmpl-c1","object":"chat.completion.chunk","created":1677652288,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"chatcmpl-c1","object":"chat.completion.chunk","created":1677652288,"model":"m","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_tokens_details":{"cached_tokens":80}}}

data: [DONE]

`
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseData))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{Provider: ProviderOpenAI, APIKey: "k", BaseURL: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := client.Stream(context.Background(), &MessageRequest{
		Model:     "m",
		MaxTokens: 10,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	acc := &Accumulator{}
	for {
		ev, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if ev == nil {
			continue
		}
		if err := acc.Process(ev); err != nil {
			t.Fatal(err)
		}
	}
	result := acc.Result()
	if result.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", result.Usage.InputTokens)
	}
	if result.Usage.CacheReadInputTokens != 80 {
		t.Errorf("CacheReadInputTokens = %d, want 80 (provider cache hit must be observable)", result.Usage.CacheReadInputTokens)
	}

	opts, ok := gotBody["stream_options"].(map[string]any)
	if !ok || opts["include_usage"] != true {
		t.Errorf("request must set stream_options.include_usage=true, got %v", gotBody["stream_options"])
	}
}

// TestOpenAIComplete_CacheUsage verifies the non-streaming path maps cache hits.
func TestOpenAIComplete_CacheUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-c2","object":"chat.completion","created":1677652288,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":50,"completion_tokens":2,"total_tokens":52,"prompt_cache_hit_tokens":32}}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{Provider: ProviderOpenAI, APIKey: "k", BaseURL: server.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Complete(context.Background(), &MessageRequest{
		Model:     "m",
		MaxTokens: 10,
		Messages:  []MessageParam{{Role: "user", Content: json.RawMessage(`"hi"`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Usage.CacheReadInputTokens != 32 {
		t.Errorf("CacheReadInputTokens = %d, want 32 (DeepSeek-style hit count)", resp.Usage.CacheReadInputTokens)
	}
}

// TestAccumulator_DeltaUsageMerge verifies final input/cache counts from
// message_delta overwrite the message_start snapshot while output tokens
// still accumulate (Anthropic delta semantics preserved).
func TestAccumulator_DeltaUsageMerge(t *testing.T) {
	acc := &Accumulator{}
	if err := acc.Process(&StreamEvent{
		Type: EventMessageStart,
		MessageStart: &MessageStartData{Message: MessageResponse{
			ID: "m", Usage: Usage{InputTokens: 10},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := acc.Process(&StreamEvent{
		Type: EventMessageDelta,
		MessageDelta: &MessageDeltaData{Usage: Usage{
			OutputTokens:         7,
			InputTokens:          100,
			CacheReadInputTokens: 80,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	u := acc.Result().Usage
	if u.InputTokens != 100 || u.CacheReadInputTokens != 80 || u.OutputTokens != 7 {
		t.Errorf("usage = %+v, want input 100 / cache read 80 / output 7", u)
	}
}
