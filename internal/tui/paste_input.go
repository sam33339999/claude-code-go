package tui

import (
	tea "charm.land/bubbletea/v2"
)

// insertPaste inserts text at the input cursor. Over-threshold text is stored
// and replaced by its marker; shorter text is inserted with newlines normalized.
func (m AppModel) insertPaste(text string) (AppModel, tea.Cmd) {
	if shouldCollapse(text) {
		text = m.pastes.addText(text)
	} else {
		text = normalizeNewlines(text)
	}
	m.input = m.input.InsertString(text)
	m.history.idx = len(m.history.entries) // editing ends history browsing
	return m, m.refreshCompletion()
}
