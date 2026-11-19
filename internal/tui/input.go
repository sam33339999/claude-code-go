package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Input box geometry: a rounded border (1 cell each side) plus a 1-cell
// horizontal padding, and the "> " prompt column.
const (
	inputBorderCells = 2
	inputPaddingCell = 2
	inputPromptCells = 2
	// inputChromeWidth is everything in a box row that isn't text.
	inputChromeWidth = inputBorderCells + inputPaddingCell + inputPromptCells
	// inputMaxRows caps how tall the input grows before it scrolls.
	inputMaxRows = 8
)

// VimMode represents the current Vim editing mode.
type VimMode int

const (
	VimModeInsert VimMode = iota
	VimModeNormal
	VimModeVisual
)

// InputModel is the bottom prompt input, supporting normal and Vim key modes.
type InputModel struct {
	textarea   textarea.Model
	vimMode    VimMode
	vimEnabled bool
	// For normal-mode multi-key sequences.
	pendingKey string
}

// NewInput creates a new InputModel.
func NewInput(vimEnabled bool) InputModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message · / commands · @ files · alt+enter newline"
	ta.ShowLineNumbers = false
	ta.Prompt = "" // the "> " prompt is drawn once, beside the first row
	// Grow with the content (1..inputMaxRows rows) instead of a fixed 3 rows.
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = inputMaxRows
	// MaxHeight alone would also cap the *content* at inputMaxRows lines
	// (bubbles' legacy atContentLimit); keep it purely a visible-height cap.
	ta.MaxContentHeight = 10_000
	ta.SetHeight(1)
	// No cursor-line highlight inside the box.
	styles := ta.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(styles)
	ta.Focus()

	return InputModel{
		textarea:   ta,
		vimMode:    VimModeInsert,
		vimEnabled: vimEnabled,
	}
}

// Value returns the current input text.
func (m InputModel) Value() string {
	return m.textarea.Value()
}

// SetValue sets the input text.
func (m InputModel) SetValue(s string) InputModel {
	m.textarea.SetValue(s)
	return m
}

// SetWidth sizes the input box to occupy totalWidth terminal cells.
func (m InputModel) SetWidth(totalWidth int) InputModel {
	w := totalWidth - inputChromeWidth
	if w < 10 {
		w = 10
	}
	m.textarea.SetWidth(w)
	return m
}

// InsertNewline inserts a line break at the cursor (alt+enter). The textarea's
// own InsertNewline binding is "enter", which the app reserves for submit.
func (m InputModel) InsertNewline() InputModel {
	m.textarea.InsertString("\n")
	return m
}

// SetMaxRows caps how many rows the box grows to (short terminals).
func (m InputModel) SetMaxRows(n int) InputModel {
	if n < 1 {
		n = 1
	}
	if n > inputMaxRows {
		n = inputMaxRows
	}
	m.textarea.MaxHeight = n
	return m
}

// OnFirstLine reports whether the cursor is on the first logical line.
func (m InputModel) OnFirstLine() bool {
	return m.textarea.Line() == 0
}

// OnLastLine reports whether the cursor is on the last logical line.
func (m InputModel) OnLastLine() bool {
	return m.textarea.Line() >= m.textarea.LineCount()-1
}

// CursorAtEnd reports whether the cursor sits at the very end of the text.
func (m InputModel) CursorAtEnd() bool {
	if !m.OnLastLine() {
		return false
	}
	lines := strings.Split(m.textarea.Value(), "\n")
	return m.textarea.Column() >= len([]rune(lines[len(lines)-1]))
}

// Focus gives focus to the input.
func (m InputModel) Focus() (InputModel, tea.Cmd) {
	cmd := m.textarea.Focus()
	return m, cmd
}

// Blur removes focus from the input.
func (m InputModel) Blur() InputModel {
	m.textarea.Blur()
	return m
}

// Init returns the textarea init command.
func (m InputModel) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles key messages for the input model.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if m.vimEnabled {
		return m.updateVim(msg)
	}
	return m.updateNormal(msg)
}

// updateNormal handles keys in non-Vim mode.
func (m InputModel) updateNormal(msg tea.Msg) (InputModel, tea.Cmd) {
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// updateVim handles keys in Vim mode.
func (m InputModel) updateVim(msg tea.Msg) (InputModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	switch m.vimMode {
	case VimModeInsert:
		return m.vimInsertKey(keyMsg)
	case VimModeNormal:
		return m.vimNormalKey(keyMsg)
	default:
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
}

func (m InputModel) vimInsertKey(key tea.KeyPressMsg) (InputModel, tea.Cmd) {
	if key.Code == tea.KeyEsc {
		m.vimMode = VimModeNormal
		m.pendingKey = ""
		return m, nil
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(key)
	return m, cmd
}

func (m InputModel) vimNormalKey(key tea.KeyPressMsg) (InputModel, tea.Cmd) {
	// Handle pending sequences first.
	if m.pendingKey != "" {
		seq := m.pendingKey + key.String()
		m.pendingKey = ""
		switch seq {
		case "dd":
			// Delete line — clear the whole input.
			m.textarea.SetValue("")
			return m, nil
		case "yy":
			// Yank line — no-op in TUI context.
			return m, nil
		}
		return m, nil
	}

	switch key.String() {
	case "i":
		m.vimMode = VimModeInsert
	case "a":
		m.vimMode = VimModeInsert
		// Move cursor right.
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	case "A":
		m.vimMode = VimModeInsert
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	case "o":
		m.vimMode = VimModeInsert
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "h":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	case "l":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	case "k":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	case "j":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	case "0":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	case "$":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	case "d", "y":
		// Start two-key sequence.
		m.pendingKey = key.String()
	case "x":
		m.textarea, _ = m.textarea.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
	case "p":
		// Paste — no-op without clipboard integration.
	}
	return m, nil
}

// VimIndicator returns the styled Vim mode label ("" when Vim is off).
func (m InputModel) VimIndicator(theme Theme) string {
	if !m.vimEnabled {
		return ""
	}
	switch m.vimMode {
	case VimModeNormal:
		return warningStyle(theme).Render("-- NORMAL --")
	case VimModeVisual:
		return accentStyle(theme).Render("-- VISUAL --")
	default:
		return successStyle(theme).Render("-- INSERT --")
	}
}

// View renders the input as a rounded box: "> " beside the first row, the
// textarea (which grows with its content) to its right.
func (m InputModel) View(theme Theme) string {
	rows := strings.Count(m.textarea.View(), "\n") + 1
	prompt := accentStyle(theme).Bold(true).Render(">") + " " + strings.Repeat("\n", rows-1)
	body := lipgloss.JoinHorizontal(lipgloss.Top, prompt, m.textarea.View())
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Muted).
		Padding(0, 1)
	return box.Render(body)
}

// IsSlashCommand returns true when the current value starts with '/'.
func (m InputModel) IsSlashCommand() bool {
	return strings.HasPrefix(strings.TrimSpace(m.textarea.Value()), "/")
}

// SlashPrefix returns the current /command prefix for Tab-completion.
func (m InputModel) SlashPrefix() string {
	v := strings.TrimSpace(m.textarea.Value())
	if !strings.HasPrefix(v, "/") {
		return ""
	}
	// Return everything up to the first space.
	if idx := strings.Index(v, " "); idx >= 0 {
		return v[:idx]
	}
	return v
}
