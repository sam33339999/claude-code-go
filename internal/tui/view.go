package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// reservedLines is the number of terminal lines reserved for non-message UI
// (status bar + input area + spinner + padding).
const reservedLines = 6

// minViewportLines is the floor for the message viewport height. Even when
// bottom extras (coordinator panel, dialogs, todos) are tall, the output area
// must stay usable rather than collapsing to a single line.
const minViewportLines = 5

// viewportHeight returns the available height for the message viewport.
func (m AppModel) viewportHeight() int {
	h := m.termHeight - reservedLines
	if h < 1 {
		h = 1
	}
	return h
}

// View is the BubbleTea View method. It renders the entire TUI to a string.
func (m AppModel) View() tea.View {
	if m.termWidth == 0 {
		return m.screenView("")
	}

	// Render bottom-anchored extras FIRST and measure them, so the message
	// viewport can shrink to keep everything inside the terminal. Without
	// this, dialogs/todo panel append below a full-height viewport and get
	// clipped off-screen (the approval dialog becomes invisible).
	dialog := m.renderActiveDialog()
	extraLines := renderedLines(dialog)

	var todoPanel string
	if todos := m.appState.GetState().Todos; len(todos) > 0 {
		todoPanel = renderTodoPanel(todos, m.theme)
		extraLines += renderedLines(todoPanel)
	}

	var coordPanel string
	if m.coordinatorMode {
		coordPanel = m.coordinatorPanel.View(m.termWidth, m.theme)
		if coordPanel != "" {
			extraLines += renderedLines(coordPanel) + 1 // trailing newline
		}
	}

	var sb strings.Builder

	// --- Coordinator panel (multi-agent) ---
	if coordPanel != "" {
		sb.WriteString(coordPanel)
		sb.WriteString("\n")
	}

	// --- Message list rendered through viewport (shrunk by extras) ---
	// Only show viewport area when there is content to display;
	// otherwise avoid filling the terminal with blank lines.
	vp := m.viewport
	if extraLines > 0 {
		h := vp.Height() - extraLines
		if h < minViewportLines {
			h = minViewportLines
		}
		vp.SetHeight(h)
	}
	vpContent := strings.TrimRight(vp.View(), "\n ")
	if vpContent != "" {
		sb.WriteString(vpContent)
		sb.WriteString("\n")
	}

	// --- Spinner / loading indicator ---
	if m.isLoading && m.showSpinner {
		sb.WriteString(m.spinner.View(m.theme))
		sb.WriteString("\n")
	}

	// --- Status bar ---
	m.statusBar.mode = m.appState.GetState().ToolPermissionContext.Mode
	sb.WriteString(m.statusBar.View(m.termWidth, m.theme))
	sb.WriteString("\n")

	// --- Todo panel (BF-07) ---
	if todoPanel != "" {
		sb.WriteString(todoPanel)
	}

	// --- Input area ---
	sb.WriteString(m.input.View(m.theme))

	// --- Active dialog (bottom-anchored, space reserved above) ---
	if dialog != "" {
		sb.WriteString("\n")
		sb.WriteString(dialog)
	}

	return m.screenView(sb.String())
}

// renderActiveDialog renders the currently open dialog ("" when none).
func (m AppModel) renderActiveDialog() string {
	switch m.activeDialog {
	case dialogPermission:
		if m.permReq != nil {
			return m.permReq.View(m.termWidth, m.theme)
		}
	case dialogCompact:
		return renderConfirmDialog(
			"Compact conversation?",
			"This will summarise the history and reduce context usage.",
			m.termWidth, m.theme,
		)
	case dialogExit:
		return renderConfirmDialog(
			"Exit Claude Code?",
			"Press y to confirm, n to cancel.",
			m.termWidth, m.theme,
		)
	case dialogConfig:
		return m.renderConfigDialog()
	case dialogQuestion:
		return renderQuestionDialog(m.questionReq, m.theme)
	}
	return ""
}

// renderedLines counts the terminal lines a rendered block occupies.
func renderedLines(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
}

// screenView wraps rendered content in a tea.View with the program's screen
// options (v2: alt-screen and mouse mode live on the View, not program options).
func (m AppModel) screenView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// syncViewportContent re-renders the message list into the viewport content
// and optionally scrolls to bottom when pinnedToBottom is true.
// The welcome header is included as part of the content so it scrolls with messages.
func (m *AppModel) syncViewportContent() {
	var contentBuilder strings.Builder

	// Always include welcome header at the top of the content
	header := m.welcomeHeader.View(m.termWidth, m.theme)
	if header != "" {
		contentBuilder.WriteString(header)
	}

	// Render messages
	msgs := m.visibleMessages()
	msgContent := MessageListView(msgs, m.termWidth, m.darkMode, m.theme, m.markdownRenderer(), m.expandedToolResults)
	contentBuilder.WriteString(msgContent)

	// Trim trailing newlines to avoid excessive blank space at bottom.
	content := strings.TrimRight(contentBuilder.String(), "\n")
	m.viewport.SetContent(content)
	if m.pinnedToBottom {
		m.viewport.GotoBottom()
	}
}

// renderQuestionDialog renders the AskUserQuestion dialog (BF-08): question,
// numbered options (selected marked), free-text answer buffer, key hints.
// Display-only apart from the HIL response callback invoked by key handling.
func renderQuestionDialog(q *questionState, theme Theme) string {
	if q == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(warningStyle(theme).Bold(true).Render("Question"))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render(q.question))
	sb.WriteString("\n")
	for i, opt := range q.options {
		marker := "  "
		if i == q.selected {
			marker = "> "
		}
		sb.WriteString(mutedStyle(theme).Render(fmt.Sprintf("%s%d. %s", marker, i+1, opt)))
		sb.WriteString("\n")
	}
	sb.WriteString(mutedStyle(theme).Render("Answer: " + q.answer))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render("(1-9 select · type answer · Enter submit · Esc dismiss)"))
	return sb.String()
}

// renderTodoPanel renders the session todo list (BF-07) above the input area.
// Display-only: reads AppState.Todos (mutex-protected store), never touches
// engine history or schemas (cache-first gate).
func renderTodoPanel(todos []types.TodoItem, theme Theme) string {
	var sb strings.Builder
	sb.WriteString(mutedStyle(theme).Bold(true).Render("Todos"))
	sb.WriteString("\n")
	for _, t := range todos {
		marker := "[ ]"
		switch t.Status {
		case types.TodoCompleted:
			marker = "[x]"
		case types.TodoInProgress:
			marker = "[>]"
		}
		sb.WriteString(mutedStyle(theme).Render("  " + marker + " " + t.Content))
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderConfigDialog renders the read-only /config settings panel (BF-05).
// Editing happens via the dedicated slash commands (/model, /theme, /vim,
// /effort) and persistent settings files; interactive in-panel editing is a
// documented follow-up (design/tui.md §4.1). Display-only: it never touches
// engine history or tool schemas (cache-first gate).
func (m AppModel) renderConfigDialog() string {
	st := m.appState.GetState()
	themeName := "light"
	if m.darkMode {
		themeName = "dark"
	}
	vim := "off"
	if m.input.vimEnabled {
		vim = "on"
	}
	effort := m.effort
	if effort == "" {
		effort = "default"
	}
	var sb strings.Builder
	sb.WriteString(warningStyle(m.theme).Bold(true).Render("Configuration"))
	sb.WriteString("\n")
	line := func(k, v string) {
		sb.WriteString(mutedStyle(m.theme).Render(fmt.Sprintf("  %-12s %s", k, v)))
		sb.WriteString("\n")
	}
	line("Model", st.MainLoopModel.ModelID)
	line("Theme", themeName)
	line("Vim mode", vim)
	line("Effort", effort)
	line("Working dir", st.WorkingDir)
	if m.sessionID != "" {
		line("Session", m.sessionID)
	}
	sb.WriteString(mutedStyle(m.theme).Render("Change: /model /theme /vim /effort · persistent: ~/.claude/settings.json"))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(m.theme).Render("(Esc to close)"))
	return sb.String()
}

// renderConfirmDialog renders a minimal yes/no confirmation dialog.
func renderConfirmDialog(title, body string, width int, theme Theme) string {
	var sb strings.Builder
	sb.WriteString(warningStyle(theme).Bold(true).Render(title))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render(body))
	sb.WriteString("\n")
	sb.WriteString(mutedStyle(theme).Render("(y/n)"))
	_ = width
	return sb.String()
}
