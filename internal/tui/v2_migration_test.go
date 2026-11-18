package tui

import (
	"testing"

	"charm.land/bubbletea/v2"
)

// TestView_ScreenOptions proves the v2 migration: alt-screen and mouse mode
// live on the returned tea.View (program options were removed in v2).
func TestView_ScreenOptions(t *testing.T) {
	m := newTestModel()
	v := m.View()
	if !v.AltScreen {
		t.Error("View must enable AltScreen (v2: set on View, not program options)")
	}
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("View must enable cell-motion mouse, got %v", v.MouseMode)
	}
	// Zero-width early return must carry the same screen options.
	m.termWidth = 0
	v = m.View()
	if !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion {
		t.Error("empty view must keep screen options stable")
	}
}

// TestKeyRelease_Ignored proves key-release events (Kitty protocol, new in
// v2) do not trigger actions — only KeyPressMsg is handled.
func TestKeyRelease_Ignored(t *testing.T) {
	m := newTestModel()
	before := len(m.messages)
	updated, cmd := m.Update(tea.KeyReleaseMsg{Code: 'c', Mod: tea.ModCtrl})
	m = updated.(AppModel)
	if cmd != nil {
		t.Error("key release must not produce a Cmd (ctrl+c release must not quit)")
	}
	if len(m.messages) != before {
		t.Error("key release must not mutate messages")
	}
}

// TestShiftTab_ComposedKey proves shift+tab arrives as Tab+ModShift in v2
// and still cycles the permission mode.
func TestShiftTab_ComposedKey(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(AppModel)
	if got := m.appState.GetState().ToolPermissionContext.Mode; got != "acceptEdits" {
		t.Errorf("shift+tab must cycle mode, got %q", got)
	}
	// Plain tab must NOT cycle (it is tab completion).
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(AppModel)
	if got := m.appState.GetState().ToolPermissionContext.Mode; got != "acceptEdits" {
		t.Errorf("plain tab must not cycle mode, got %q", got)
	}
}
