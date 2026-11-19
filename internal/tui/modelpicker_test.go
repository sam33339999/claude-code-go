package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/pkg/types"
)

type fakeSwitcher struct {
	opts    []commands.ModelOption
	applied []commands.ModelOption
	resolve func(string) (commands.ModelOption, error)
}

func (f *fakeSwitcher) Options() []commands.ModelOption { return f.opts }
func (f *fakeSwitcher) Resolve(ref string) (commands.ModelOption, error) {
	if f.resolve != nil {
		return f.resolve(ref)
	}
	for _, o := range f.opts {
		if o.Name == ref {
			return o, nil
		}
	}
	return commands.ModelOption{}, errors.New("unknown model " + ref)
}
func (f *fakeSwitcher) Apply(o commands.ModelOption) error {
	f.applied = append(f.applied, o)
	return nil
}

func newSwitcher() *fakeSwitcher {
	return &fakeSwitcher{opts: []commands.ModelOption{
		{Name: "default", Provider: "main", Model: "claude-opus-5-5", Available: true, Current: true},
		{Name: "lite", Provider: "zai", Model: "glm-5.3-flash", ContextWindow: 131072, Available: true},
		{Name: "pro", Reason: "unbound"},
		{Name: "max", Provider: "main", Model: "claude-opus-5-5-max", ContextWindow: 20000, Available: true},
	}}
}

func newPickerModel(sw commands.ModelSwitcher, history bool) (AppModel, *fakeEngine) {
	fe := newFakeEngine()
	if history {
		fe.messages = []types.Message{newUserMessage("hello"), newUserMessage(strings.Repeat("x", 120_000))}
	}
	appStore := state.NewAppStateStore(state.AppState{WorkingDir: "/tmp", MainLoopModel: state.ModelSetting{ModelID: "claude-opus-5-5"}})
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	m := newAppModel(fe, appStore, false, true, reg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, sw, nil, nil, nil, nil)
	m.termWidth, m.termHeight = 100, 30
	return m, fe
}

func openPicker(m AppModel) AppModel {
	m, _ = m.applyCommandResult(commands.Result{OpenDialog: "model", Display: commands.DisplayNone}, "model")
	return m
}

func lastText(m AppModel) string {
	if len(m.messages) == 0 {
		return ""
	}
	c := m.messages[len(m.messages)-1].Content
	if len(c) == 0 || c[0].Text == nil {
		return ""
	}
	return *c[0].Text
}

func TestModelPicker_OpensWithOptions(t *testing.T) {
	m, _ := newPickerModel(newSwitcher(), false)
	m = openPicker(m)
	if m.activeDialog != dialogModelPicker {
		t.Fatalf("dialog = %v", m.activeDialog)
	}
	v := m.renderActiveDialog()
	for _, w := range []string{"default", "lite", "zai:glm-5.3-flash", "unbound", "current"} {
		if !strings.Contains(v, w) {
			t.Errorf("picker missing %q:\n%s", w, v)
		}
	}
}

func TestModelPicker_SkipsUnavailable(t *testing.T) {
	m, _ := newPickerModel(newSwitcher(), false)
	m = openPicker(m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown}) // lite
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown}) // pro unavailable → max
	if got := m.modelPicker.opts[m.modelPicker.cursor].Name; got != "max" {
		t.Errorf("cursor on %q, want max (pro skipped)", got)
	}
}

func TestModelPicker_ConfirmWhenHistory(t *testing.T) {
	sw := newSwitcher()
	m, _ := newPickerModel(sw, true)
	m = openPicker(m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.activeDialog != dialogModelConfirm {
		t.Fatalf("dialog = %v, want confirm", m.activeDialog)
	}
	v := m.renderActiveDialog()
	if !strings.Contains(v, "cache") || !strings.Contains(v, "≈") {
		t.Errorf("confirm view:\n%s", v)
	}
	m2, _ := press(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if len(sw.applied) != 0 || m2.activeDialog != dialogNone {
		t.Fatal("n must cancel without applying")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(sw.applied) != 1 || sw.applied[0].Name != "lite" {
		t.Fatalf("applied = %+v", sw.applied)
	}
	if m.statusBar.model != "glm-5.3-flash" || m.statusBar.contextWindow != 131072 {
		t.Errorf("status bar = %q %d", m.statusBar.model, m.statusBar.contextWindow)
	}
	if !strings.Contains(lastText(m), "session only") {
		t.Errorf("notice = %q", lastText(m))
	}
}

func TestModelPicker_NoConfirmWhenEmpty(t *testing.T) {
	sw := newSwitcher()
	m, _ := newPickerModel(sw, false)
	m = openPicker(m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(sw.applied) != 1 || m.activeDialog != dialogNone {
		t.Errorf("applied %d, dialog %v", len(sw.applied), m.activeDialog)
	}
}

func TestModelPicker_SameTargetNoop(t *testing.T) {
	sw := newSwitcher()
	m, _ := newPickerModel(sw, true)
	m = openPicker(m)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter}) // default is current
	if len(sw.applied) != 0 || !strings.Contains(lastText(m), "Already") {
		t.Errorf("applied %d, notice %q", len(sw.applied), lastText(m))
	}
}

func TestModelPicker_WindowWarning(t *testing.T) {
	m, _ := newPickerModel(newSwitcher(), true)
	m = openPicker(m)
	for range 2 {
		m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyDown}) // → max (20k window)
	}
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if v := m.renderActiveDialog(); !strings.Contains(v, "compacted") {
		t.Errorf("missing window warning:\n%s", v)
	}
}

func TestModelPicker_RejectWhileLoading(t *testing.T) {
	m, _ := newPickerModel(newSwitcher(), false)
	m.isLoading = true
	m = openPicker(m)
	if m.activeDialog != dialogNone || !strings.Contains(lastText(m), "Esc") {
		t.Errorf("dialog %v notice %q", m.activeDialog, lastText(m))
	}
}

func TestModelPicker_TypedRef(t *testing.T) {
	sw := newSwitcher()
	m, _ := newPickerModel(sw, true)
	m, _ = m.applyCommandResult(commands.Result{ModelRef: "lite"}, "model")
	if m.activeDialog != dialogModelConfirm {
		t.Fatalf("typed ref with history must confirm, dialog %v", m.activeDialog)
	}
	m, _ = m.applyCommandResult(commands.Result{ModelRef: "nope"}, "model")
	m2, _ := newPickerModel(sw, false)
	m2, _ = m2.applyCommandResult(commands.Result{ModelRef: "nope"}, "model")
	if !strings.Contains(lastText(m2), "unknown model") {
		t.Errorf("error notice = %q", lastText(m2))
	}
}

func TestModelPicker_ConfirmBlocksReentry(t *testing.T) {
	sw := newSwitcher()
	m, _ := newPickerModel(sw, true)
	m, _ = m.applyCommandResult(commands.Result{ModelRef: "lite"}, "model")
	m = openPicker(m)
	if m.activeDialog != dialogModelConfirm || len(sw.applied) != 0 {
		t.Errorf("dialog %v applied %d", m.activeDialog, len(sw.applied))
	}
}

func TestModelPicker_NilSwitcherFallsBack(t *testing.T) {
	m, fe := newPickerModel(nil, false)
	m, _ = m.applyCommandResult(commands.Result{ModelRef: "x-model"}, "model")
	if fe.model != "x-model" || m.statusBar.model != "x-model" {
		t.Errorf("fallback SetModel: engine %q status %q", fe.model, m.statusBar.model)
	}
}
