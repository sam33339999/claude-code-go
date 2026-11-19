package permissions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// TestAskUser_CodeModeScope covers CM-03: FromCodeMode and Note propagate to
// the TUI ask, and an AlwaysInCodeMode approval does NOT create a project-wide
// allow rule (the CodeMode bridge persists it separately).
func TestAskUser_CodeModeScope(t *testing.T) {
	askCh := make(chan AskRequest, 1)
	respCh := make(chan AskResponse, 1)
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysAskRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"Write"},
			},
		},
		AskCh:  askCh,
		RespCh: respCh,
	})

	type outcome struct {
		res tools.PermissionResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := c.RequestPermission(context.Background(), PermissionRequest{
			ToolName:     "Write",
			ToolUseID:    "cm-1",
			FromCodeMode: true,
			Note:         "batch-writes gate scope",
		}, nil)
		done <- outcome{res, err}
	}()

	select {
	case req := <-askCh:
		if !req.FromCodeMode {
			t.Error("FromCodeMode must propagate to the AskRequest")
		}
		if !strings.Contains(req.Message, "batch-writes gate scope") {
			t.Errorf("Note must be appended to the message, got %q", req.Message)
		}
		respCh <- AskResponse{ID: req.ID, Decision: tools.PermissionAllow, AlwaysInCodeMode: true}
	case <-time.After(2 * time.Second):
		t.Fatal("ask not forwarded")
	}

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("request: %v", out.err)
		}
		if out.res.Behavior != tools.PermissionAllow || !out.res.AlwaysInCodeMode {
			t.Errorf("expected Allow with AlwaysInCodeMode, got %+v", out.res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete")
	}

	// No project rule was persisted: the same tool still asks.
	can, err := c.CanUseTool(context.Background(), "Write", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if can.Behavior != tools.PermissionAsk {
		t.Errorf("CodeMode-scoped approval must NOT become a project rule, got %q", can.Behavior)
	}
}
