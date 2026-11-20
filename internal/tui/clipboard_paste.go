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
// While loading, bubbles from the turn-start index (and a queued user bubble
// from the previous turn) are included too. Older bubbles are not scanned:
// their images are already blocks inside GetMessages.
func (m AppModel) sessionImageCount() int {
	n := countImages(m.queryEngine.GetMessages())
	texts := []string{m.input.Value()}
	if m.isLoading {
		tail := m.inflightTail()
		n += countImages(tail)
		texts = append(texts, messageTexts(tail)...)
		if msg, ok := m.pendingInflightMessage(); ok {
			texts = append(texts, messagePlainText(msg))
		}
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

// pendingInflightMessage is the queued user bubble from the previous turn.
func (m AppModel) pendingInflightMessage() (types.Message, bool) {
	i := m.inflightPending
	if i < 0 || i >= len(m.messages) || i >= m.inflightBase {
		return types.Message{}, false
	}
	return m.messages[i], true
}

// noteInflightUser records a user bubble already on screen whose text matches
// the prompt about to run. beginQuery must run first. A bubble inside the new
// window is left alone so its markers are not counted twice.
func (m AppModel) noteInflightUser(text string) AppModel {
	idx := userBubbleIndex(m.messages, text)
	if idx >= 0 && idx < m.inflightBase {
		m.inflightPending = idx
	}
	return m
}

func userBubbleIndex(msgs []types.Message, text string) int {
	found := -1
	for i, msg := range msgs {
		if msg.Role != types.RoleUser {
			continue
		}
		if messagePlainText(msg) == text {
			found = i
		}
	}
	return found
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

// distinctImageMarkers counts stored image markers in texts, once per id.
func distinctImageMarkers(pastes *pasteStore, texts []string) int {
	if pastes == nil {
		return 0
	}
	seen := make(map[int]struct{})
	for _, text := range texts {
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
	}
	return len(seen)
}
