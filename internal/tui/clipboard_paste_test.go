package tui

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/clipboard"
	"github.com/tunsuy/glaude/pkg/types"
)

// fakeClip is a clipboardReader stand-in. imgErr short-circuits ReadImage.
type fakeClip struct {
	image     []byte
	imgErr    error
	text      string
	textErr   error
	textCalls int
}

func (f *fakeClip) ReadImage(context.Context) ([]byte, error) {
	if f.imgErr != nil {
		return nil, f.imgErr
	}
	return f.image, nil
}

func (f *fakeClip) ReadText(context.Context) (string, error) {
	f.textCalls++
	if f.textErr != nil {
		return "", f.textErr
	}
	return f.text, nil
}

func ctrlVKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
}

// applyCtrlV sends ctrl+v, runs the returned command, and feeds its message back.
func applyCtrlV(t *testing.T, m AppModel) AppModel {
	t.Helper()
	key := ctrlVKey()
	if key.String() != "ctrl+v" {
		t.Fatalf("ctrl+v String() = %q", key.String())
	}
	m, cmd := press(m, key)
	if cmd == nil {
		t.Fatal("ctrl+v produced no command")
	}
	msg := cmd()
	if msg == nil {
		t.Fatal("clipboard command produced no message")
	}
	updated, _ := m.Update(msg)
	return updated.(AppModel)
}

func png10(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func TestCtrlV_ImageInsertsMarker(t *testing.T) {
	raw := png10(t)
	m := newTestModel()
	m.clip = &fakeClip{image: raw}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "[Image #1]" {
		t.Fatalf("input = %q, want [Image #1]", got)
	}
	if !m.pastes.isMarker("[Image #1]") {
		t.Fatal("stored marker missing")
	}
	item := m.pastes.items[1]
	if item == nil || item.image == nil || item.image.Type != "base64" || item.image.MediaType != "image/png" {
		t.Fatalf("image source = %+v", item)
	}
	got, err := base64.StdEncoding.DecodeString(item.image.Data)
	if err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatal("stored image bytes differ from the normalized png")
	}
}

func TestCtrlV_TextFallbackCollapses(t *testing.T) {
	m := newTestModel()
	body := strings.Repeat("a", 900)
	fake := &fakeClip{imgErr: clipboard.ErrNoImage, text: body}
	m.clip = fake
	m = applyCtrlV(t, m)
	if fake.textCalls != 1 {
		t.Fatalf("ReadText calls = %d, want 1", fake.textCalls)
	}
	if got := m.input.Value(); got != "[Pasted text #1 900 chars]" {
		t.Fatalf("input = %q", got)
	}
}

func TestCtrlV_TextareaBuiltinDisabled(t *testing.T) {
	if NewInput(false).textarea.KeyMap.Paste.Enabled() {
		t.Fatal("textarea builtin paste must be disabled")
	}
}

func TestCtrlV_SessionImageCap(t *testing.T) {
	m := newTestModel()
	m.queryEngine.SetMessages(messagesWithImages(10, 9))
	marker := m.pastes.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	m.input = m.input.SetValue(marker)
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != marker {
		t.Fatalf("input = %q, want unchanged %q", got, marker)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatalf("view missing cap notice: %q", m.View().Content)
	}
}

// messagesWithImages builds one user message with top image blocks plus
// nested images inside a single tool_result. Text blocks are not images.
func messagesWithImages(top, nested int) []types.Message {
	blocks := []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("look")}}
	for range top {
		blocks = append(blocks, types.ContentBlock{
			Type:   types.ContentTypeImage,
			Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="},
		})
	}
	id := "tool-1"
	inner := make([]types.ContentBlock, 0, nested)
	for range nested {
		inner = append(inner, types.ContentBlock{
			Type:   types.ContentTypeImage,
			Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "QQ=="},
		})
	}
	blocks = append(blocks, types.ContentBlock{
		Type:      types.ContentTypeToolResult,
		ToolUseID: &id,
		Content:   inner,
	})
	return []types.Message{{Role: types.RoleUser, Content: blocks}}
}

func TestCtrlV_ResultInsertedAtCurrentCursor(t *testing.T) {
	raw := png10(t)

	m := newTestModel()
	m.clip = &fakeClip{image: raw}
	key := ctrlVKey()
	m, cmd := press(m, key)
	if cmd == nil {
		t.Fatal("ctrl+v produced no command")
	}
	m.input = m.input.SetValue("ab")
	m.input.textarea.SetCursorColumn(1)
	updated, _ := m.Update(cmd())
	m = updated.(AppModel)
	if got := m.input.Value(); got != "a[Image #1]b" {
		t.Fatalf("input = %q, want a[Image #1]b", got)
	}

	m = newTestModel()
	m.clip = &fakeClip{image: raw}
	m, cmd = press(m, key)
	if cmd == nil {
		t.Fatal("ctrl+v produced no command")
	}
	m.input = m.input.SetValue("sent")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.input.Value() != "" {
		t.Fatalf("input after submit = %q, want empty", m.input.Value())
	}
	before := len(m.messages)
	updated, _ = m.Update(cmd())
	m = updated.(AppModel)
	if got := m.input.Value(); got != "[Image #1]" {
		t.Fatalf("input = %q, want [Image #1]", got)
	}
	if len(m.messages) != before {
		t.Fatalf("messages %d → %d; paste must not append a submitted turn", before, len(m.messages))
	}
	for _, msg := range m.messages {
		for _, block := range msg.Content {
			if block.Text != nil && strings.Contains(*block.Text, "[Image #") {
				t.Fatalf("submitted message contains marker: %q", *block.Text)
			}
			if block.Type == types.ContentTypeImage {
				t.Fatal("submitted message contains an image block")
			}
		}
	}
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("clipboard result wrote engine history")
	}
}

func TestCtrlV_MissingToolNotice(t *testing.T) {
	const hint = "Install wl-clipboard or xclip to paste from the clipboard"
	m := newTestModel()
	fake := &fakeClip{imgErr: &clipboard.MissingToolError{Hint: hint}}
	m.clip = fake
	m.input = m.input.SetValue("keep")
	m = applyCtrlV(t, m)
	if fake.textCalls != 0 {
		t.Fatalf("ReadText calls = %d, want 0", fake.textCalls)
	}
	if got := m.input.Value(); got != "keep" {
		t.Fatalf("input = %q, want unchanged", got)
	}
	if !strings.Contains(m.View().Content, hint) {
		t.Fatalf("view missing hint: %q", m.View().Content)
	}
}
