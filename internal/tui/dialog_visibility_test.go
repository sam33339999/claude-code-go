package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// viewLines counts rendered terminal lines of the view content.
func viewLines(v tea.View) int {
	s := strings.TrimRight(v.Content, "\n")
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// TestView_PermissionDialogFitsTerminal is the regression for the invisible
// approval dialog: with a full-height message viewport, opening the dialog
// must shrink the viewport so the TOTAL view fits the terminal and the
// dialog text is actually rendered.
func TestView_PermissionDialogFitsTerminal(t *testing.T) {
	m := newTestModel() // termHeight 24
	// Fill the viewport with enough messages to occupy its full height.
	var msgs []types.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, newUserMessage("message line that fills the viewport"))
	}
	m.messages = msgs
	m.syncViewportContent()

	updated, _ := m.Update(PermissionRequestMsg{
		RequestID: "r1",
		ToolName:  "Bash",
		Message:   "Run: rm -rf /tmp/test",
		Input:     `{"command":"rm -rf /tmp/test"}`,
		RespFn:    func(bool, bool) {},
	})
	m = updated.(AppModel)
	if m.activeDialog != dialogPermission {
		t.Fatal("expected permission dialog open")
	}

	v := m.View()
	if lines := viewLines(v); lines > m.termHeight {
		t.Errorf("view height %d exceeds terminal height %d — dialog would be clipped", lines, m.termHeight)
	}
	if !strings.Contains(v.Content, "Run: rm -rf /tmp/test") {
		t.Error("permission dialog text must be visible in the view")
	}
}

// TestView_QuestionDialogFitsTerminal covers the AskUserQuestion dialog.
func TestView_QuestionDialogFitsTerminal(t *testing.T) {
	m := newTestModel()
	var msgs []types.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, newUserMessage("filler message for the viewport"))
	}
	m.messages = msgs
	m.syncViewportContent()

	updated, _ := m.Update(QuestionRequestMsg{
		RequestID: "q1",
		Question:  "Which database should we use?",
		Options:   []string{"postgres", "sqlite", "mysql"},
		RespFn:    func(string, bool) {},
	})
	m = updated.(AppModel)
	v := m.View()
	if lines := viewLines(v); lines > m.termHeight {
		t.Errorf("view height %d exceeds terminal height %d", lines, m.termHeight)
	}
	if !strings.Contains(v.Content, "Which database should we use?") {
		t.Error("question dialog text must be visible")
	}
}

// TestView_DialogPlusTodoPanelFits covers the combined extras case.
func TestView_DialogPlusTodoPanelFits(t *testing.T) {
	m := newTestModel()
	m.appState.SetState(func(prev state.AppState) state.AppState {
		prev.Todos = []types.TodoItem{
			{ID: "1", Content: "task one", Status: types.TodoInProgress},
			{ID: "2", Content: "task two", Status: types.TodoPending},
		}
		return prev
	})
	var msgs []types.Message
	for i := 0; i < 40; i++ {
		msgs = append(msgs, newUserMessage("filler message for the viewport"))
	}
	m.messages = msgs
	m.syncViewportContent()

	updated, _ := m.Update(PermissionRequestMsg{
		RequestID: "r1", ToolName: "Bash", Message: "Run something",
		RespFn: func(bool, bool) {},
	})
	m = updated.(AppModel)
	v := m.View()
	if lines := viewLines(v); lines > m.termHeight {
		t.Errorf("view height %d exceeds terminal height %d with dialog+todos", lines, m.termHeight)
	}
	if !strings.Contains(v.Content, "Run something") || !strings.Contains(v.Content, "task one") {
		t.Error("both dialog and todo panel must be visible")
	}
}

// TestView_NoDialogUnchanged guards the normal (no-dialog) rendering path.
func TestView_NoDialogUnchanged(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{newUserMessage("hello")}
	m.syncViewportContent()
	v := m.View()
	if !strings.Contains(v.Content, "hello") {
		t.Error("message must render without dialog")
	}
	if lines := viewLines(v); lines > m.termHeight {
		t.Errorf("normal view height %d exceeds terminal %d", lines, m.termHeight)
	}
}
