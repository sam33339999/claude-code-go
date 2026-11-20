package tui

import (
	"context"
	"encoding/base64"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/clipboard"
	"github.com/tunsuy/glaude/pkg/types"
)

// clipboardReader is the clipboard surface the paste path calls.
// Tests assign a fake; newAppModel uses clipboard.New.
type clipboardReader interface {
	ReadImage(ctx context.Context) ([]byte, error)
	ReadText(ctx context.Context) (string, error)
}

// maxSessionImages is the most images one conversation may hold, counting
// history and distinct image markers still in the input.
const maxSessionImages = 20

const (
	noticeClipboardEmpty   = "Clipboard is empty"
	noticeImageUnsupported = "Pasting images is not supported on this platform"
	noticeImageCap         = "This conversation already has 20 images · /compact before pasting more"
	noticeImageTooLarge    = "Image is too large to send (limit 3.75 MB after downscaling)"
	noticeImageFormat      = "Unsupported image format"
)

// clipboardImageMsg is a normalized image ready to insert at the cursor.
type clipboardImageMsg struct {
	data      []byte
	mediaType string
}

// clipboardTextMsg is clipboard text to insert, collapsing when oversized.
type clipboardTextMsg struct {
	text string
}

// clipboardNoticeMsg is a one-shot hint. Nothing is inserted.
type clipboardNoticeMsg struct {
	text string
}

// startClipboardPaste schedules a clipboard read. The read itself runs in the command.
func (m AppModel) startClipboardPaste() (AppModel, tea.Cmd) {
	return m, readClipboardCmd(m.clip)
}

// readClipboardCmd reads an image, or text when the clipboard has none.
func readClipboardCmd(r clipboardReader) tea.Cmd {
	return func() tea.Msg {
		return readClipboard(r)
	}
}

func readClipboard(r clipboardReader) tea.Msg {
	data, err := r.ReadImage(context.Background())
	if err == nil {
		return normalizeClipboardImage(data)
	}
	if msg, stop := clipboardImageFailure(err); stop {
		return msg
	}
	return readClipboardText(r, err)
}

// clipboardImageFailure reports errors that must not fall through to text.
// ErrNoImage and ErrUnsupported return stop=false so the caller reads text.
func clipboardImageFailure(err error) (tea.Msg, bool) {
	var missing *clipboard.MissingToolError
	if errors.As(err, &missing) {
		return clipboardNoticeMsg{text: missing.Hint}, true
	}
	if errors.Is(err, clipboard.ErrNoImage) || errors.Is(err, clipboard.ErrUnsupported) {
		return nil, false
	}
	return clipboardNoticeMsg{text: clipboardReadFailed(err)}, true
}

func readClipboardText(r clipboardReader, imageErr error) tea.Msg {
	text, err := r.ReadText(context.Background())
	if err != nil {
		return clipboardNoticeMsg{text: clipboardReadFailed(err)}
	}
	if text == "" {
		return clipboardNoticeMsg{text: emptyClipboardNotice(imageErr)}
	}
	return clipboardTextMsg{text: text}
}

func emptyClipboardNotice(imageErr error) string {
	if errors.Is(imageErr, clipboard.ErrUnsupported) {
		return noticeImageUnsupported
	}
	return noticeClipboardEmpty
}

func clipboardReadFailed(err error) string {
	return "Clipboard read failed: " + err.Error()
}

func normalizeClipboardImage(data []byte) tea.Msg {
	out, media, err := clipboard.NormalizeImage(data)
	if err != nil {
		return clipboardNoticeMsg{text: normalizeImageNotice(err)}
	}
	return clipboardImageMsg{data: out, mediaType: media}
}

func normalizeImageNotice(err error) string {
	switch {
	case errors.Is(err, clipboard.ErrImageTooLarge):
		return noticeImageTooLarge
	case errors.Is(err, clipboard.ErrImageFormat):
		return noticeImageFormat
	default:
		return clipboardReadFailed(err)
	}
}

// onClipboardImage inserts a marker at the cursor that exists now.
// A dialog opened while the read was in flight drops the result.
// The cap is counted here, not when the read started.
func (m AppModel) onClipboardImage(msg clipboardImageMsg) (AppModel, tea.Cmd) {
	if m.activeDialog != dialogNone {
		return m, nil
	}
	if m.sessionImageCount() >= maxSessionImages {
		m.setPasteNotice(noticeImageCap)
		return m, nil
	}
	src := &types.ImageSource{
		Type:      "base64",
		MediaType: msg.mediaType,
		Data:      base64.StdEncoding.EncodeToString(msg.data),
	}
	m.input = m.input.InsertString(m.pastes.addImage(src))
	m.history.idx = len(m.history.entries) // editing ends history browsing
	return m, m.refreshCompletion()
}

func (m AppModel) onClipboardText(msg clipboardTextMsg) (AppModel, tea.Cmd) {
	if m.activeDialog != dialogNone {
		return m, nil
	}
	return m.insertPaste(msg.text)
}

func (m AppModel) onClipboardNotice(msg clipboardNoticeMsg) (AppModel, tea.Cmd) {
	if m.activeDialog != dialogNone {
		return m, nil
	}
	m.setPasteNotice(msg.text)
	return m, nil
}

// countImages counts top-level image blocks and image blocks nested one level
// inside tool_result content. Deeper nesting is not counted.
func countImages(msgs []types.Message) int {
	n := 0
	for _, msg := range msgs {
		for _, block := range msg.Content {
			n += imageBlockCount(block)
		}
	}
	return n
}

func imageBlockCount(block types.ContentBlock) int {
	switch block.Type {
	case types.ContentTypeImage:
		return 1
	case types.ContentTypeToolResult:
		n := 0
		for _, inner := range block.Content {
			if inner.Type == types.ContentTypeImage {
				n++
			}
		}
		return n
	default:
		return 0
	}
}

// sessionImageCount is history plus images not yet written back to the engine.
// While idle, that is GetMessages, distinct markers in the input, and distinct
// markers on every prompt still in unsentUsers. Abort and stream errors leave
// those prompts queued, including bubbles at or after inflightBase.
// unsentMarkerTexts skips that range, so the idle path must not call it.
// Ordinary history text is not scanned, and the idle path does not walk the
// turn tail. While loading, the tail supplies tool-result blocks and marker
// text except StreamUserTurnMsg echoes. Earlier queued bubbles still come from
// unsentMarkerTexts, one submission at a time. Bubbles already written back
// are not scanned: their markers are image blocks in GetMessages.
func (m AppModel) sessionImageCount() int {
	n := countImages(m.queryEngine.GetMessages())
	texts := []string{m.input.Value()}
	if m.isLoading {
		n += countImages(m.inflightTail())
		texts = append(texts, m.tailMarkerTexts()...)
		texts = append(texts, m.unsentMarkerTexts()...)
	} else {
		texts = append(texts, m.idleUnsentMarkerTexts()...)
	}
	return n + distinctImageMarkers(m.pastes, texts)
}

// idleUnsentMarkerTexts is every queued prompt still outside GetMessages.
// Unlike unsentMarkerTexts, it keeps idx >= inflightBase. It does not walk
// ordinary history bubbles.
func (m AppModel) idleUnsentMarkerTexts() []string {
	var out []string
	for _, u := range m.unsentUsers {
		if u.idx < 0 || u.idx >= len(m.messages) {
			continue
		}
		out = append(out, messagePlainText(m.messages[u.idx]))
	}
	return out
}

// inflightTail is the display bubbles appended after this turn's window opened.
func (m AppModel) inflightTail() []types.Message {
	if m.inflightBase < 0 || m.inflightBase > len(m.messages) {
		return nil
	}
	return m.messages[m.inflightBase:]
}

// queuedBubble is one user submission still absent from GetMessages.
// id is the queue command, so a mid-turn drain can drop it after write-back
// without releasing prompts that are still queued.
type queuedBubble struct {
	idx int
	id  string
}

// tailMarkerTexts is marker text from the open turn, skipping StreamUserTurnMsg
// echoes. Tool-result image blocks are counted separately from the same tail.
func (m AppModel) tailMarkerTexts() []string {
	if m.inflightBase < 0 || m.inflightBase > len(m.messages) {
		return nil
	}
	var out []string
	for i := m.inflightBase; i < len(m.messages); i++ {
		if m.isEchoed(i) {
			continue
		}
		out = append(out, messagePlainText(m.messages[i]))
	}
	return out
}

func (m AppModel) isEchoed(idx int) bool {
	for _, echo := range m.echoedUsers {
		if echo == idx {
			return true
		}
	}
	return false
}

// noteStreamUserTurn records the bubble StreamUserTurnMsg just appended.
func (m AppModel) noteStreamUserTurn() AppModel {
	if len(m.messages) == 0 {
		return m
	}
	m.echoedUsers = append(m.echoedUsers, len(m.messages)-1)
	return m
}

// unsentMarkerTexts is queued user bubbles still outside GetMessages and
// outside the current turn tail. Each submission is its own text.
func (m AppModel) unsentMarkerTexts() []string {
	var out []string
	for _, u := range m.unsentUsers {
		if u.idx < 0 || u.idx >= len(m.messages) || u.idx >= m.inflightBase {
			continue
		}
		out = append(out, messagePlainText(m.messages[u.idx]))
	}
	return out
}

// noteQueuedUser records the user bubble just appended by a mid-turn enqueue.
func (m AppModel) noteQueuedUser(id string) AppModel {
	if len(m.messages) == 0 {
		return m
	}
	m.unsentUsers = append(m.unsentUsers, queuedBubble{idx: len(m.messages) - 1, id: id})
	return m
}

// claimQueuedUser marks the oldest queued bubble as the one this query writes
// back. beginQuery must run first. The bubble stays unsent until release.
func (m AppModel) claimQueuedUser() AppModel {
	if len(m.unsentUsers) == 0 {
		m.inflightUser = -1
		return m
	}
	m.inflightUser = m.unsentUsers[0].idx
	return m
}

// releaseWrittenUser drops the bubble this query claimed, and every queued
// bubble whose command this turn already removed. Prompts still in the queue
// stay, including one submitted after a mid-turn echo.
func (m AppModel) releaseWrittenUser() AppModel {
	kept := make([]queuedBubble, 0, len(m.unsentUsers))
	for _, u := range m.unsentUsers {
		if u.idx == m.inflightUser {
			continue
		}
		if m.commandStillQueued(u.id) {
			kept = append(kept, u)
		}
	}
	m.unsentUsers = kept
	m.inflightUser = -1
	return m
}

func (m AppModel) commandStillQueued(id string) bool {
	if m.msgQueue == nil || id == "" {
		return false
	}
	for _, cmd := range m.msgQueue.Snapshot() {
		if cmd.ID == id {
			return true
		}
	}
	return false
}

// dropUnsentUsers forgets queued bubbles when display history is replaced.
func (m AppModel) dropUnsentUsers() AppModel {
	m.unsentUsers = nil
	m.echoedUsers = nil
	m.inflightUser = -1
	return m
}

func messagePlainText(msg types.Message) string {
	text := ""
	for _, block := range msg.Content {
		if block.Type == types.ContentTypeText && block.Text != nil {
			text += *block.Text
		}
	}
	return text
}

// distinctImageMarkers counts stored image markers once per id inside each
// text, then sums the texts. Two submissions of the same marker both count.
func distinctImageMarkers(pastes *pasteStore, texts []string) int {
	if pastes == nil {
		return 0
	}
	n := 0
	for _, text := range texts {
		n += imageMarkersIn(pastes, text)
	}
	return n
}

func imageMarkersIn(pastes *pasteStore, text string) int {
	seen := make(map[int]struct{})
	for _, tok := range markerRe.FindAllString(text, -1) {
		if !pastes.isMarker(tok) {
			continue
		}
		item := pastes.items[markerID(tok)]
		if item == nil || item.kind != pasteImage {
			continue
		}
		seen[markerID(tok)] = struct{}{}
	}
	return len(seen)
}
