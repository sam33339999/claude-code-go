package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/msgqueue"
	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// btwEngine records btw queries and hands back a controllable channel; the
// main history (embedded fakeEngine.messages) is never touched by Query.
type btwEngine struct {
	*fakeEngine
	queries  []engine.QueryParams
	ch       chan engine.Msg
	setCalls int
}

func (b *btwEngine) Query(ctx context.Context, p engine.QueryParams) (<-chan engine.Msg, error) {
	b.queries = append(b.queries, p)
	b.ch = make(chan engine.Msg, 16)
	return b.ch, nil
}
func (b *btwEngine) SetMessages(m []types.Message) { b.setCalls++; b.fakeEngine.SetMessages(m) }

func newBTWModel(t *testing.T, target func(string) (engine.MainTarget, error)) (AppModel, *btwEngine) {
	t.Helper()
	be := &btwEngine{fakeEngine: newFakeEngine()}
	be.fakeEngine.messages = []types.Message{newUserMessage("earlier question")}
	be.fakeEngine.main = engine.MainTarget{ProviderID: "main", Model: "main-model", MaxTokens: 8192}
	be.fakeEngine.model = "main-model"
	appStore := state.NewAppStateStore(state.AppState{WorkingDir: "/tmp", MainLoopModel: state.ModelSetting{ModelID: "main-model"}})
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	m := newAppModel(be, appStore, false, true, reg, nil, nil, nil, nil, msgqueue.NewMessageQueue(), nil, nil, nil, nil, nil, nil, target, nil, nil, nil)
	m.termWidth, m.termHeight = 100, 30
	m.memdirPrompt = "project memory"
	return m, be
}

func submit(m AppModel, text string) (AppModel, tea.Cmd) {
	m.input = m.input.SetValue(text)
	return press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
}

// feed delivers msgs on the btw channel and runs the listener cmds.
func feed(m AppModel, be *btwEngine, cmd tea.Cmd, msgs ...engine.Msg) (AppModel, tea.Cmd) {
	for _, em := range msgs {
		be.ch <- em
		updated, next := m.Update(cmd())
		m, cmd = updated.(AppModel), next
	}
	return m, cmd
}

func closeBTW(m AppModel, be *btwEngine, cmd tea.Cmd) AppModel {
	close(be.ch)
	updated, _ := m.Update(cmd())
	return updated.(AppModel)
}

func btwBlock(m AppModel) string {
	for _, msg := range m.messages {
		if msg.ID == btwMessageID && len(msg.Content) > 0 && msg.Content[0].Text != nil {
			return *msg.Content[0].Text
		}
	}
	return ""
}

func TestBTW_ParseArgs(t *testing.T) {
	cases := []struct{ in, tier, q string }{{"--tier pro why?", "pro", "why?"}, {"why is it slow?", "", "why is it slow?"}}
	for _, c := range cases {
		tier, q, err := parseBTW(c.in)
		if err != nil || tier != c.tier || q != c.q {
			t.Errorf("parseBTW(%q) = %q %q %v", c.in, tier, q, err)
		}
	}
	for _, bad := range []string{"", "  ", "--tier", "--tier pro"} {
		if _, _, err := parseBTW(bad); err == nil {
			t.Errorf("parseBTW(%q) want error", bad)
		}
	}
}

func TestBTW_StartsImmediatelyWhileLoading(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m.isLoading = true
	m, _ = submit(m, "/btw what is the backoff cap?")
	if m.msgQueue.Len() != 0 {
		t.Fatal("/btw was queued instead of starting")
	}
	if len(be.queries) != 1 {
		t.Fatalf("queries = %d", len(be.queries))
	}
	p := be.queries[0]
	if !p.NoWriteBack || p.QuerySource != "btw" || p.ToolFilter == nil || !p.NoCompaction || !p.NonInteractivePermissions || p.MaxTurns != 10 || p.CompactTracker == nil {
		t.Errorf("btw params = %+v", p)
	}
	if p.ToolUseContext == nil || !p.ToolUseContext.TodoAppendOnly {
		t.Error("TodoWrite must be append-only in btw")
	}
	last := p.Messages[len(p.Messages)-1]
	if !strings.Contains(*last.Content[0].Text, "what is the backoff cap?") {
		t.Errorf("question missing: %+v", last)
	}
}

func TestBTW_ToolFilter(t *testing.T) {
	m, be := newBTWModel(t, nil)
	_, _ = submit(m, "/btw q")
	if be.queries[0].ToolFilter == nil {
		t.Fatal("no ToolFilter")
	}
	allowed := btwAllowTool
	if !allowed("Read", true) || !allowed("TodoWrite", false) {
		t.Error("read-only tools and TodoWrite must be allowed")
	}
	for _, name := range []string{"AskUserQuestion", "EnterPlanMode", "ExitPlanMode", "Sleep", "Agent", "SendMessage"} {
		if allowed(name, true) {
			t.Errorf("%s must be refused even if read-only", name)
		}
	}
	if allowed("Write", false) {
		t.Error("Write must be refused")
	}
}

func TestBTW_UsesSnapshotWhileRunning(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m.isLoading = true
	snapClient := &stubClient{}
	_ = snapClient
	m.mainUseCtx = &tools.UseContext{CacheParams: &engine.CacheSafeParams{
		SystemPrompt:    engine.SystemPrompt{Parts: []engine.SystemPromptPart{{Text: "snap system"}}},
		ContextMessages: []types.Message{newUserMessage("snap history")},
		Model:           "snap-model", Client: snapClient, MaxTokens: 4096,
	}}
	m, _ = submit(m, "/btw q")
	p := be.queries[0]
	if p.SystemPrompt.Parts[0].Text != "snap system" || *p.Messages[0].Content[0].Text != "snap history" || p.Model != "snap-model" || p.Client != snapClient {
		t.Errorf("params not from snapshot: %+v", p)
	}
}

func TestBTW_UsesHistoryWhenIdle(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m.mainUseCtx = &tools.UseContext{CacheParams: &engine.CacheSafeParams{ContextMessages: []types.Message{newUserMessage("stale")}}}
	m, _ = submit(m, "/btw q")
	p := be.queries[0]
	if *p.Messages[0].Content[0].Text != "earlier question" || p.SystemPrompt.Parts[0].Text != "project memory" || p.Model != "main-model" {
		t.Errorf("idle params = %+v", p)
	}
}

func TestBTW_SecondRejected(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m, _ = submit(m, "/btw one")
	m, _ = submit(m, "/btw two")
	if len(be.queries) != 1 || !strings.Contains(lastText(m), "already running") {
		t.Errorf("queries %d, notice %q", len(be.queries), lastText(m))
	}
}

func TestBTW_StreamsIntoBlock(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m, cmd := submit(m, "/btw q")
	m, cmd = feed(m, be, cmd, engine.Msg{Type: engine.MsgTypeStreamText, TextDelta: "a"}, engine.Msg{Type: engine.MsgTypeStreamText, TextDelta: "b"})
	if !strings.Contains(btwBlock(m), "ab") {
		t.Errorf("block = %q", btwBlock(m))
	}
	m = closeBTW(m, be, cmd)
	if m.btw.status != btwDone || !strings.Contains(btwBlock(m), "ctrl+y") {
		t.Errorf("status %v block %q", m.btw.status, btwBlock(m))
	}
}

func TestBTW_NotInEngineHistory(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m, cmd := submit(m, "/btw q")
	m, cmd = feed(m, be, cmd, engine.Msg{Type: engine.MsgTypeStreamText, TextDelta: "answer"})
	_ = closeBTW(m, be, cmd)
	if be.setCalls != 0 || len(be.fakeEngine.messages) != 1 {
		t.Errorf("engine history touched: set=%d len=%d", be.setCalls, len(be.fakeEngine.messages))
	}
}

func TestBTW_TierResolveError(t *testing.T) {
	m, be := newBTWModel(t, func(string) (engine.MainTarget, error) { return engine.MainTarget{}, errors.New("tier max is unbound") })
	m, _ = submit(m, "/btw --tier max q")
	if len(be.queries) != 0 || !strings.Contains(lastText(m), "unbound") {
		t.Errorf("queries %d notice %q", len(be.queries), lastText(m))
	}
}

func TestBTW_TierTarget(t *testing.T) {
	c := &stubClient{}
	m, be := newBTWModel(t, func(ref string) (engine.MainTarget, error) {
		return engine.MainTarget{Client: c, ProviderID: "zai", Model: "glm", ContextWindow: 131072, MaxTokens: 8192}, nil
	})
	m, _ = submit(m, "/btw --tier lite q")
	if p := be.queries[0]; p.Client != c || p.Model != "glm" || p.ContextWindow != 131072 {
		t.Errorf("tier params = %+v", p)
	}
}

func TestBTW_TooLargeRejected(t *testing.T) {
	m, be := newBTWModel(t, nil)
	be.fakeEngine.messages = []types.Message{newUserMessage(strings.Repeat("x", 900_000))}
	m, _ = submit(m, "/btw q")
	if len(be.queries) != 0 || !strings.Contains(lastText(m), "too large") {
		t.Errorf("queries %d notice %q", len(be.queries), lastText(m))
	}
}

func TestBTW_EscCancelsBTWFirst(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m, _ = submit(m, "/btw q")
	m.isLoading = true
	aborted := false
	m.abortFn = func() { aborted = true }
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.btw.status != btwCancelled || aborted || !m.isLoading {
		t.Errorf("first Esc: btw %v, main aborted %v", m.btw.status, aborted)
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.isLoading {
		t.Error("second Esc must interrupt the main loop")
	}
}

func TestBTW_MainTurnEndsDuringBTW(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m.isLoading = true
	m, cmd := submit(m, "/btw q")
	before := be.queries[0]
	updated, _ := m.Update(StreamDoneMsg{})
	m = updated.(AppModel)
	m, _ = feed(m, be, cmd, engine.Msg{Type: engine.MsgTypeStreamText, TextDelta: "still"})
	if len(be.queries) != 1 || be.queries[0].Model != before.Model || !strings.Contains(btwBlock(m), "still") {
		t.Errorf("btw disturbed by main turn end: %q", btwBlock(m))
	}
}

func TestBTW_ClearDuringBTW(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m, _ = submit(m, "/btw q")
	cancelled := false
	m.btw.cancel = func() { cancelled = true }
	m, _ = m.applyCommandResult(commands.Result{ClearHistory: true, Display: commands.DisplayNone}, "clear")
	if !cancelled || m.btw != nil {
		t.Errorf("btw not cancelled by /clear (cancelled %v)", cancelled)
	}
}

func TestBTW_ErrorShownInBlock(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m, cmd := submit(m, "/btw q")
	m, _ = feed(m, be, cmd, engine.Msg{Type: engine.MsgTypeError, Err: errors.New("rate limited")})
	if m.btw.status != btwFailed || !strings.Contains(btwBlock(m), "rate limited") || m.isLoading {
		t.Errorf("status %v block %q", m.btw.status, btwBlock(m))
	}
}

// stubClient is an identity-only api.Client.
type stubClient struct{}

func (stubClient) Stream(context.Context, *api.MessageRequest) (api.StreamReader, error) {
	return nil, errors.New("unused")
}
func (stubClient) Complete(context.Context, *api.MessageRequest) (*api.MessageResponse, error) {
	return nil, errors.New("unused")
}
