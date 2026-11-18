package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// TestSlashConfig_OpensDialogAndEscCloses covers the /config panel flow:
// the command opens dialogConfig, the view renders current settings, Esc
// closes it — and the whole flow is display-only (cache-first gate: engine
// history must not change).
func TestSlashConfig_OpensDialogAndEscCloses(t *testing.T) {
	m := newTestModel()
	m.queryEngine.SetMessages([]types.Message{newUserMessage("seed history")})
	engineBefore := m.queryEngine.GetMessages()

	updated, _ := m.handleSlashCommand("/config")
	if updated.activeDialog != dialogConfig {
		t.Fatalf("expected dialogConfig, got %v", updated.activeDialog)
	}
	view := updated.View()
	for _, want := range []string{"Configuration", "test-model", "Esc to close"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected %q in config dialog view", want)
		}
	}

	closed, _ := updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	closedModel := closed.(AppModel)
	if closedModel.activeDialog != dialogNone {
		t.Errorf("Esc should close the config dialog, got %v", closedModel.activeDialog)
	}
	if engineAfter := closedModel.queryEngine.GetMessages(); !reflect.DeepEqual(engineBefore, engineAfter) {
		t.Errorf("/config must not modify engine history (cache gate display-only): before %v, after %v",
			engineBefore, engineAfter)
	}
}

// TestSlashConfig_ShowsVimAndTheme verifies the panel reflects live model
// state (vim toggle, dark mode) rather than static text.
func TestSlashConfig_ShowsVimAndTheme(t *testing.T) {
	m := newTestModel()
	m.input.vimEnabled = true
	m.darkMode = true

	updated, _ := m.handleSlashCommand("/config")
	view := updated.View()
	if !strings.Contains(view, "on") || !strings.Contains(view, "dark") {
		t.Errorf("expected vim=on and theme=dark in dialog, got:\n%s", view)
	}
}
