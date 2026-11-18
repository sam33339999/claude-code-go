package engine

import (
	"context"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/api"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// TestInterrupt_NotHijackedByBackgroundQuery guards the live-fire fix: a
// NoWriteBack (sub-agent/fork) query must not overwrite the interrupt slot,
// so Interrupt (Ctrl+C) still cancels the main conversation.
func TestInterrupt_NotHijackedByBackgroundQuery(t *testing.T) {
	mainStarted := make(chan struct{})
	client := &mockClient{streamFn: func(ctx context.Context, req *api.MessageRequest) (api.StreamReader, error) {
		if req.QuerySource == "foreground" {
			// Main query: block until cancelled.
			close(mainStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		// Background query: complete immediately.
		return newStaticReader(buildEndTurnEvents("bg done")...), nil
	}}
	eng := New(Config{Client: client, Registry: tools.NewRegistry(), Model: "claude-test"})

	mainCh, err := eng.Query(context.Background(), QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("main")}}}},
		QuerySource:    "foreground",
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	})
	if err != nil {
		t.Fatalf("main Query: %v", err)
	}
	select {
	case <-mainStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("main query did not start")
	}

	// Background (fork-style) query on the same engine.
	bgCh, err := eng.Query(context.Background(), QueryParams{
		Messages:       []types.Message{{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("bg")}}}},
		QuerySource:    "background",
		NoWriteBack:    true,
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	})
	if err != nil {
		t.Fatalf("bg Query: %v", err)
	}
	bgMsgs := drainMsgs(bgCh)
	if _, ok := findMsg(bgMsgs, MsgTypeError); ok {
		t.Fatal("background query must complete independently")
	}

	// Interrupt must cancel the MAIN query, not the (already finished) bg one.
	eng.Interrupt(context.Background())
	deadline := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-mainCh:
			if !ok {
				return // channel closed: main query observed the cancellation
			}
		case <-deadline:
			t.Fatal("Interrupt did not cancel the main query (slot hijacked by background query?)")
		}
	}
}
