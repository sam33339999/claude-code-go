package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/mcp"
	"github.com/tunsuy/glaude/internal/msgqueue"
	"github.com/tunsuy/glaude/internal/session"
	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/pkg/types"
	utilfs "github.com/tunsuy/glaude/pkg/utils/fs"
)

// ---------------------------------------------------------------------------
// Fakes / helpers
// ---------------------------------------------------------------------------

// fakeMCPPool is a commands.MCPServerLister with canned connections.
type fakeMCPPool struct {
	conns []*mcp.ServerConnection
}

func (f *fakeMCPPool) GetAll() []*mcp.ServerConnection { return f.conns }

// tuiSessionEntry combines the JSONL envelope type with the message snapshot.
type tuiSessionEntry struct {
	Type string `json:"type"`
	types.SerializedMessage
}

// writeTUISession creates a session under cwd with one user message per text.
func writeTUISession(t *testing.T, cwd string, texts ...string) string {
	t.Helper()
	sid, mgr, err := session.New(cwd)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	for _, text := range texts {
		text := text
		entry := tuiSessionEntry{
			Type: string(types.RoleUser),
			SerializedMessage: types.SerializedMessage{
				Message: types.Message{
					Role: types.RoleUser,
					Content: []types.ContentBlock{
						{Type: types.ContentTypeText, Text: &text},
					},
				},
				SessionId: sid,
			},
		}
		if err := mgr.AppendEntry(entry); err != nil {
			t.Fatalf("AppendEntry: %v", err)
		}
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close: %v", err)
	}
	return string(sid)
}

// modelWithWorkingDir returns a test model whose app state points at cwd.
func modelWithWorkingDir(t *testing.T, cwd string) AppModel {
	t.Helper()
	m := newTestModel()
	m.appState.SetState(func(prev state.AppState) state.AppState {
		prev.WorkingDir = cwd
		return prev
	})
	return m
}

// lastMessageText extracts the text of the last message in the model.
func lastMessageText(t *testing.T, m AppModel) string {
	t.Helper()
	if len(m.messages) == 0 {
		t.Fatal("expected at least one message")
	}
	last := m.messages[len(m.messages)-1]
	if len(last.Content) == 0 || last.Content[0].Text == nil {
		t.Fatalf("expected text content in last message: %+v", last)
	}
	return *last.Content[0].Text
}

// ---------------------------------------------------------------------------
// /mcp — async status report via tea.Cmd
// ---------------------------------------------------------------------------

func TestSlashMCP_FetchesStatusViaCmd(t *testing.T) {
	m := newTestModel()
	m.mcpPool = &fakeMCPPool{conns: []*mcp.ServerConnection{
		{Name: "test-server", Status: mcp.StatusDisabled},
	}}
	// Cache-gate guard (CR-BF03-6): /mcp is display-only — the engine history
	// must be byte-identical across the whole fetch/display flow.
	m.queryEngine.SetMessages([]types.Message{newUserMessage("seed history")})
	engineBefore := m.queryEngine.GetMessages()

	updated, cmd := m.handleSlashCommand("/mcp")
	if cmd == nil {
		t.Fatal("expected /mcp to return an async fetch Cmd")
	}
	// No synchronous output: the report arrives as MCPStatusMsg.
	if len(updated.messages) != 0 {
		t.Errorf("expected no immediate messages, got %d", len(updated.messages))
	}

	msg := cmd()
	statusMsg, ok := msg.(MCPStatusMsg)
	if !ok {
		t.Fatalf("expected MCPStatusMsg, got %T", msg)
	}
	if !strings.Contains(statusMsg.Text, "test-server") {
		t.Errorf("expected server name in report, got %q", statusMsg.Text)
	}

	updated = applyMsg(updated, statusMsg)
	if !strings.Contains(lastMessageText(t, updated), "test-server") {
		t.Errorf("expected report displayed as system message, got %q", lastMessageText(t, updated))
	}
	if engineAfter := updated.queryEngine.GetMessages(); !reflect.DeepEqual(engineBefore, engineAfter) {
		t.Errorf("/mcp must not modify engine history (cache gate display-only): before %v, after %v",
			engineBefore, engineAfter)
	}
}

func TestSlashMCP_NilPoolFriendlyMessage(t *testing.T) {
	m := newTestModel() // mcpPool is nil
	_, cmd := m.handleSlashCommand("/mcp")
	if cmd == nil {
		t.Fatal("expected a Cmd even with nil pool")
	}
	statusMsg, ok := cmd().(MCPStatusMsg)
	if !ok {
		t.Fatalf("expected MCPStatusMsg")
	}
	if !strings.Contains(statusMsg.Text, "not available") {
		t.Errorf("expected friendly nil-pool message, got %q", statusMsg.Text)
	}
}

func TestUpdate_MCPStatusMsg_EmptyIgnored(t *testing.T) {
	m := newTestModel()
	updated := applyMsg(m, MCPStatusMsg{Text: ""})
	if len(updated.messages) != 0 {
		t.Error("empty MCP status report must not append a message")
	}
}

// ---------------------------------------------------------------------------
// /resume — listing and history loading
// ---------------------------------------------------------------------------

func TestSlashResume_BareOpensPicker(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTUISession(t, cwd, "remember this question")
	m := modelWithWorkingDir(t, cwd)

	pending, cmd := m.handleSlashCommand("/resume")
	if cmd == nil || pending.activeDialog == dialogResume {
		t.Fatal("bare /resume should load sessions in a background Cmd")
	}
	updated := runSlash(t, m, "/resume")
	if updated.activeDialog != dialogResume || updated.resumePicker == nil {
		t.Fatal("bare /resume should open the session picker")
	}
	view := stripANSI(updated.View().Content)
	if !strings.Contains(view, sid) || !strings.Contains(view, "remember this question") {
		t.Errorf("picker should show the selected session id and title, got:\n%s", view)
	}
	if len(updated.messages) != 0 {
		t.Error("the picker replaces the old text listing; nothing should be appended")
	}
}

func TestResumePicker_EnterResumesSelected(t *testing.T) {
	cwd := t.TempDir()
	older := writeTUISession(t, cwd, "older question")
	newer := writeTUISession(t, cwd, "newer question", "follow up")
	base := time.Now()
	setTUISessionModTime(t, cwd, older, base.Add(-2*time.Hour))
	setTUISessionModTime(t, cwd, newer, base.Add(-time.Minute))
	m := modelWithWorkingDir(t, cwd)

	m = runSlash(t, m, "/resume")
	if s, _ := m.resumePicker.selected(); s.ID != newer {
		t.Fatalf("picker should start on the newest session, got %s", s.ID)
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.activeDialog != dialogNone {
		t.Error("Enter should close the picker")
	}
	if m.sessionID != older {
		t.Errorf("Enter should resume the selected (older) session, got %q", m.sessionID)
	}
	if !strings.Contains(lastMessageText(t, m), "Resumed session") {
		t.Errorf("expected resume confirmation, got %q", lastMessageText(t, m))
	}
}

func TestResumePicker_HidesEmptySessions(t *testing.T) {
	cwd := t.TempDir()
	writeTUISession(t, cwd) // empty, e.g. a launch with no chat
	real := writeTUISession(t, cwd, "real chat")
	writeTUISession(t, cwd)
	m := modelWithWorkingDir(t, cwd)
	m = runSlash(t, m, "/resume")
	if m.resumePicker == nil || len(m.resumePicker.sessions) != 1 || m.resumePicker.sessions[0].ID != real {
		t.Fatalf("picker should list only the non-empty session, got %+v", m.resumePicker)
	}
}

func TestResumePicker_OnlyEmptySessionsFallsBackToText(t *testing.T) {
	cwd := t.TempDir()
	writeTUISession(t, cwd)
	m := modelWithWorkingDir(t, cwd)
	m = runSlash(t, m, "/resume")
	if m.activeDialog == dialogResume {
		t.Error("no picker when there is nothing to resume")
	}
	if !strings.Contains(lastMessageText(t, m), "Recent sessions") {
		t.Errorf("expected the text listing, got %q", lastMessageText(t, m))
	}
}

func TestResumePicker_EscCancels(t *testing.T) {
	cwd := t.TempDir()
	writeTUISession(t, cwd, "q")
	m := modelWithWorkingDir(t, cwd)
	m = runSlash(t, m, "/resume")
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.activeDialog != dialogNone || m.resumePicker != nil || m.sessionID != "" {
		t.Error("Esc should close the picker without resuming")
	}
}

func TestResumePicker_CurrentSessionMarked(t *testing.T) {
	cwd := t.TempDir()
	cur := writeTUISession(t, cwd, "this session")
	m := modelWithWorkingDir(t, cwd)
	m.sessionID = cur
	m = runSlash(t, m, "/resume")
	if !strings.Contains(stripANSI(m.View().Content), "current ·") {
		t.Error("the running session should be marked current")
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(lastMessageText(t, m), "Already in this session") {
		t.Errorf("choosing the current session should be a no-op notice, got %q", lastMessageText(t, m))
	}
}

func TestResumePicker_ScrollsAndFitsTerminal(t *testing.T) {
	sessions := make([]commands.SessionSummary, 25)
	for i := range sessions {
		sessions[i] = commands.SessionSummary{ID: fmt.Sprintf("id-%02d", i), Title: fmt.Sprintf("很長的中文標題 %d ", i) + strings.Repeat("字", 50), ModTime: time.Now()}
	}
	m := newTestModel()
	m.resumePicker = newResumePicker(sessions)
	m.activeDialog = dialogResume
	for i := 0; i < 20; i++ {
		m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	v := m.View()
	view := stripANSI(v.Content)
	if !strings.Contains(view, "id-20") || !strings.Contains(view, "more") {
		t.Errorf("picker should scroll to the cursor and show overflow hints:\n%s", view)
	}
	if lines := viewLines(v); lines > m.termHeight {
		t.Errorf("view %d lines exceeds terminal %d", lines, m.termHeight)
	}
	for _, l := range strings.Split(v.Content, "\n") {
		if displayWidth(l) > m.termWidth {
			t.Errorf("line wider than terminal: %d", displayWidth(l))
		}
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{
		10 * time.Second: "just now", 5 * time.Minute: "5m ago", 3 * time.Hour: "3h ago",
		30 * time.Hour: "yesterday", 4 * 24 * time.Hour: "4d ago", 20 * 24 * time.Hour: "Oct 30",
	}
	for d, want := range cases {
		if got := relativeTime(now.Add(-d), now); got != want {
			t.Errorf("%v: got %q want %q", d, got, want)
		}
	}
}

func TestSlashResume_NoSessionsFriendlyMessage(t *testing.T) {
	m := modelWithWorkingDir(t, t.TempDir())
	updated := runSlash(t, m, "/resume")
	if !strings.Contains(lastMessageText(t, updated), "No previous sessions") {
		t.Errorf("expected friendly empty message, got %q", lastMessageText(t, updated))
	}
}

func TestSlashResume_LoadsHistoryIntoEngine(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTUISession(t, cwd, "first question", "second question")
	m := modelWithWorkingDir(t, cwd)
	// Pre-existing history must be replaced, not appended to.
	m.messages = []types.Message{newUserMessage("stale history")}
	m.queryEngine.SetMessages(m.messages)

	updated, _ := m.handleSlashCommand("/resume " + sid)

	if updated.sessionID != sid {
		t.Errorf("expected sessionID %q, got %q", sid, updated.sessionID)
	}
	// Two loaded messages + one resume confirmation system message.
	if len(updated.messages) != 3 {
		t.Fatalf("expected 3 messages (2 loaded + confirmation), got %d", len(updated.messages))
	}
	if !strings.Contains(lastMessageText(t, updated), "Resumed session") {
		t.Errorf("expected resume confirmation, got %q", lastMessageText(t, updated))
	}
	fe := updated.queryEngine.(*fakeEngine)
	if len(fe.messages) != 2 {
		t.Fatalf("expected engine history replaced with 2 messages, got %d", len(fe.messages))
	}
	if fe.messages[0].Content[0].Text == nil || *fe.messages[0].Content[0].Text != "first question" {
		t.Errorf("unexpected engine history: %+v", fe.messages)
	}
}

func TestSlashResume_RejectedWhileLoading(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTUISession(t, cwd, "question")
	m := modelWithWorkingDir(t, cwd)
	m.isLoading = true
	m.messages = []types.Message{newUserMessage("in-flight conversation")}
	m.queryEngine.SetMessages(m.messages)

	updated, _ := m.handleSlashCommand("/resume " + sid)

	if updated.sessionID != "" {
		t.Errorf("resume must be rejected while loading, sessionID = %q", updated.sessionID)
	}
	fe := updated.queryEngine.(*fakeEngine)
	if len(fe.messages) != 1 {
		t.Errorf("engine history must be untouched while loading, got %d messages", len(fe.messages))
	}
	if !strings.Contains(lastMessageText(t, updated), "Cannot resume") {
		t.Errorf("expected rejection notice, got %q", lastMessageText(t, updated))
	}
}

func TestSlashResume_UnknownIDShowsError(t *testing.T) {
	m := modelWithWorkingDir(t, t.TempDir())
	updated, _ := m.handleSlashCommand("/resume nope")
	text := lastMessageText(t, updated)
	if !strings.Contains(text, "Error:") || !strings.Contains(text, "not found") {
		t.Errorf("expected error display for unknown session, got %q", text)
	}
	if updated.sessionID != "" {
		t.Error("failed resume must not change sessionID")
	}
}

// ---------------------------------------------------------------------------
// Regression: /clear's legacy NewMessages value must not break resume handling
// ---------------------------------------------------------------------------

func TestSlashClear_StillClearsWithLegacyNewMessages(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{newUserMessage("hello")}
	m.queryEngine.SetMessages(m.messages)

	updated, _ := m.handleSlashCommand("/clear")
	if len(updated.messages) != 1 { // only the "Conversation cleared." notice
		t.Fatalf("expected history cleared, got %d messages", len(updated.messages))
	}
	fe := updated.queryEngine.(*fakeEngine)
	if len(fe.messages) != 0 {
		t.Errorf("expected engine history cleared, got %d", len(fe.messages))
	}
}

// ---------------------------------------------------------------------------
// New() constructor accepts the MCP pool injection
// ---------------------------------------------------------------------------

func TestNew_AcceptsMCPPool(t *testing.T) {
	pool := &fakeMCPPool{}
	appStore := state.NewAppStateStore(state.AppState{
		WorkingDir:    t.TempDir(),
		MainLoopModel: state.ModelSetting{ModelID: "test-model"},
	})
	model := New(newFakeEngine(), appStore, false, true, nil, nil, nil, nil, nil, nil, nil, pool, nil, nil, nil)
	m, ok := model.(AppModel)
	if !ok {
		t.Fatalf("expected AppModel, got %T", model)
	}
	if m.mcpPool != pool {
		t.Error("expected injected MCP pool to be stored on the model")
	}
}

// Ensure the fetch Cmd type satisfies tea.Cmd at compile time.
var _ tea.Cmd = fetchMCPStatusCmd(nil)

// setTUISessionModTime sets a session file's mtime (listing order).
func setTUISessionModTime(t *testing.T, cwd, sid string, mt time.Time) {
	t.Helper()
	hash, err := utilfs.ProjectHash(cwd)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cwd, ".glaude", "projects", hash, sid+".jsonl")
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

// runSlash runs a slash command, executing any async Cmd it returns and
// applying the resulting message (as the Bubble Tea runtime would).
func runSlash(t *testing.T, m AppModel, text string) AppModel {
	t.Helper()
	m, cmd := m.handleSlashCommand(text)
	if res := findMsg[asyncCommandResultMsg](cmd); res != nil {
		m = applyMsg(m, *res)
	}
	return m
}

func TestResumePicker_FitsShortAndNarrowTerminals(t *testing.T) {
	sessions := make([]commands.SessionSummary, 15)
	old := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := range sessions {
		sessions[i] = commands.SessionSummary{
			ID:      fmt.Sprintf("1795071132759-77527df52b86bf%02d", i),
			Title:   "很長的中文標題 " + strings.Repeat("字", 40),
			ModTime: old, MessageCount: 1234, Current: i == 0,
		}
	}
	for _, size := range [][2]int{{40, 10}, {40, 12}, {60, 13}, {80, 24}, {120, 40}} {
		m := newTestModel()
		m = applyMsg(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m.resumePicker = newResumePicker(sessions)
		m.activeDialog = dialogResume
		for i := 0; i < 9; i++ {
			m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown})
		}
		v := m.View()
		if lines := viewLines(v); lines > size[1] {
			t.Errorf("%dx%d: view is %d lines", size[0], size[1], lines)
		}
		for _, l := range strings.Split(v.Content, "\n") {
			if displayWidth(l) > size[0] {
				t.Errorf("%dx%d: line is %d cells", size[0], size[1], displayWidth(l))
			}
		}
		if !strings.Contains(stripANSI(v.Content), "❯") {
			t.Errorf("%dx%d: the selected row must stay visible", size[0], size[1])
		}
	}
}

func TestResumePicker_RejectedWhileLoadingShowsListing(t *testing.T) {
	m := newTestModel()
	m.isLoading = true
	m, _ = m.applyCommandResult(commands.Result{
		Text: "Recent sessions:\n  abc", OpenDialog: "resume",
		Sessions: []commands.SessionSummary{{ID: "abc", MessageCount: 2}},
	}, "resume")
	if m.activeDialog == dialogResume {
		t.Error("picker must not open while a query runs")
	}
	if !strings.Contains(lastMessageText(t, m), "Recent sessions") {
		t.Error("the text listing should still be shown when the picker is refused")
	}
}

func TestResumePicker_DefersQueueDrainUntilClosed(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.resumePicker = newResumePicker([]commands.SessionSummary{{ID: "x", MessageCount: 1}})
	m.activeDialog = dialogResume
	updated, cmd := m.Update(AgentStatusMsg{TaskID: "a1", Status: AgentCompleted, Result: "done"})
	m = updated.(AppModel)
	if m.isLoading {
		t.Fatal("an agent wake-up must not start a query while the picker is open")
	}
	_ = cmd
	if m.msgQueue.Len() != 1 {
		t.Fatalf("the notification should stay queued, len=%d", m.msgQueue.Len())
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(AppModel)
	if !m.isLoading || m.msgQueue.Len() != 0 {
		t.Error("closing the picker should drain the deferred queue")
	}
}
