package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/claude-code-go/internal/mcp"
	"github.com/tunsuy/claude-code-go/internal/session"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/pkg/types"
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

func TestSlashResume_BareListsSessions(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTUISession(t, cwd, "remember this question")
	m := modelWithWorkingDir(t, cwd)

	updated, cmd := m.handleSlashCommand("/resume")
	if cmd != nil {
		t.Error("bare /resume listing should be synchronous")
	}
	text := lastMessageText(t, updated)
	if !strings.Contains(text, sid) || !strings.Contains(text, "remember this question") {
		t.Errorf("expected session listing with id and snippet, got %q", text)
	}
}

func TestSlashResume_NoSessionsFriendlyMessage(t *testing.T) {
	m := modelWithWorkingDir(t, t.TempDir())
	updated, _ := m.handleSlashCommand("/resume")
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
