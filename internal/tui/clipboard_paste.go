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
	if countImages(m.queryEngine.GetMessages())+m.distinctInputImages() >= maxSessionImages {
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

// distinctInputImages counts stored image markers in the input, once per id.
func (m AppModel) distinctInputImages() int {
	if m.pastes == nil {
		return 0
	}
	seen := make(map[int]struct{})
	for _, tok := range markerRe.FindAllString(m.input.Value(), -1) {
		if !m.pastes.isMarker(tok) {
			continue
		}
		item := m.pastes.items[markerID(tok)]
		if item == nil || item.kind != pasteImage {
			continue
		}
		seen[markerID(tok)] = struct{}{}
	}
	return len(seen)
}
