package tui

import (
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/msgqueue"
)

// ─────────────────────────────────────────────────────────────────────────────
// Coordinator completion wake (background agents must drive the main loop)
// ─────────────────────────────────────────────────────────────────────────────

// TestAgentCompletion_WakesCoordinatorWhenIdle guards the core fix: when a
// background agent finishes and the main loop is idle, a <task-notification>
// is injected and a query starts so the coordinator continues.
func TestAgentCompletion_WakesCoordinatorWhenIdle(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.coordinatorMode = true

	updated, cmd := m.Update(AgentStatusMsg{
		TaskID:      "aworker-1",
		Status:      AgentCompleted,
		Description: "summarize bigdoc",
		Result:      "the 3-sentence summary",
		Summary:     "agent aworker-1 completed",
	})
	m = updated.(AppModel)

	if !m.isLoading {
		t.Fatal("idle coordinator must be woken (query started) on agent completion")
	}
	if !strings.Contains(lastMessageText(t, m), "<task-notification") {
		t.Error("a <task-notification> must be injected into the conversation")
	}
	if !strings.Contains(lastMessageText(t, m), "the 3-sentence summary") {
		t.Error("the notification must carry the agent's result")
	}
	if cmd == nil {
		t.Fatal("expected a wake Cmd")
	}
	// Execute the Cmd: the fake engine must receive the notification as input.
	_ = cmd()
	fe := m.queryEngine.(*fakeEngine)
	found := false
	for _, msg := range fe.messages {
		for _, b := range msg.Content {
			if b.Text != nil && strings.Contains(*b.Text, "the 3-sentence summary") {
				found = true
			}
		}
	}
	if !found {
		t.Error("the wake query must feed the notification to the engine")
	}
}

// TestAgentCompletion_QueuedWhenRunning verifies that when the main loop is
// already running (e.g. a synchronous agent awaited via WaitForAgent), the
// notification is queued for the between-turn drain rather than restarting
// the query (which would abort the in-flight turn).
func TestAgentCompletion_QueuedWhenRunning(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.coordinatorMode = true
	m.isLoading = true // a query is in flight

	updated, _ := m.Update(AgentStatusMsg{
		TaskID: "aworker-2",
		Status: AgentCompleted,
		Result: "done",
	})
	m = updated.(AppModel)

	if m.msgQueue.Len() != 1 {
		t.Fatalf("notification must be queued while running, queue len = %d", m.msgQueue.Len())
	}
	if !m.isLoading {
		t.Error("must not disturb the in-flight query state")
	}
}

// TestAgentRunning_NoWake verifies running-status events do not inject wakes.
func TestAgentRunning_NoWake(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.coordinatorMode = true

	updated, _ := m.Update(AgentStatusMsg{TaskID: "aworker-3", Status: AgentRunning})
	m = updated.(AppModel)

	if m.msgQueue.Len() != 0 {
		t.Errorf("running status must not enqueue a wake, queue len = %d", m.msgQueue.Len())
	}
	if m.isLoading {
		t.Error("running status must not start a query")
	}
}

// TestAgentFailed_WakesCoordinator verifies failure also wakes the coordinator
// (so it can react to a failed delegation, not just success).
func TestAgentFailed_WakesCoordinator(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.coordinatorMode = true

	updated, _ := m.Update(AgentStatusMsg{
		TaskID: "aworker-4",
		Status: AgentFailed,
		Result: "boom",
	})
	m = updated.(AppModel)

	if !m.isLoading {
		t.Fatal("failed agent must also wake the idle coordinator")
	}
	if !strings.Contains(lastMessageText(t, m), "failed") {
		t.Error("notification must reflect the failed status")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Coordinator panel bounded height (must not squeeze the output viewport)
// ─────────────────────────────────────────────────────────────────────────────

func TestCoordinatorPanel_BoundedHeight(t *testing.T) {
	p := CoordinatorPanel{Tasks: map[string]AgentTaskState{}, TaskOrder: []string{}}
	// 12 agents: 2 running, 10 completed.
	for i := 0; i < 12; i++ {
		id := string(rune('a'+i)) + "-agent"
		st := AgentCompleted
		if i < 2 {
			st = AgentRunning
		}
		p.Tasks[id] = AgentTaskState{ID: id, Name: id, Status: st}
		p.TaskOrder = append(p.TaskOrder, id)
	}

	view := p.View(80, DefaultDarkTheme)
	if !strings.Contains(view, "more agents") {
		t.Error("overflow must be summarized with a '+N more agents' line")
	}
	// Count rendered agent rows (lines carrying the status icons), excluding
	// the header/footer. Bounded by maxPanelTaskRows.
	rows := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "●") || strings.Contains(line, "✓") || strings.Contains(line, "✗") {
			rows++
		}
	}
	if rows > maxPanelTaskRows {
		t.Errorf("rendered %d agent rows, want <= %d (panel must stay bounded)", rows, maxPanelTaskRows)
	}
	// Running agents must be prioritized into the visible window.
	if !strings.Contains(view, "a-agent") || !strings.Contains(view, "b-agent") {
		t.Error("running agents must stay visible in the bounded panel")
	}
}
