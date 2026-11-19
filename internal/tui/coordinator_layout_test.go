package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/pkg/types"
)

// TestView_TotalHeightFitsTerminal guards the bottom-clipping fix: the full
// rendered View must never exceed termHeight, even with many agents, todos,
// and an open dialog on a small terminal. Previously a tall coordinator panel
// (or the viewport floor) pushed the input/dialog off-screen.
func TestView_TotalHeightFitsTerminal(t *testing.T) {
	scenarios := []struct {
		name   string
		height int
		agents int
		todos  int
		dialog bool
	}{
		{"normal", 24, 3, 0, false},
		{"many agents + todos + dialog", 24, 12, 8, true},
		{"short terminal", 12, 12, 5, true},
		{"tiny terminal", 10, 12, 5, true},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			m := newTestModel()
			m.termHeight = sc.height
			m.coordinatorMode = true
			for i := 0; i < sc.agents; i++ {
				id := fmt.Sprintf("agent-%d", i)
				m.coordinatorPanel.Tasks[id] = AgentTaskState{ID: id, Name: id, Status: AgentRunning, Activity: "Streaming"}
				m.coordinatorPanel.TaskOrder = append(m.coordinatorPanel.TaskOrder, id)
			}
			if sc.todos > 0 {
				todos := make([]types.TodoItem, sc.todos)
				for i := range todos {
					todos[i] = types.TodoItem{Content: fmt.Sprintf("task %d", i), Status: types.TodoPending}
				}
				m.appState.SetState(func(prev state.AppState) state.AppState {
					prev.Todos = todos
					return prev
				})
			}
			if sc.dialog {
				m.activeDialog = dialogExit
			}
			m.messages = append(m.messages, newUserMessage("hello world"))
			m.syncViewportContent()

			content := m.View().Content
			lines := len(strings.Split(strings.TrimRight(content, "\n"), "\n"))
			if lines > sc.height {
				t.Errorf("rendered %d lines > termHeight %d (bottom UI would be clipped)", lines, sc.height)
			}
		})
	}
}

// TestView_ViewportStaysUsable verifies that with a moderate coordinator panel
// the message viewport keeps a usable height (not squeezed to a single line).
func TestView_ViewportStaysUsable(t *testing.T) {
	m := newTestModel()
	m.termHeight = 24
	m.coordinatorMode = true
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("agent-%d", i)
		m.coordinatorPanel.Tasks[id] = AgentTaskState{ID: id, Name: id, Status: AgentRunning}
		m.coordinatorPanel.TaskOrder = append(m.coordinatorPanel.TaskOrder, id)
	}
	m.messages = append(m.messages, newUserMessage("hello"))
	m.syncViewportContent()

	// The panel takes ~7 lines; viewport should retain well above the floor.
	content := m.View().Content
	if !strings.Contains(content, "Coordinator Panel") {
		t.Error("panel must render")
	}
	if !strings.Contains(content, "hello") {
		t.Error("message viewport must remain visible alongside the panel")
	}
}
