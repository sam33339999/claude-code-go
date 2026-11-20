package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// pasteImageNotice is shown when a path that cannot carry images sees one.
const pasteImageNotice = "Images can only be attached to a normal message or /btw"

// setPasteNotice records a one-shot hint shown above the input.
func (m *AppModel) setPasteNotice(s string) {
	m.pasteNotice = s
}

// clearPasteNotice drops the hint. The next idle key does this.
func (m *AppModel) clearPasteNotice() {
	m.pasteNotice = ""
}

// noticeDroppedImages sets pasteImageNotice when expandTextOnly dropped images.
func (m *AppModel) noticeDroppedImages(dropped bool) {
	if dropped {
		m.setPasteNotice(pasteImageNotice)
	}
}

// pasteNoticeHint is the hint-row rendering of pasteNotice, or "" when unset.
func (m AppModel) pasteNoticeHint() string {
	if m.pasteNotice == "" {
		return ""
	}
	return mutedStyle(m.theme).Render(m.pasteNotice)
}

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

// tryDeleteToken removes a whole stored paste marker for backspace, delete,
// word-delete, or vim-normal x. Other keys, and a cursor that is not on a
// stored marker, return false so the key stays ordinary input.
func (m AppModel) tryDeleteToken(kp tea.KeyPressMsg) (AppModel, bool) {
	dir, ok := tokenDeleteDir(kp, m.input)
	if !ok {
		return m, false
	}
	next, deleted := m.input.DeleteToken(dir, m.pastes.isMarker)
	if !deleted {
		return m, false
	}
	m.input = next
	m.history.idx = len(m.history.entries) // editing ends history browsing
	return m, true
}

// tokenDeleteDir maps a key to a marker-delete direction. Word-delete uses
// the textarea binding (alt+backspace, ctrl+w, ctrl+backspace). x is forward
// only in vim normal mode; in insert mode it is a typed character.
func tokenDeleteDir(kp tea.KeyPressMsg, in InputModel) (deleteDir, bool) {
	switch {
	case key.Matches(kp, in.textarea.KeyMap.DeleteWordBackward):
		return delWordBackward, true
	case kp.Code == tea.KeyBackspace && kp.Mod == 0:
		return delBackward, true
	case kp.Code == tea.KeyDelete && kp.Mod == 0:
		return delForward, true
	case in.vimEnabled && in.vimMode == VimModeNormal && kp.String() == "x":
		return delForward, true
	default:
		return 0, false
	}
}
