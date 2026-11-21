package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/pkg/types"
)

// --- 1. Display-width aware truncation ---

func TestTruncateWidth_CJKNeverSplitsRunes(t *testing.T) {
	s := "中文路徑與輸出結果非常長的一段文字🙂emoji"
	for w := 1; w <= displayWidth(s)+2; w++ {
		got := truncateWidth(s, w)
		if !utf8.ValidString(got) {
			t.Fatalf("width %d: invalid UTF-8 %q", w, got)
		}
		if gw := displayWidth(got); gw > w {
			t.Fatalf("width %d: result occupies %d cells: %q", w, gw, got)
		}
	}
	if truncateWidth("short", 10) != "short" {
		t.Error("strings that fit must be returned unchanged")
	}
	if truncateWidth("abc", 0) != "" {
		t.Error("non-positive width yields empty string")
	}
}

func TestTruncateWidth_PreservesANSI(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Render("你好世界你好世界")
	got := truncateWidth(styled, 7)
	if displayWidth(got) > 7 {
		t.Errorf("styled truncation too wide: %d", displayWidth(got))
	}
	if !strings.Contains(stripANSI(got), "…") {
		t.Errorf("expected ellipsis, got %q", stripANSI(got))
	}
}

func TestTruncateLeftWidth_KeepsTail(t *testing.T) {
	p := "/Users/someone/專案/很長的目錄名稱/glaude"
	got := truncateLeftWidth(p, 16)
	if displayWidth(got) > 16 {
		t.Errorf("too wide: %d %q", displayWidth(got), got)
	}
	if !strings.HasSuffix(got, "glaude") || !strings.HasPrefix(got, "…") {
		t.Errorf("expected …<tail>, got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("invalid UTF-8: %q", got)
	}
}

func TestToolResult_CJKOutputStaysValid(t *testing.T) {
	line := strings.Repeat("測試輸出", 60)
	blk := types.ContentBlock{Type: types.ContentTypeToolResult,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr(line)}}}
	out := renderToolResult("Bash", nil, blk, false, renderOptions{width: 60, theme: DefaultDarkTheme})
	if !utf8.ValidString(out) {
		t.Fatal("tool output must remain valid UTF-8 after truncation")
	}
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if displayWidth(l) > 60 {
			t.Errorf("line exceeds terminal width: %d", displayWidth(l))
		}
	}
}

// --- 2. Deterministic argument order ---

func TestSummariseToolInput_SortedAndStable(t *testing.T) {
	in := map[string]any{"zeta": "1", "alpha": "2", "mid": "3", "beta": "4"}
	first := summariseToolInput(in, 200)
	if first != "alpha=2, beta=4, mid=3, zeta=1" {
		t.Errorf("expected sorted keys, got %q", first)
	}
	for i := 0; i < 50; i++ {
		if got := summariseToolInput(in, 200); got != first {
			t.Fatalf("summary changed between calls: %q vs %q", got, first)
		}
	}
}

// --- 4. Per-tool presentation ---

func TestToolArgSummary_PerTool(t *testing.T) {
	cwd := "/repo"
	cases := []struct {
		name  string
		input map[string]any
		want  string
	}{
		{"Bash", map[string]any{"command": "git status\necho hi", "timeout": float64(5)}, "git status"},
		{"Read", map[string]any{"file_path": "/repo/internal/tui/view.go"}, "internal/tui/view.go"},
		{"Read", map[string]any{"file_path": "/repo/a.go", "offset": float64(10), "limit": float64(5)}, "a.go · lines 10-14"},
		{"Edit", map[string]any{"file_path": "/elsewhere/x.go", "old_string": "a", "new_string": "b"}, "/elsewhere/x.go"},
		{"Grep", map[string]any{"pattern": "func main", "path": "/repo/cmd"}, `pattern: "func main", path: cmd`},
		{"Glob", map[string]any{"pattern": "**/*.go"}, `pattern: "**/*.go"`},
		{"WebFetch", map[string]any{"url": "https://example.com", "prompt": "x"}, "https://example.com"},
		{"TodoWrite", map[string]any{"todos": []any{}}, ""},
	}
	for _, c := range cases {
		if got := toolArgSummary(c.name, c.input, cwd, 80); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func resultBlock(text string, isErr bool) types.ContentBlock {
	blk := types.ContentBlock{Type: types.ContentTypeToolResult,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr(text)}}}
	if isErr {
		blk.IsError = &isErr
	}
	return blk
}

func TestRenderToolResult_Summaries(t *testing.T) {
	opts := renderOptions{width: 80, theme: DefaultDarkTheme, cwd: "/repo"}
	cases := []struct {
		name  string
		input map[string]any
		text  string
		want  string
	}{
		{"Read", nil, "1\ta\n2\tb\n3\tc\n", "Read 3 lines"},
		{"Grep", nil, "a.go:1:x\nb.go:2:y\n", "Found 2 matches"},
		{"Grep", map[string]any{"output_mode": "files_with_matches"}, "a.go\nb.go\nc.go\n", "Found 3 files"},
		{"Grep", nil, "No matches found.", "No matches found"},
		{"Glob", nil, "a.go\nb.go\n", "Found 2 files"},
		{"Glob", nil, "No files found matching the pattern.", "No files found"},
		{"TodoWrite", nil, "ok", "Updated todos"},
	}
	for _, c := range cases {
		out := stripANSI(renderToolResult(c.name, c.input, resultBlock(c.text, false), false, opts))
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q in %q", c.name, c.want, out)
		}
		if renderedLines(out) != 1 {
			t.Errorf("%s summary should be one line, got %q", c.name, out)
		}
	}
}

func TestRenderToolResult_ErrorShowsOutput(t *testing.T) {
	opts := renderOptions{width: 80, theme: DefaultDarkTheme}
	out := stripANSI(renderToolResult("Read", nil, resultBlock("file does not exist: x", true), false, opts))
	if !strings.Contains(out, "file does not exist") {
		t.Errorf("errors must show the tool's message, got %q", out)
	}
}

func TestRenderToolResult_GenericCollapsesAndExpands(t *testing.T) {
	opts := renderOptions{width: 80, theme: DefaultDarkTheme}
	text := "l1\nl2\nl3\nl4\nl5\nl6\n"
	collapsed := stripANSI(renderToolResult("Bash", nil, resultBlock(text, false), false, opts))
	if strings.Contains(collapsed, "l6") || !strings.Contains(collapsed, "+2 lines (ctrl+o to expand)") {
		t.Errorf("collapsed output wrong: %q", collapsed)
	}
	expanded := stripANSI(renderToolResult("Bash", nil, resultBlock(text, false), true, opts))
	if !strings.Contains(expanded, "l6") || strings.Contains(expanded, "ctrl+o") {
		t.Errorf("expanded output wrong: %q", expanded)
	}
}

func TestRenderEditDiff(t *testing.T) {
	opts := renderOptions{width: 80, theme: DefaultDarkTheme, cwd: "/repo"}
	input := map[string]any{
		"file_path":  "/repo/main.go",
		"old_string": "a\nb\nc",
		"new_string": "a\nB\nc\nd",
	}
	out := stripANSI(renderToolResult("Edit", input, resultBlock("ok", false), false, opts))
	for _, want := range []string{"Updated main.go with 2 additions and 1 removal", "- b", "+ B", "+ d", "  a"} {
		if !strings.Contains(out, want) {
			t.Errorf("diff missing %q:\n%s", want, out)
		}
	}
}

func TestLineDiff(t *testing.T) {
	ops := lineDiff([]string{"a", "b", "c"}, []string{"a", "x", "c"})
	var got []string
	for _, op := range ops {
		got = append(got, string(op.kind)+op.text)
	}
	want := " a -b +x  c"
	if strings.Join(got, " ") != want {
		t.Errorf("got %q want %q", strings.Join(got, " "), want)
	}
	// Oversized inputs degrade to remove-all/add-all but stay complete.
	big := make([]string, 600)
	for i := range big {
		big[i] = itoa(i)
	}
	ops = lineDiff(big, big[:500])
	if len(ops) != 1100 {
		t.Errorf("fallback diff should list every line, got %d ops", len(ops))
	}
}

func TestRenderWriteResult(t *testing.T) {
	opts := renderOptions{width: 80, theme: DefaultDarkTheme, cwd: "/repo"}
	input := map[string]any{"file_path": "/repo/n.txt", "content": "1\n2\n3\n4\n5\n6"}
	out := stripANSI(renderToolResult("Write", input, resultBlock("ok", false), false, opts))
	if !strings.Contains(out, "Wrote 6 lines to n.txt") || !strings.Contains(out, "+2 lines") {
		t.Errorf("unexpected write result: %q", out)
	}
}

// --- 3. Render cache & stream throttling ---

func toolTurn(id, name string, input map[string]any) types.Message {
	return types.Message{Role: types.RoleAssistant, Content: []types.ContentBlock{
		{Type: types.ContentTypeToolUse, ID: strPtr(id), Name: strPtr(name), Input: input},
	}}
}

func toolResultTurn(id, text string) types.Message {
	blk := resultBlock(text, false)
	blk.ToolUseID = strPtr(id)
	return types.Message{Role: types.RoleUser, Content: []types.ContentBlock{blk}}
}

func TestRenderCache_ReusesAndInvalidates(t *testing.T) {
	msgs := []types.Message{
		toolTurn("t1", "Bash", map[string]any{"command": "ls"}),
	}
	opts := renderOptions{width: 80, theme: DefaultDarkTheme, expanded: map[string]bool{}}
	cache := newMessageRenderCache()

	first := renderMessageList(msgs, opts, cache)
	if len(cache.entries) != 1 {
		t.Fatalf("expected 1 cache entry, got %d", len(cache.entries))
	}
	// Poison the entry: a cache hit must return it verbatim.
	cache.entries[0].out = "CACHED"
	if got := renderMessageList(msgs, opts, cache); got != "CACHED" {
		t.Errorf("expected cache hit, got %q", got)
	}

	// The tool resolving (a later message) must invalidate the tool_use entry.
	msgs = append(msgs, toolResultTurn("t1", "a\nb\nc\nd\ne\nf"))
	resolved := renderMessageList(msgs, opts, cache)
	if strings.Contains(resolved, "CACHED") || resolved == first {
		t.Errorf("resolved tool must re-render, got %q", resolved)
	}

	// Expanding (ctrl+o) must invalidate too.
	cache.entries[0].out = "CACHED"
	opts.expanded["t1"] = true
	if got := renderMessageList(msgs, opts, cache); strings.Contains(got, "CACHED") {
		t.Error("expansion change must re-render")
	}

	// Width change invalidates.
	cache.entries[0].out = "CACHED"
	opts.width = 100
	if got := renderMessageList(msgs, opts, cache); strings.Contains(got, "CACHED") {
		t.Error("width change must re-render")
	}

	// Replaced history (e.g. /resume) at the same index must not hit.
	cache.entries[0].out = "CACHED"
	other := []types.Message{newUserMessage("different")}
	if got := renderMessageList(other, opts, cache); strings.Contains(got, "CACHED") {
		t.Error("a different message at the same index must re-render")
	}
}

func TestStreamToken_ThrottlesRender(t *testing.T) {
	m := newTestModel()
	m.streamCh = make(chan engine.Msg, 4)

	updated, cmd1 := m.Update(StreamTokenMsg{Delta: "hello "})
	m = updated.(AppModel)
	if !m.streamFlushPending || cmd1 == nil {
		t.Fatal("first token must schedule a flush")
	}
	if strings.Contains(stripANSI(m.viewport.View()), "hello") {
		t.Error("first token should not render synchronously (coalesced)")
	}
	updated, _ = m.Update(StreamTokenMsg{Delta: "world"})
	m = updated.(AppModel)
	if m.streamingText != "hello world" {
		t.Errorf("tokens must accumulate, got %q", m.streamingText)
	}

	m = applyMsg(m, streamFlushMsg{})
	if m.streamFlushPending {
		t.Error("flush must clear the pending flag")
	}
	if !strings.Contains(stripANSI(m.viewport.View()), "hello world") {
		t.Errorf("flush must render the streamed text, got %q", stripANSI(m.viewport.View()))
	}
}

// --- 6. Spinner ---

func TestSpinner_VerbTokensAndHint(t *testing.T) {
	s := newSpinner()
	out := stripANSI(s.ViewStatus(DefaultDarkTheme, spinnerStatus{tool: "Read", outputTokens: 1500}))
	for _, want := range []string{"Reading…", "↓ 1.5k tokens", "esc to interrupt"} {
		if !strings.Contains(out, want) {
			t.Errorf("spinner missing %q: %q", want, out)
		}
	}
	idle := stripANSI(s.ViewStatus(DefaultDarkTheme, spinnerStatus{}))
	if !strings.Contains(idle, "Thinking…") || strings.Contains(idle, "tokens") {
		t.Errorf("idle spinner wrong: %q", idle)
	}
}

func TestUpdate_ToolStartSetsSpinnerTool(t *testing.T) {
	m := newTestModel()
	m.streamCh = make(chan engine.Msg, 1)
	m = applyMsg(m, StreamToolUseStartMsg{ToolUseID: "x", ToolName: "Grep"})
	if m.activeTool != "Grep" {
		t.Errorf("expected activeTool=Grep, got %q", m.activeTool)
	}
	m.isLoading, m.showSpinner = true, true
	if !strings.Contains(stripANSI(m.View().Content), "Searching…") {
		t.Error("spinner should describe the running tool")
	}
}

// --- 7. Status bar: real usage + context meter ---

func TestUpdate_UsageReachesStatusBar(t *testing.T) {
	m := newTestModel()
	m.statusBar.contextWindow = 200_000
	m.streamCh = make(chan engine.Msg, 1)
	m = applyMsg(m, StreamAssistantTurnMsg{Usage: &TokenUsage{Input: 1000, Output: 500, CacheRead: 98500}})
	m = applyMsg(m, StreamDoneMsg{Usage: &TokenUsage{Input: 1000, Output: 300}})
	if m.statusBar.tokenUsage.Output != 800 {
		t.Errorf("output tokens must accumulate, got %d", m.statusBar.tokenUsage.Output)
	}
	bar := stripANSI(m.statusBar.View(120, m.theme))
	if !strings.Contains(bar, "tok") {
		t.Errorf("status bar must show token usage: %q", bar)
	}
	// Latest request: 1000 + 300 = 1300 of 200k → 0% but meter rendered.
	if !strings.Contains(bar, "ctx") {
		t.Errorf("status bar must show context meter: %q", bar)
	}
	if strings.Contains(bar, "$") {
		t.Errorf("no cost is computed; status bar must not show a fake $ amount: %q", bar)
	}
}

func TestStatusBar_ContextMeterPercent(t *testing.T) {
	s := StatusBar{contextWindow: 200_000}.addUsage(TokenUsage{Input: 20_000, CacheRead: 120_000, Output: 10_000})
	if p := s.contextPercent(); p != 75 {
		t.Errorf("expected 75%%, got %d", p)
	}
	if !strings.Contains(stripANSI(s.contextMeter(DefaultDarkTheme)), "75%") {
		t.Error("meter must show the percentage")
	}
}

func TestStatusBar_ContextMeterUsesBlockGlyphs(t *testing.T) {
	// 75% of 5 cells rounds to 4 filled. █/░ render in common terminal
	// fonts; ▰/▱ fell back to odd-width glyphs in some.
	s := StatusBar{contextWindow: 200_000}.addUsage(TokenUsage{Input: 150_000})
	if got := stripANSI(s.contextMeter(DefaultDarkTheme)); !strings.Contains(got, "ctx ████░ 75%") {
		t.Errorf("meter = %q, want ctx ████░ 75%%", got)
	}
}

func TestStatusBar_FitsWidthWithCJKPath(t *testing.T) {
	s := StatusBar{model: "claude-opus-5-5", cwd: "/Users/someone/專案目錄/非常長的子目錄/glaude", contextWindow: 200_000,
		mode: types.PermissionModeAcceptEdits}.addUsage(TokenUsage{Input: 5000, Output: 100})
	for _, w := range []int{40, 60, 80, 120} {
		if got := displayWidth(s.View(w, DefaultDarkTheme)); got > w {
			t.Errorf("width %d: status bar is %d cells", w, got)
		}
	}
	if !strings.Contains(stripANSI(s.View(120, DefaultDarkTheme)), "shift+tab") {
		t.Error("non-default mode should hint at shift+tab")
	}
}

// --- 8. Input: completion popup, @files, history ---

func typeText(m AppModel, s string) AppModel {
	for _, r := range s {
		m = applyMsg(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func TestSlashCompletion_PopupTabAndEsc(t *testing.T) {
	m := newTestModel()
	m = typeText(m, "/he")
	if !m.completion.visible() || m.completion.kind != completionSlash {
		t.Fatal("typing /he should open the slash completion popup")
	}
	if !strings.Contains(stripANSI(m.View().Content), "/help") {
		t.Error("popup should list /help")
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.input.Value() != "/help " || m.completion.visible() {
		t.Errorf("Tab should complete to '/help ', got %q", m.input.Value())
	}

	m2 := typeText(newTestModel(), "/h")
	m2 = applyMsg(m2, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m2.completion.visible() {
		t.Error("Esc should dismiss the popup")
	}
	if m2.input.Value() != "/h" {
		t.Error("Esc on the popup must not clear the input")
	}
}

func TestSlashCompletion_EnterRunsCommand(t *testing.T) {
	m := newTestModel()
	m = typeText(m, "/hel")
	before := len(m.messages)
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.input.Value() != "" {
		t.Errorf("Enter on a slash suggestion should submit, input=%q", m.input.Value())
	}
	if len(m.messages) <= before {
		t.Error("/help should have produced output")
	}
}

func TestFileCompletion(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"internal/tui/view.go", "internal/tui/model.go", "README.md", "node_modules/x/index.js"} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := newTestModel()
	m.fileIndex = newFileIndex(dir)
	m.appState.SetState(func(prev state.AppState) state.AppState { prev.WorkingDir = dir; return prev })
	// The first @ must not walk the tree on the Update loop: it returns a Cmd.
	m = typeText(m, "look at ")
	updated, cmd := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	m = updated.(AppModel)
	if cmd == nil || m.completion.visible() {
		t.Fatal("first @ should schedule a background index build and show nothing yet")
	}
	ready := findMsg[fileIndexReadyMsg](cmd)
	if ready == nil {
		t.Fatal("index Cmd should yield fileIndexReadyMsg")
	}
	m = applyMsg(m, *ready)
	m = typeText(m, "vie")
	if !m.completion.visible() || m.completion.kind != completionFile {
		t.Fatal("@vie should open file completion")
	}
	if m.completion.items[0].value != "internal/tui/view.go" {
		t.Errorf("expected view.go first, got %+v", m.completion.items)
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.input.Value() != "look at @internal/tui/view.go " {
		t.Errorf("Enter should insert the path, got %q", m.input.Value())
	}
	for _, f := range walkProjectFiles(dir) {
		if strings.HasPrefix(f, "node_modules/") {
			t.Error("node_modules must be skipped")
		}
	}
}

func TestInputHistory_UpDown(t *testing.T) {
	m := newTestModel()
	m.msgQueue = nil
	for _, s := range []string{"first prompt", "second prompt"} {
		m = typeText(m, s)
		m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
		m.isLoading = false // pretend the query finished
	}
	m = typeText(m, "draft")
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.input.Value() != "second prompt" {
		t.Fatalf("↑ should recall the latest prompt, got %q", m.input.Value())
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.input.Value() != "first prompt" {
		t.Fatalf("↑↑ should recall the older prompt, got %q", m.input.Value())
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.input.Value() != "draft" {
		t.Errorf("↓ past the newest should restore the draft, got %q", m.input.Value())
	}
}

func TestUpWithoutHistory_ScrollsConversation(t *testing.T) {
	m := newTestModel()
	for i := 0; i < 60; i++ {
		m.messages = append(m.messages, newUserMessage("filler"))
	}
	m.syncViewportContent()
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.pinnedToBottom {
		t.Error("↑ with no history to recall should scroll the conversation up")
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if !m.pinnedToBottom {
		t.Error("scrolling back to the bottom should re-pin")
	}
}

func TestInputBox_BorderedAndGrows(t *testing.T) {
	in := NewInput(false).SetWidth(60)
	box := stripANSI(in.View(DefaultDarkTheme))
	if !strings.HasPrefix(box, "╭") || renderedLines(box) != 3 {
		t.Errorf("empty input should be a 3-line rounded box, got:\n%s", box)
	}
	if w := displayWidth(strings.Split(box, "\n")[0]); w != 60 {
		t.Errorf("box should span the requested width, got %d", w)
	}
	in = in.SetValue("a\nb\nc")
	if got := renderedLines(stripANSI(in.View(DefaultDarkTheme))); got != 5 {
		t.Errorf("3-line input should grow the box to 5 lines, got %d", got)
	}
}

// --- 9. Dialogs, todo panel, thinking, theme colours, system notices ---

func TestView_PinnedConversationVisibleWithDialog(t *testing.T) {
	m := newTestModel()
	for i := 0; i < 40; i++ {
		m.messages = append(m.messages, newUserMessage("filler"))
	}
	m.messages = append(m.messages, newUserMessage("LAST MESSAGE"))
	m.syncViewportContent()
	m.activeDialog = dialogExit
	if !strings.Contains(m.View().Content, "LAST MESSAGE") {
		t.Error("newest message must stay visible when a dialog shrinks the viewport")
	}
}

func TestConfirmDialog_Bordered(t *testing.T) {
	out := stripANSI(renderConfirmDialog("Exit Glaude?", "body", 40, DefaultDarkTheme))
	if !strings.HasPrefix(out, "╭") || !strings.Contains(out, "y confirm") {
		t.Errorf("confirm dialog should be a bordered box with key hints:\n%s", out)
	}
	if w := displayWidth(strings.Split(out, "\n")[0]); w != 40 {
		t.Errorf("dialog should honour width, got %d", w)
	}
}

func TestPermissionDialog_HintsOnlySupportedKeys(t *testing.T) {
	d := newPermissionDialog(PermissionRequestMsg{ToolName: "Bash", Input: `{"command":"ls"}`})
	out := stripANSI(d.View(80, DefaultDarkTheme))
	if strings.Contains(out, "Tab to amend") || strings.Contains(out, "ctrl+e") {
		t.Error("dialog must not advertise unimplemented keys")
	}
}

func TestCtrlO_TogglesThinking(t *testing.T) {
	m := newTestModel()
	thinking := "secret reasoning"
	m.messages = []types.Message{{Role: types.RoleAssistant, Content: []types.ContentBlock{
		{Type: types.ContentTypeThinking, Thinking: &thinking},
	}}}
	m.syncViewportContent()
	if strings.Contains(stripANSI(m.viewport.View()), "secret reasoning") {
		t.Error("thinking should start collapsed")
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if !strings.Contains(stripANSI(m.viewport.View()), "secret reasoning") {
		t.Error("ctrl+o should expand thinking")
	}
}

func TestSystemNotices_InfoVsError(t *testing.T) {
	opts := renderOptions{width: 80, theme: DefaultDarkTheme}
	lk := buildMessageLookups(nil)
	info := stripANSI(renderMessageOpts(newSystemMessage("all good"), opts, lk))
	errOut := stripANSI(renderMessageOpts(newSystemMessage("Error: it broke"), opts, lk))
	if !strings.Contains(info, "※") || strings.Contains(info, "✗") {
		t.Errorf("info notice marker wrong: %q", info)
	}
	if !strings.Contains(errOut, "✗ Error: it broke") {
		t.Errorf("error notice marker wrong: %q", errOut)
	}
	if strings.Contains(info, blackCircle()) {
		t.Error("system notices must not look like model output")
	}
}

func TestGlamourColor_FromTheme(t *testing.T) {
	if got := *glamourColor(lipgloss.Color("12")); got != "12" {
		t.Errorf("basic ANSI colour should stay an index, got %q", got)
	}
	if got := *glamourColor(lipgloss.Color("205")); got != "205" {
		t.Errorf("256-colour index should stay an index, got %q", got)
	}
	if got := *glamourColor(lipgloss.Color("#ff8800")); got != "#ff8800" {
		t.Errorf("hex colour should round-trip, got %q", got)
	}
}

func TestThemeChange_RebuildsMarkdownRenderer(t *testing.T) {
	m := newTestModel()
	r1 := m.markdownRenderer()
	if r1 == nil {
		t.Skip("glamour unavailable")
	}
	m.theme = TokyoNightTheme
	if r2 := m.markdownRenderer(); r2 == r1 {
		t.Error("theme change must rebuild the markdown renderer")
	}
}

func TestTextBlock_WrapsLongCJKLine(t *testing.T) {
	long := "中文寬度測試：" + strings.Repeat("很長", 60)
	opts := renderOptions{width: 50, theme: DefaultDarkTheme}
	out := renderTextBlock(long, opts, "*")
	plain := strings.ReplaceAll(strings.ReplaceAll(stripANSI(out), "\n", ""), " ", "")
	if strings.Count(plain, "很長") != 60 {
		t.Errorf("no CJK text may be lost, got %d of 60 runs:\n%s", strings.Count(plain, "很長"), plain)
	}
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if displayWidth(l) > 50 {
			t.Errorf("line wider than terminal: %d", displayWidth(l))
		}
	}
}

func TestStatusBar_NarrowKeepsContextMeter(t *testing.T) {
	s := StatusBar{model: "gpt-4o-mini", cwd: "/x", contextWindow: 200_000}.addUsage(TokenUsage{Input: 30_000, Output: 100})
	out := stripANSI(s.View(50, DefaultDarkTheme))
	if !strings.Contains(out, "ctx") {
		t.Errorf("narrow status bar should keep the context meter: %q", out)
	}
	if displayWidth(out) > 50 {
		t.Errorf("too wide: %d", displayWidth(out))
	}
}

// findMsg runs cmd (expanding batches) and returns the first message of type T.
func findMsg[T any](cmd tea.Cmd) *T {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case T:
		return &msg
	case tea.BatchMsg:
		for _, c := range msg {
			if got := findMsg[T](c); got != nil {
				return got
			}
		}
	}
	return nil
}

// --- Review fixes ---

func TestAltEnter_InsertsNewline(t *testing.T) {
	m := typeText(newTestModel(), "line1")
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	m = typeText(m, "line2")
	if m.input.Value() != "line1\nline2" {
		t.Errorf("alt+enter should insert a newline, got %q", m.input.Value())
	}
	if len(m.messages) != 0 {
		t.Error("alt+enter must not submit")
	}
}

func TestShiftEnter_InsertsNewline(t *testing.T) {
	m := typeText(newTestModel(), "line1")
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = typeText(m, "line2")
	if m.input.Value() != "line1\nline2" {
		t.Errorf("shift+enter should insert a newline, got %q", m.input.Value())
	}
	if len(m.messages) != 0 {
		t.Error("shift+enter must not submit")
	}
}

func TestShiftEnter_WithCompletionInsertsNewline(t *testing.T) {
	m := typeText(newTestModel(), "/he")
	if !m.completion.visible() {
		t.Fatal("expected the slash completion popup")
	}
	before := len(m.messages)
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if m.input.Value() != "/he\n" {
		t.Errorf("shift+enter with the popup open should insert a newline, got %q", m.input.Value())
	}
	if len(m.messages) != before {
		t.Error("shift+enter must not run the suggested command")
	}
}

func TestInput_ContentNotCappedAtVisibleRows(t *testing.T) {
	m := newTestModel()
	for i := 0; i < 15; i++ {
		m = typeText(m, "x")
		m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	}
	if got := m.input.textarea.LineCount(); got != 16 {
		t.Errorf("input should hold 16 lines (only the view is capped), got %d", got)
	}
}

func TestSlashEnter_AmbiguousDoesNotRunFirstMatch(t *testing.T) {
	m := newTestModel()
	m.messages = []types.Message{newUserMessage("keep me")}
	m = typeText(m, "/c")
	if !m.completion.visible() || len(m.completion.items) < 2 {
		t.Fatal("/c should be ambiguous")
	}
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	found := false
	for _, msg := range m.messages {
		for _, b := range msg.Content {
			if b.Text != nil && *b.Text == "keep me" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("ambiguous /c + Enter must not run /clear (history was wiped)")
	}
}

func TestSlashEnter_NavigatedSelectionRuns(t *testing.T) {
	m := newTestModel()
	m = typeText(m, "/he")
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if !m.completion.navigated {
		t.Fatal("arrow keys should mark the selection as chosen")
	}
	before := len(m.messages)
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.messages) <= before || m.input.Value() != "" {
		t.Error("Enter on an explicitly chosen command should run it")
	}
}

func TestEscWithPopup_StillInterruptsQuery(t *testing.T) {
	m := typeText(newTestModel(), "/he")
	m.isLoading = true
	m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.isLoading {
		t.Error("Esc must interrupt a running query even while the popup is open")
	}
	if m.completion.visible() {
		t.Error("Esc should also close the popup")
	}
}

func TestCompact_ResetsContextMeter(t *testing.T) {
	m := newTestModel()
	m.statusBar.contextTokens = 150_000
	m = applyMsg(m, CompactDoneMsg{Summary: "done"})
	if m.statusBar.contextTokens != 0 {
		t.Error("compaction must reset the context meter")
	}
}

func TestPaste_InsertsIntoInput(t *testing.T) {
	m := applyMsg(newTestModel(), tea.PasteMsg{Content: "pasted 中文\nsecond"})
	if m.input.Value() != "pasted 中文\nsecond" {
		t.Errorf("bracketed paste should land in the input, got %q", m.input.Value())
	}
}

func TestFileCompletion_ExactMatchHides(t *testing.T) {
	fi := newFileIndex("/r")
	fi.adopt(fileIndexReadyMsg{root: "/r", files: []string{"main.go"}})
	c := computeCompletion(completionState{}, "see @main.go", true, nil, fi)
	if c.visible() {
		t.Error("an exact, unique file mention should not keep the popup open")
	}
}

func TestView_ShortTerminalWithTallInputFits(t *testing.T) {
	m := newTestModel()
	m = applyMsg(m, tea.WindowSizeMsg{Width: 80, Height: 12})
	for i := 0; i < 10; i++ {
		m = typeText(m, "row")
		m = applyMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	}
	m = typeText(m, "/c")
	m.isLoading, m.showSpinner = true, true
	if lines := viewLines(m.View()); lines > 12 {
		t.Errorf("view is %d lines on a 12-line terminal", lines)
	}
}

// --- Process memory in the status bar ---

func TestSampleProcessMemory_Plausible(t *testing.T) {
	b := sampleProcessMemory()
	if b < 1<<20 || b > 64<<30 {
		t.Errorf("implausible process memory sample: %d bytes", b)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{512 << 10: "512KB", 3 << 20: "3.0MB", 48 << 20: "48MB", 3 << 29: "1.5GB"}
	for b, want := range cases {
		if got := formatBytes(b); got != want {
			t.Errorf("%d: got %q want %q", b, got, want)
		}
	}
}

func TestStatusBar_ShowsMemory(t *testing.T) {
	m := newTestModel()
	// A tick past the interval requests a sample via a Cmd (off the loop).
	updated, cmd := m.Update(TickMsg{Time: m.lastMemSample.Add(2 * memSampleInterval)})
	m = updated.(AppModel)
	sample := findMsg[memSampleMsg](cmd)
	if sample == nil || !m.memSamplePending {
		t.Fatal("tick should schedule a background memory sample")
	}
	// Not again before the interval has passed.
	if _, c := m.Update(TickMsg{Time: m.lastMemSample.Add(memSampleInterval / 2)}); findMsg[memSampleMsg](c) != nil {
		t.Error("must not sample more often than memSampleInterval")
	}
	// No second sample while one is in flight.
	if _, cmd2 := m.Update(TickMsg{Time: m.lastMemSample.Add(4 * memSampleInterval)}); findMsg[memSampleMsg](cmd2) != nil {
		t.Error("must not stack memory samples")
	}
	m = applyMsg(m, *sample)
	if m.memSamplePending || m.statusBar.memBytes == 0 {
		t.Error("sample result should land in the status bar")
	}
	if !strings.Contains(stripANSI(m.statusBar.View(120, m.theme)), "mem ") {
		t.Error("status bar should show process memory")
	}
}

func TestSampleProcessMemory_MatchesOSRSS(t *testing.T) {
	rss, ok := processRSS()
	if !ok {
		t.Skip("OS RSS unavailable on this platform")
	}
	if goMem := goRuntimeMemory(); rss < goMem/2 {
		t.Errorf("RSS %d implausibly below Go runtime memory %d", rss, goMem)
	}
}
