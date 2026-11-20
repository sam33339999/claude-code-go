package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/tools/shell"
)

func lastDisplayText(m AppModel) string {
	if len(m.messages) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, b := range m.messages[len(m.messages)-1].Content {
		if b.Text != nil {
			sb.WriteString(*b.Text)
		}
	}
	return sb.String()
}

// runBash submits a ! command and delivers its result to the model.
func runBash(t *testing.T, m AppModel, text string) AppModel {
	t.Helper()
	m, cmd := submit(m, text)
	if cmd == nil {
		t.Fatal("expected a command for " + text)
	}
	msg, ok := cmd().(bashResultMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want bashResultMsg", msg)
	}
	updated, _ := m.Update(msg)
	return updated.(AppModel)
}

func TestParseBang(t *testing.T) {
	cases := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"!ls", "ls", true},
		{"!  git status ", "git status", true},
		{"!", "", true},
		{"ls", "", false},
	}
	for _, c := range cases {
		got, ok := parseBang(c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("parseBang(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestBashSubmit_EmptyIgnored(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m, cmd := submit(m, "!  ")
	if cmd != nil {
		t.Error("empty ! must not run anything")
	}
	if len(m.bashPending) != 0 || m.isLoading || len(be.queries) != 0 {
		t.Errorf("pending=%d loading=%v queries=%d", len(m.bashPending), m.isLoading, len(be.queries))
	}
}

func TestFormatBashContext(t *testing.T) {
	ctx := formatBashContext("ls", shell.RunResult{Stdout: "a"})
	if !strings.Contains(ctx, "<bash-input>ls</bash-input>") || !strings.Contains(ctx, "<bash-stdout>a</bash-stdout>") {
		t.Errorf("ctx = %q", ctx)
	}
	if strings.Contains(ctx, "<bash-stderr>") {
		t.Error("empty stderr must be omitted")
	}
	failed := formatBashContext("x", shell.RunResult{Stderr: "boom", ExitCode: 2})
	if !strings.Contains(failed, "<bash-stderr>boom</bash-stderr>") || !strings.Contains(failed, "exit code 2") {
		t.Errorf("failed ctx = %q", failed)
	}
}

func TestFormatBashContext_TruncatesRunes(t *testing.T) {
	ctx := formatBashContext("big", shell.RunResult{Stdout: strings.Repeat("中", 40_000)})
	if !utf8.ValidString(ctx) {
		t.Fatal("truncated context is not valid UTF-8")
	}
	if n := strings.Count(ctx, "中"); n > bashContextLimit {
		t.Errorf("kept %d runes, limit %d", n, bashContextLimit)
	}
	if !strings.Contains(ctx, "truncated") {
		t.Error("missing truncation note")
	}
}

func TestBashResult_AppendsPendingAndDisplay(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m = runBash(t, m, "!echo hi")
	if len(m.bashPending) != 1 {
		t.Fatalf("pending = %d, want 1", len(m.bashPending))
	}
	if !strings.Contains(lastDisplayText(m), "hi") {
		t.Errorf("display = %q", lastDisplayText(m))
	}
	if len(be.queries) != 0 {
		t.Error("! must not start a model query")
	}
	if !strings.Contains(m.View().Content, "attached") {
		t.Error("pending hint not shown")
	}
}

func TestBashPending_Accumulates(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m = runBash(t, m, "!echo one")
	m = runBash(t, m, "!echo two")
	out := m.takeBashPending("q")
	i1, i2 := strings.Index(out, "echo one"), strings.Index(out, "echo two")
	if i1 < 0 || i2 < 0 || i1 > i2 || !strings.HasSuffix(out, "q") {
		t.Errorf("out = %q", out)
	}
	if len(m.bashPending) != 0 {
		t.Error("pending not cleared")
	}
}

func TestBashPending_AttachedViaStartQuery(t *testing.T) {
	m, be := newBTWModel(t, nil)
	m = runBash(t, m, "!echo hi")
	m, cmd := submit(m, "explain")
	runCmds(cmd)
	if len(be.queries) == 0 {
		t.Fatal("query not started")
	}
	msgs := be.queries[len(be.queries)-1].Messages
	text := *msgs[len(msgs)-1].Content[0].Text
	if !strings.Contains(text, "<bash-input>echo hi</bash-input>") || !strings.HasSuffix(text, "explain") {
		t.Errorf("prompt = %q", text)
	}
	if len(m.bashPending) != 0 {
		t.Error("pending not cleared after attaching")
	}
}

func TestBashResult_StaleAfterClearDropped(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m, cmd := submit(m, "!echo late")
	m, _ = m.applyCommandResult(commands.Result{ClearHistory: true, Display: commands.DisplayNone}, "clear")
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if len(m.bashPending) != 0 || len(m.messages) != 0 {
		t.Errorf("stale result leaked: pending=%d messages=%d", len(m.bashPending), len(m.messages))
	}
}

func TestBashEsc_CancelsRunning(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.isLoading = true
	m, cmd := submit(m, "!sleep 5")
	if m.bashCancel == nil {
		t.Fatal("running command has no cancel")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if !m.isLoading {
		t.Error("Esc on a running ! must not abort the main query")
	}
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if !strings.Contains(lastDisplayText(m), "canceled") {
		t.Errorf("display = %q", lastDisplayText(m))
	}
	if len(m.bashPending) != 0 {
		t.Error("canceled command must not be attached")
	}
}

func TestBashEsc_DiscardsPending(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m = runBash(t, m, "!echo hi")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if len(m.bashPending) != 0 {
		t.Error("Esc must discard pending shell output")
	}
}
