package engine

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// TestQueryLoop_Retries429 guards the reactive-retry wiring: a 429 from the
// provider (e.g. Qwen rate limit) must be retried with backoff instead of
// failing the turn. Before this wiring, WithRetry existed but was never
// called by the engine.
func TestQueryLoop_Retries429(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	client := &mockClient{streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			return nil, &api.APIError{
				StatusCode: 429,
				Message:    "rate limited",
				Headers:    http.Header{"Retry-After": {"0"}},
			}
		}
		return newStaticReader(buildEndTurnEvents("recovered")...), nil
	}}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	ch, err := eng.Query(context.Background(), QueryParams{
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hi")}}},
		},
		QuerySource:    "foreground",
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := drainMsgs(ch)

	if _, ok := findMsg(msgs, MsgTypeError); ok {
		t.Fatal("429 must be retried, not surfaced as a turn error")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("provider calls = %d, want 2 (429 then retry)", calls)
	}
}

// TestQueryLoop_401FailsFast verifies auth errors do not burn the backoff
// ladder when no refresh hook is wired.
func TestQueryLoop_401FailsFast(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	client := &mockClient{streamFn: func(_ context.Context, _ *api.MessageRequest) (api.StreamReader, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil, &api.APIError{StatusCode: 401, Message: "bad key", Headers: http.Header{}}
	}}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	ch, err := eng.Query(context.Background(), QueryParams{
		Messages: []types.Message{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("hi")}}},
		},
		QuerySource:    "foreground",
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := drainMsgs(ch)
	if _, ok := findMsg(msgs, MsgTypeError); !ok {
		t.Fatal("401 must surface as an error")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("provider calls = %d, want 1 (fail fast on auth)", calls)
	}
}
