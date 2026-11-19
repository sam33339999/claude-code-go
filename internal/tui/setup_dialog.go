package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
)

// setupModelsMsg carries a ListModels result back to Update. gen identifies
// the request so results of abandoned requests are dropped.
type setupModelsMsg struct {
	gen    int
	models []commands.SetupModel
	err    error
}

// setupAppliedMsg carries an Apply result back to Update.
type setupAppliedMsg struct {
	gen    int
	report commands.SetupReport
	err    error
}

const (
	setupListTimeout  = 15 * time.Second
	setupApplyTimeout = 30 * time.Second
)

// openSetupDialog starts the /setup wizard from the backend's current values.
func (m AppModel) openSetupDialog() AppModel {
	if m.setupBackend == nil {
		m.messages = append(m.messages, newSystemErrorMessage("Setup is not available in this session."))
		m.syncViewportContent()
		return m
	}
	m.setupWizard = newSetupWizard(m.setupBackend.Current(), m.setupBackend.Overrides())
	m.activeDialog = dialogSetup
	return m
}

// handleSetupKey routes a key to the wizard and runs the requested side
// effect as a tea.Cmd (Update itself never does I/O).
func (m AppModel) handleSetupKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	w := m.setupWizard
	if w == nil || m.setupBackend == nil {
		m.activeDialog = dialogNone
		return m, nil
	}
	switch w.handleKey(key) {
	case actClose:
		m.activeDialog = dialogNone
		m.setupWizard = nil
		return m, m.drainDeferredQueue()
	case actValidate:
		return m, setupListModelsCmd(m.setupBackend, w.input(), w.gen)
	case actApply:
		return m, setupApplyCmd(m.setupBackend, w.plan(), w.gen)
	}
	return m, nil
}

func setupListModelsCmd(b commands.SetupBackend, in commands.SetupInput, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), setupListTimeout)
		defer cancel()
		models, err := b.ListModels(ctx, in)
		return setupModelsMsg{gen: gen, models: models, err: err}
	}
}

func setupApplyCmd(b commands.SetupBackend, plan commands.SetupPlan, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), setupApplyTimeout)
		defer cancel()
		report, err := b.Apply(ctx, plan)
		return setupAppliedMsg{gen: gen, report: report, err: err}
	}
}

// onSetupModels applies a ListModels result if it belongs to the live request.
func (m AppModel) onSetupModels(msg setupModelsMsg) AppModel {
	if w := m.setupWizard; w != nil && msg.gen == w.gen {
		w.onModels(msg.models, msg.err)
	}
	return m
}

// onSetupApplied applies an Apply result if it belongs to the live request.
func (m AppModel) onSetupApplied(msg setupAppliedMsg) AppModel {
	if w := m.setupWizard; w != nil && msg.gen == w.gen {
		w.onApplied(msg.report, msg.err)
	}
	return m
}

// setupStartupNotice is shown when no usable credentials were found.
const setupStartupNotice = "No model is set up yet. Type /setup to configure an endpoint, API key and model."
