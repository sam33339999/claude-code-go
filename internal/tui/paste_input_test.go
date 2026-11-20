package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/pkg/types"
)

func TestPaste_LargeTextCollapses(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	big := strings.Repeat("line\n", 40) + "end"
	updated, _ := m.Update(tea.PasteMsg{Content: big})
	m = updated.(AppModel)
	if got := m.input.Value(); got != "[Pasted text #1 +40 lines]" {
		t.Fatalf("input = %q", got)
	}
}

func TestPaste_SmallTextInline(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("see: ")
	updated, _ := m.Update(tea.PasteMsg{Content: "a\r\nb"})
	if got := updated.(AppModel).input.Value(); got != "see: a\nb" {
		t.Fatalf("input = %q", got)
	}
}

func TestPaste_InsertsAtCursor(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("ab")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	updated, _ := m.Update(tea.PasteMsg{Content: strings.Repeat("z", 900)})
	if got := updated.(AppModel).input.Value(); got != "a[Pasted text #1 900 chars]b" {
		t.Fatalf("input = %q", got)
	}
}

func TestPaste_ReplacesSelection(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("ab")
	m.input.textarea.SelectAll()
	updated, _ := m.Update(tea.PasteMsg{Content: "Z"})
	if got := updated.(AppModel).input.Value(); got != "Z" {
		t.Fatalf("input = %q", got)
	}
}

func TestDeleteToken_Backspace(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	marker := m.pastes.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	m.input = m.input.SetValue("look " + marker)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.input.Value(); got != "look " {
		t.Fatalf("input = %q", got)
	}
	if !m.pastes.isMarker(marker) {
		t.Fatal("deleting a marker must leave it in the store")
	}
}

func TestDeleteToken_TypedLookalikeIsPlainText(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("[Image #9]")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.input.Value(); got != "[Image #9" {
		t.Fatalf("input = %q", got)
	}
}

func TestDeleteToken_RecalledHistory(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	updated, _ := m.Update(tea.PasteMsg{Content: strings.Repeat("q\n", 20)})
	m = updated.(AppModel)
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmds(cmd)
	m.isLoading = false
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.input.Value(); got != "" {
		t.Fatalf("recalled marker not deleted whole: %q", got)
	}
	if m.history.browsing() {
		t.Fatal("token delete must end history browsing")
	}
}

func TestDeleteToken_VimNormalX(t *testing.T) {
	appStore := state.NewAppStateStore(state.AppState{
		WorkingDir:    "/tmp",
		MainLoopModel: state.ModelSetting{ModelID: "main-model"},
	})
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	m := newAppModel(newFakeEngine(), appStore, true, true, reg,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	marker := m.pastes.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	m.input = m.input.SetValue(marker)
	// Insert mode: x is typed text. Only vim normal mode deletes forward.
	m, _ = press(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.input.Value(); got != marker+"x" {
		t.Fatalf("insert-mode x = %q", got)
	}
	m.input = m.input.SetValue(marker + " tail")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	// SetValue leaves the cursor at the end, outside a forward delete.
	m, _ = press(m, tea.KeyPressMsg{Code: '0', Text: "0"})
	m, _ = press(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := m.input.Value(); got != " tail" {
		t.Fatalf("input = %q", got)
	}
}

func TestDeleteToken_DeleteAndWord(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	marker := m.pastes.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	m.input = m.input.SetValue(marker + " tail")
	for range len([]rune(marker + " tail")) {
		m.input.textarea, _ = m.input.textarea.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDelete})
	if got := m.input.Value(); got != " tail" {
		t.Fatalf("delete = %q", got)
	}

	m.input = m.input.SetValue("see " + marker + " now")
	for range 3 {
		m.input.textarea, _ = m.input.textarea.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	m, _ = press(m, tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if got := m.input.Value(); got != "see  now" {
		t.Fatalf("word delete = %q", got)
	}
}
