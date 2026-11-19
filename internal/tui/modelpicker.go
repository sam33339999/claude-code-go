package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/compact"
	"github.com/tunsuy/glaude/internal/state"
)

// modelPicker is the /model menu: "default" plus the tiers. Unavailable
// rows are shown with their reason and skipped by the cursor.
type modelPicker struct {
	opts   []commands.ModelOption
	cursor int
}

func newModelPicker(opts []commands.ModelOption) *modelPicker {
	p := &modelPicker{opts: opts}
	for i, o := range opts {
		if o.Current {
			p.cursor = i
			return p
		}
	}
	p.move(0)
	return p
}

// move steps the cursor by delta (0 = settle on an available row), skipping
// unavailable rows.
func (p *modelPicker) move(delta int) {
	n := len(p.opts)
	if n == 0 {
		return
	}
	step := delta
	if step == 0 {
		step = 1
		if p.opts[p.cursor].Available {
			return
		}
	}
	for range n {
		p.cursor = (p.cursor + step + n) % n
		if p.opts[p.cursor].Available {
			return
		}
	}
}

func (p *modelPicker) view(width int, theme Theme) string {
	var sb strings.Builder
	sb.WriteString(primaryStyle(theme).Bold(true).Render("Switch main model (this session only)") + "\n\n")
	for i, o := range p.opts {
		label := fmt.Sprintf("%-8s %s", o.Name, o.Label())
		if o.ContextWindow > 0 {
			label += "  " + compactCount(o.ContextWindow)
		}
		switch {
		case !o.Available:
			sb.WriteString(mutedStyle(theme).Render("  "+label+"  ("+o.Reason+")") + "\n")
		case i == p.cursor:
			line := "❯ " + label
			if o.Current {
				line += "  (current)"
			}
			sb.WriteString(primaryStyle(theme).Render(line) + "\n")
		default:
			if o.Current {
				label += "  (current)"
			}
			sb.WriteString("  " + label + "\n")
		}
	}
	sb.WriteString("\n" + mutedStyle(theme).Render("↑/↓ select · Enter switch · Esc cancel"))
	return dialogBox(sb.String(), width, theme)
}

// openModelPicker handles "/model" without arguments.
func (m AppModel) openModelPicker() AppModel {
	if m.activeDialog == dialogModelConfirm {
		return m // a switch is awaiting confirmation; don't stack dialogs
	}
	if m.modelSwitcher == nil {
		m.messages = append(m.messages, newSystemMessage("Current model: "+m.appState.GetState().MainLoopModel.ModelID))
		m.syncViewportContent()
		return m
	}
	if m.isLoading {
		return m.systemNotice("Cannot switch the model while a query is running. Wait for it to finish or press Esc to interrupt it.")
	}
	m.modelPicker = newModelPicker(m.modelSwitcher.Options())
	m.activeDialog = dialogModelPicker
	return m
}

// switchModelRef handles "/model <ref>".
func (m AppModel) switchModelRef(ref string) AppModel {
	if m.activeDialog == dialogModelConfirm {
		return m
	}
	if m.modelSwitcher == nil {
		m.queryEngine.SetModel(ref)
		m.statusBar.model = ref
		m.statusBar.contextWindow = compact.ResolveContextWindow(ref, mainContextWindow(m.tiers))
		m.appState.SetState(func(st state.AppState) state.AppState {
			st.MainLoopModel.ModelID = ref
			return st
		})
		return m.systemNotice(fmt.Sprintf("Model set to %q.", ref))
	}
	opt, err := m.modelSwitcher.Resolve(ref)
	if err != nil {
		return m.systemError("Error: " + err.Error())
	}
	return m.requestModelSwitch(opt)
}

// requestModelSwitch applies opt, asking for confirmation first when the
// conversation has content (the switch invalidates the prompt cache).
func (m AppModel) requestModelSwitch(opt commands.ModelOption) AppModel {
	if m.isLoading {
		return m.systemNotice("Cannot switch the model while a query is running. Wait for it to finish or press Esc to interrupt it.")
	}
	if opt.Current {
		return m.systemNotice("Already on " + opt.Label() + ".")
	}
	if len(m.queryEngine.GetMessages()) > 0 {
		o := opt
		m.pendingModel = &o
		m.activeDialog = dialogModelConfirm
		return m
	}
	return m.applyModelSwitch(opt)
}

func (m AppModel) applyModelSwitch(opt commands.ModelOption) AppModel {
	if err := m.modelSwitcher.Apply(opt); err != nil {
		return m.systemError("Error: " + err.Error())
	}
	m.statusBar.model = opt.Model
	m.statusBar.contextWindow = compact.ResolveContextWindow(opt.Model, opt.ContextWindow)
	name := opt.Name
	if name == "" {
		name = "custom"
	}
	return m.systemNotice(fmt.Sprintf("Main model: %s → %s (session only).", name, opt.Label()))
}

func (m AppModel) handleModelPickerKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.modelPicker
	if p == nil {
		m.activeDialog = dialogNone
		return m, nil
	}
	switch key.String() {
	case "up", "k", "shift+tab":
		p.move(-1)
	case "down", "j", "tab":
		p.move(1)
	case "esc", "q", "ctrl+g":
		m.activeDialog, m.modelPicker = dialogNone, nil
	case "enter", "ctrl+j":
		opt := p.opts[p.cursor]
		m.activeDialog, m.modelPicker = dialogNone, nil
		if opt.Available {
			m = m.requestModelSwitch(opt)
		}
	}
	return m, nil
}

func (m AppModel) handleModelConfirmKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	opt := m.pendingModel
	switch {
	case opt == nil:
		m.activeDialog = dialogNone
	case key.String() == "y" || key.String() == "Y":
		m.activeDialog, m.pendingModel = dialogNone, nil
		m = m.applyModelSwitch(*opt)
	case key.String() == "n" || key.String() == "N" || key.Code == tea.KeyEsc:
		m.activeDialog, m.pendingModel = dialogNone, nil
	}
	return m, nil
}

// renderModelConfirm explains the cache cost of switching.
func (m AppModel) renderModelConfirm() string {
	opt := m.pendingModel
	if opt == nil {
		return ""
	}
	est := compact.EstimateTokens(m.queryEngine.GetMessages())
	var sb strings.Builder
	sb.WriteString(primaryStyle(m.theme).Bold(true).Render("Switch main model to "+opt.Name+" ("+opt.Label()+")?") + "\n\n")
	sb.WriteString("The prompt cache is tied to the current model and provider: the next\n")
	sb.WriteString(fmt.Sprintf("request re-sends the whole conversation (≈%s tokens) without cache.\n", compactCount(est)))
	target := compact.Target{Model: opt.Model, ContextWindow: opt.ContextWindow, MaxOutputTokens: opt.MaxTokens}
	if est >= compact.AutoCompactThreshold(target) {
		sb.WriteString(warningStyle(m.theme).Render("⚠ The conversation is larger than the new model's window: it will be compacted\n  on the next turn and the oldest part may be trimmed.") + "\n")
	}
	sb.WriteString("\n" + mutedStyle(m.theme).Render("y switch · n cancel"))
	return dialogBox(sb.String(), m.dialogWidth(), m.theme)
}

func (m AppModel) systemNotice(text string) AppModel {
	m.messages = append(m.messages, newSystemMessage(text))
	m.syncViewportContent()
	return m
}

func (m AppModel) systemError(text string) AppModel {
	m.messages = append(m.messages, newSystemErrorMessage(text))
	m.syncViewportContent()
	return m
}
