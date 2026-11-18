package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// TestPermissionDialog_CodeModeOptionText covers CM-03: CodeMode-originated
// requests offer the CodeMode-scoped persistent approval, not the project rule.
func TestPermissionDialog_CodeModeOptionText(t *testing.T) {
	d := newPermissionDialog(PermissionRequestMsg{
		RequestID:    "r1",
		ToolName:     "Bash",
		Message:      "Run something",
		ProjectPath:  "/tmp/proj",
		FromCodeMode: true,
		RespFn:       func(bool, bool) {},
	})
	view := d.View(80, DefaultDarkTheme)
	if !strings.Contains(view, "inside CodeMode") {
		t.Error("CodeMode dialog must offer the CodeMode-scoped always-allow option")
	}
	if strings.Contains(view, "from this project") {
		t.Error("CodeMode dialog must NOT offer the project-wide rule option")
	}

	plain := newPermissionDialog(PermissionRequestMsg{
		RequestID: "r2", ToolName: "Bash", Message: "Run", ProjectPath: "/tmp/proj",
		RespFn: func(bool, bool) {},
	})
	if !strings.Contains(plain.View(80, DefaultDarkTheme), "from this project") {
		t.Error("non-CodeMode dialog must keep the project-wide option")
	}
}

// TestListenPermission_CodeModeResponseMapping covers the CM-03 plumbing:
// "always allow" inside CodeMode maps to AlwaysInCodeMode, never AlwaysAllow.
func TestListenPermission_CodeModeResponseMapping(t *testing.T) {
	for _, fromCM := range []bool{true, false} {
		askCh := make(chan permissions.AskRequest, 1)
		respCh := make(chan permissions.AskResponse, 1)
		askCh <- permissions.AskRequest{ID: "x1", ToolName: "Bash", FromCodeMode: fromCM}

		cmd := listenForPermissionRequest(askCh, respCh)
		msg, ok := cmd().(PermissionRequestMsg)
		if !ok {
			t.Fatalf("expected PermissionRequestMsg, got %T", cmd())
		}
		if msg.FromCodeMode != fromCM {
			t.Fatalf("FromCodeMode must propagate (want %v)", fromCM)
		}
		msg.RespFn(true, true) // user picked "always allow"
		select {
		case resp := <-respCh:
			if resp.Decision != tools.PermissionAllow {
				t.Errorf("decision must be Allow, got %v", resp.Decision)
			}
			if resp.AlwaysInCodeMode != fromCM {
				t.Errorf("AlwaysInCodeMode: want %v, got %v", fromCM, resp.AlwaysInCodeMode)
			}
			if resp.AlwaysAllow != !fromCM {
				t.Errorf("AlwaysAllow: want %v, got %v", !fromCM, resp.AlwaysAllow)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("response not sent")
		}
	}
}
