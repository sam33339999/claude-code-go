package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/msgqueue"
)

var ctrlY = tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl}

// doneBTW runs a /btw to completion with the given answer.
func doneBTW(t *testing.T, loading bool) (AppModel, *btwEngine) {
	t.Helper()
	m, be := newBTWModel(t, nil)
	m.isLoading = loading
	m, cmd := submit(m, "/btw what is the cap?")
	m, cmd = feed(m, be, cmd, engine.Msg{Type: engine.MsgTypeStreamText, TextDelta: "32 seconds"})
	return closeBTW(m, be, cmd), be
}

func TestBTW_SendWhileMainRunning(t *testing.T) {
	m, _ := doneBTW(t, true)
	m, _ = press(m, ctrlY)
	q := m.msgQueue.Snapshot()
	if len(q) != 1 || q[0].Mode != msgqueue.ModePrompt || q[0].Origin != msgqueue.OriginBTW {
		t.Fatalf("queue = %+v", q)
	}
	if !strings.Contains(q[0].Value, "[btw] Q: what is the cap?") || !strings.Contains(q[0].Value, "A: 32 seconds") {
		t.Errorf("value = %q", q[0].Value)
	}
	if !strings.Contains(btwBlock(m), "sent to main") {
		t.Errorf("block = %q", btwBlock(m))
	}
}

func TestBTW_SendWhenIdleAttachesToNext(t *testing.T) {
	m, be := doneBTW(t, false)
	m, _ = press(m, ctrlY)
	if m.msgQueue.Len() != 0 {
		t.Fatal("idle send must not queue a prompt")
	}
	if !strings.Contains(m.View().Content, "btw note will be attached") {
		t.Error("pending hint not shown")
	}
	m, cmd := submit(m, "hello")
	runCmds(cmd)
	if len(be.queries) < 2 {
		t.Fatal("main query not started")
	}
	msgs := be.queries[len(be.queries)-1].Messages
	text := *msgs[len(msgs)-1].Content[0].Text
	if !strings.HasPrefix(text, "[btw] Q:") || !strings.HasSuffix(text, "hello") {
		t.Errorf("main prompt = %q", text)
	}
	if m.btwPending != "" {
		t.Error("pending note not cleared after attaching")
	}
}

func TestBTW_PendingCancelledByEsc(t *testing.T) {
	m, _ := doneBTW(t, false)
	m, _ = press(m, ctrlY)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.btwPending != "" {
		t.Error("Esc must cancel the pending btw note")
	}
}

func TestBTW_UnconsumedBecomesPending(t *testing.T) {
	m, _ := doneBTW(t, true)
	m, _ = press(m, ctrlY)
	updated, _ := m.Update(StreamDoneMsg{})
	m = updated.(AppModel)
	if m.msgQueue.Len() != 0 {
		t.Errorf("btw note still queued (would start a new turn): %d", m.msgQueue.Len())
	}
	if !strings.HasPrefix(m.btwPending, "[btw] Q:") {
		t.Errorf("pending = %q", m.btwPending)
	}
	if m.isLoading {
		t.Error("no new main turn may start for a btw note")
	}
}

func TestBTW_SendOnce(t *testing.T) {
	m, _ := doneBTW(t, true)
	m, _ = press(m, ctrlY)
	m, _ = press(m, ctrlY)
	if m.msgQueue.Len() != 1 {
		t.Errorf("queued %d, want 1", m.msgQueue.Len())
	}
}

func TestBTW_SendBeforeDoneIgnored(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.isLoading = true
	m, _ = submit(m, "/btw q")
	m, _ = press(m, ctrlY)
	if m.msgQueue.Len() != 0 || m.btwPending != "" {
		t.Error("ctrl+y before the answer is done must do nothing")
	}
}

// runCmds executes cmd, expanding batches (without waiting on blocking ones).
func runCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				runCmds(c)
			}
		}
	case <-time.After(200 * time.Millisecond):
	}
}

// Final review I-1: Esc on a finished btw dismisses it and never interrupts
// the main loop.
func TestBTW_EscOnDoneDismissesOnly(t *testing.T) {
	m, _ := doneBTW(t, true)
	aborted := false
	m.abortFn = func() { aborted = true }
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if aborted || !m.isLoading {
		t.Fatal("Esc on a finished btw interrupted the main loop")
	}
	if strings.Contains(btwBlock(m), "ctrl+y") {
		t.Errorf("hint still shown after dismiss: %q", btwBlock(m))
	}
	m, _ = press(m, ctrlY)
	if m.msgQueue.Len() != 0 {
		t.Error("a dismissed btw must not be sendable")
	}
}

// Final review I-2: a btw note left in the queue after an abort/error must
// never start a main turn by itself.
func TestBTW_NoteNeverStartsTurnAfterAbort(t *testing.T) {
	m, be := doneBTW(t, true)
	m, _ = press(m, ctrlY)
	m.abortFn = func() {}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc}) // main aborted
	before := len(be.queries)
	runCmds(processQueueCmd(&m))
	if len(be.queries) != before {
		t.Fatal("the btw note started a main turn")
	}
	if !strings.HasPrefix(m.btwPending, "[btw] Q:") || m.msgQueue.Len() != 0 {
		t.Errorf("note not reclaimed: pending %q queue %d", m.btwPending, m.msgQueue.Len())
	}
}

// Final review M-3: /clear drops a pending btw note.
func TestBTW_ClearDropsPending(t *testing.T) {
	m, _ := doneBTW(t, false)
	m, _ = press(m, ctrlY)
	m, _ = m.applyCommandResult(commands.Result{ClearHistory: true, Display: commands.DisplayNone}, "clear")
	if m.btwPending != "" || m.btw != nil {
		t.Error("/clear must drop the btw session and its pending note")
	}
}
