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
// While idle, that is GetMessages plus distinct markers in the input.
// While loading, the turn tail supplies tool-result blocks and any bubble
// appended after the window opened. Queued user bubbles from earlier turns
// are counted from unsentUsers, one submission at a time. Bubbles already
// written back are not scanned: their markers are image blocks in GetMessages.
func (m AppModel) sessionImageCount() int {
	n := countImages(m.queryEngine.GetMessages())
	texts := []string{m.input.Value()}
	if m.isLoading {
		tail := m.inflightTail()
		n += countImages(tail)
		texts = append(texts, messageTexts(tail)...)
		texts = append(texts, m.unsentMarkerTexts()...)
	}
	return n + distinctImageMarkers(m.pastes, texts)
}

// inflightTail is the display bubbles appended after this turn's window opened.
func (m AppModel) inflightTail() []types.Message {
	if m.inflightBase < 0 || m.inflightBase > len(m.messages) {
		return nil
	}
	return m.messages[m.inflightBase:]
}

// unsentMarkerTexts is queued user bubbles still outside GetMessages and
// outside the current turn tail. Each submission is its own text.
func (m AppModel) unsentMarkerTexts() []string {
	var out []string
	for _, idx := range m.unsentUsers {
		if idx < 0 || idx >= len(m.messages) || idx >= m.inflightBase {
			continue
		}
		out = append(out, messagePlainText(m.messages[idx]))
	}
	return out
}

// noteQueuedUser records the user bubble just appended by a mid-turn enqueue.
func (m AppModel) noteQueuedUser() AppModel {
	if len(m.messages) == 0 {
		return m
	}
	m.unsentUsers = append(m.unsentUsers, len(m.messages)-1)
	return m
}

// claimQueuedUser marks the oldest queued bubble as the one this query writes
// back. beginQuery must run first. The bubble stays unsent until release.
func (m AppModel) claimQueuedUser() AppModel {
	if len(m.unsentUsers) == 0 {
		m.inflightUser = -1
		return m
	}
	m.inflightUser = m.unsentUsers[0]
	return m
}

// releaseWrittenUser drops the bubble whose query just exited. Later queued
// bubbles stay, because their images are still not in GetMessages.
func (m AppModel) releaseWrittenUser() AppModel {
	if m.inflightUser < 0 {
		return m
	}
	m.unsentUsers = dropUserIndex(m.unsentUsers, m.inflightUser)
	m.inflightUser = -1
	return m
}

// dropUnsentUsers forgets queued bubbles when display history is replaced.
func (m AppModel) dropUnsentUsers() AppModel {
	m.unsentUsers = nil
	m.inflightUser = -1
	return m
}

func dropUserIndex(idxs []int, target int) []int {
	out := make([]int, 0, len(idxs))
	for _, idx := range idxs {
		if idx != target {
			out = append(out, idx)
		}
	}
	return out
}

func messageTexts(msgs []types.Message) []string {
	out := make([]string, len(msgs))
	for i, msg := range msgs {
		out[i] = messagePlainText(msg)
	}
	return out
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
