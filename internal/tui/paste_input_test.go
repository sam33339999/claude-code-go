package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
