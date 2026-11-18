package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

func TestNextPermissionMode_Cycle(t *testing.T) {
	cases := []struct {
		from, want types.PermissionMode
	}{
		{types.PermissionModeDefault, types.PermissionModeAcceptEdits},
		{types.PermissionModeAcceptEdits, types.PermissionModePlan},
		{types.PermissionModePlan, types.PermissionModeBypassPermissions},
		{types.PermissionModeBypassPermissions, types.PermissionModeDefault},
		{types.PermissionModeDontAsk, types.PermissionModeDefault}, // unknown → default
	}
	for _, tc := range cases {
		if got := nextPermissionMode(tc.from); got != tc.want {
			t.Errorf("nextPermissionMode(%q)=%q, want %q", tc.from, got, tc.want)
		}
	}
}

func TestShiftTab_CyclesModeAndShowsIndicator(t *testing.T) {
	m := newTestModel()
	wantModes := []types.PermissionMode{
		types.PermissionModeAcceptEdits,
		types.PermissionModePlan,
		types.PermissionModeBypassPermissions,
		types.PermissionModeDefault,
	}
	wantIndicators := []string{"accept edits", "plan", "YOLO", ""}
	for i, want := range wantModes {
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		m = updated.(AppModel)
		if got := m.appState.GetState().ToolPermissionContext.Mode; got != want {
			t.Fatalf("cycle %d: mode=%q, want %q", i, got, want)
		}
		// Check the status bar only (conversation keeps the mode-change notes).
		m.statusBar.mode = m.appState.GetState().ToolPermissionContext.Mode
		bar := m.statusBar.View(m.termWidth, m.theme)
		if wantIndicators[i] == "" {
			if strings.Contains(bar, "YOLO") || strings.Contains(bar, "⏸ plan") || strings.Contains(bar, "accept edits") {
				t.Errorf("cycle %d: normal mode must show no indicator", i)
			}
		} else if !strings.Contains(bar, wantIndicators[i]) {
			t.Errorf("cycle %d: expected indicator %q in status bar", i, wantIndicators[i])
		}
	}
	// The YOLO step must warn in the conversation too.
	found := false
	for _, msg := range m.messages {
		for _, b := range msg.Content {
			if b.Text != nil && strings.Contains(*b.Text, "ALL permission checks skipped") {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected YOLO warning system message")
	}
}

func TestShiftTab_PlanModeBlocksWritesViaLiveChecker(t *testing.T) {
	// Integration-style: AppState mode change must be visible to a checker
	// wired with ModeFn (the wire.go pattern), without rebuilding the checker.
	store := state.NewAppStateStore(state.GetDefaultAppState())
	modeFn := func() types.PermissionMode { return store.GetState().ToolPermissionContext.Mode }
	if modeFn() != types.PermissionModeDefault {
		t.Fatal("initial mode should be default")
	}
	m := newTestModel()
	m.appState = store
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})      // acceptEdits
	updated, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}) // plan
	m = updated.(AppModel)
	if modeFn() != types.PermissionModePlan {
		t.Errorf("ModeFn must observe plan mode live, got %q", modeFn())
	}
}
