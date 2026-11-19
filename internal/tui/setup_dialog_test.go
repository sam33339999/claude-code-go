package tui

import (
	"context"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/state"
)

type fakeSetupBackend struct {
	mu        sync.Mutex
	defaults  commands.SetupDefaults
	models    []commands.SetupModel
	listCalls []commands.SetupInput
	applied   []commands.SetupPlan
	needs     bool
	overrides []string
}

func (f *fakeSetupBackend) Current() commands.SetupDefaults { return f.defaults }
func (f *fakeSetupBackend) ListModels(_ context.Context, in commands.SetupInput) ([]commands.SetupModel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls = append(f.listCalls, in)
	return f.models, nil
}
func (f *fakeSetupBackend) Apply(_ context.Context, p commands.SetupPlan) (commands.SetupReport, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, p)
	return commands.SetupReport{SettingsWritten: true}, nil
}
func (f *fakeSetupBackend) Overrides() []string { return f.overrides }
func (f *fakeSetupBackend) NeedsSetup() bool    { return f.needs }

func newSetupModel(b commands.SetupBackend) AppModel {
	appStore := state.NewAppStateStore(state.AppState{WorkingDir: "/tmp", MainLoopModel: state.ModelSetting{ModelID: "m"}})
	reg := commands.NewRegistry()
	commands.RegisterBuiltins(reg)
	m := newAppModel(newFakeEngine(), appStore, false, true, reg, nil, nil, nil, nil, nil, nil, nil, nil, nil, b, nil, nil, nil)
	m.termWidth, m.termHeight = 100, 30
	return m
}

func openSetup(t *testing.T, b commands.SetupBackend) AppModel {
	t.Helper()
	m, _ := newSetupModel(b).applyCommandResult(commands.Result{OpenDialog: "setup", Display: commands.DisplayNone}, "setup")
	return m
}

func press(m AppModel, k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	updated, cmd := m.Update(k)
	return updated.(AppModel), cmd
}

// toEndpointModel opens /setup and moves to the endpoint step with the API
// key field focused.
func toEndpointModel(t *testing.T, b commands.SetupBackend) AppModel {
	t.Helper()
	m := openSetup(t, b)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyTab})
	return m
}

func TestSetupDialog_OpenFromCommand(t *testing.T) {
	b := &fakeSetupBackend{defaults: commands.SetupDefaults{Format: commands.SetupFormatOpenAI}}
	m := openSetup(t, b)
	if m.activeDialog != dialogSetup || m.setupWizard == nil || m.setupWizard.formatIdx != 1 {
		t.Fatalf("dialog %v wizard %+v", m.activeDialog, m.setupWizard)
	}
	if !strings.Contains(m.renderActiveDialog(), "Setup") {
		t.Error("setup dialog not rendered")
	}
}

func TestSetupDialog_NilBackend(t *testing.T) {
	m := openSetup(t, nil)
	if m.activeDialog == dialogSetup {
		t.Error("dialog opened without a backend")
	}
	last := m.messages[len(m.messages)-1]
	if !strings.Contains(*last.Content[0].Text, "not available") {
		t.Errorf("missing notice: %q", *last.Content[0].Text)
	}
}

func TestSetupDialog_ValidateRunsAsCmd(t *testing.T) {
	b := &fakeSetupBackend{models: []commands.SetupModel{{ID: "q/a"}}}
	m := toEndpointModel(t, b)
	for _, r := range "sk" {
		m, _ = press(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter on endpoint must return a cmd")
	}
	if len(b.listCalls) != 0 {
		t.Fatal("ListModels must not run inside Update")
	}
	msg := cmd()
	if len(b.listCalls) != 1 || b.listCalls[0].APIKey != "sk" {
		t.Fatalf("listCalls = %+v", b.listCalls)
	}
	updated, _ := m.Update(msg)
	if w := updated.(AppModel).setupWizard; w.step != stepModels {
		t.Errorf("step = %v, want models", w.step)
	}
}

func TestSetupDialog_StaleModelsMsgIgnored(t *testing.T) {
	b := &fakeSetupBackend{models: []commands.SetupModel{{ID: "q/a"}}}
	m := toEndpointModel(t, b)
	for _, r := range "sk" {
		m, _ = press(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc}) // abandon validation
	updated, _ := m.Update(cmd())
	if w := updated.(AppModel).setupWizard; w.step != stepEndpoint {
		t.Errorf("stale result advanced the wizard to %v", w.step)
	}
}

func TestSetupDialog_PasteGoesToFocusedField(t *testing.T) {
	m := toEndpointModel(t, &fakeSetupBackend{})
	before := m.input.Value()
	updated, _ := m.Update(tea.PasteMsg{Content: "sk-xyz"})
	m = updated.(AppModel)
	if got := m.setupWizard.input().APIKey; got != "sk-xyz" {
		t.Errorf("APIKey = %q, want sk-xyz", got)
	}
	if m.input.Value() != before {
		t.Error("paste leaked into the main input box")
	}
}

func TestSetupDialog_ApplyRunsAsCmd(t *testing.T) {
	b := &fakeSetupBackend{models: []commands.SetupModel{{ID: "q/a"}}}
	m := toEndpointModel(t, b)
	m, _ = press(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter}) // models → confirm (cursor model)
	m, cmd = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || len(b.applied) != 0 {
		t.Fatal("apply must run as a cmd")
	}
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if len(b.applied) != 1 || b.applied[0].MainModel != "q/a" {
		t.Errorf("applied = %+v", b.applied)
	}
	if m.setupWizard.step != stepDone {
		t.Errorf("step = %v, want done", m.setupWizard.step)
	}
	m, _ = press(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.activeDialog != dialogNone || m.setupWizard != nil {
		t.Error("any key on done must close the dialog")
	}
}

func TestSetupDialog_CloseRestoresIdle(t *testing.T) {
	m := openSetup(t, &fakeSetupBackend{})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.activeDialog != dialogNone || m.setupWizard != nil {
		t.Errorf("dialog %v wizard %v", m.activeDialog, m.setupWizard)
	}
}

func TestStartup_NeedsSetupNotice(t *testing.T) {
	has := func(m AppModel) bool {
		for _, msg := range m.messages {
			if len(msg.Content) > 0 && msg.Content[0].Text != nil && strings.Contains(*msg.Content[0].Text, "/setup") {
				return true
			}
		}
		return false
	}
	if !has(newSetupModel(&fakeSetupBackend{needs: true})) {
		t.Error("missing /setup notice when no credentials")
	}
	if has(newSetupModel(&fakeSetupBackend{needs: false})) {
		t.Error("unexpected /setup notice")
	}
}

// TestSetupDialog_StaleMsgFromClosedWizardIgnored: a request abandoned in a
// previous /setup session must not be accepted by a reopened wizard.
func TestSetupDialog_StaleMsgFromClosedWizardIgnored(t *testing.T) {
	b := &fakeSetupBackend{models: []commands.SetupModel{{ID: "old/model"}}}
	m := toEndpointModel(t, b)
	m, _ = press(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m, oldCmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter}) // validating (gen 1)
	for range 3 {                                              // validating → endpoint → format → closed
		m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	}
	if m.activeDialog != dialogNone {
		t.Fatal("dialog should be closed")
	}
	m, _ = m.applyCommandResult(commands.Result{OpenDialog: "setup", Display: commands.DisplayNone}, "setup")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = press(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter}) // new request in flight
	updated, _ := m.Update(oldCmd())
	if w := updated.(AppModel).setupWizard; w.step != stepValidating {
		t.Errorf("stale result from the closed wizard was accepted (step %v)", w.step)
	}
}
