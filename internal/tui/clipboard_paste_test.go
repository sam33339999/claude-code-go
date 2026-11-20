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
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/msgqueue"
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

func TestCtrlV_EmptyClipboardNotice(t *testing.T) {
	m := newTestModel()
	m.clip = &fakeClip{imgErr: clipboard.ErrNoImage, text: ""}
	m.input = m.input.SetValue("keep")
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "keep" {
		t.Fatalf("input = %q, want unchanged", got)
	}
	if !strings.Contains(m.View().Content, "Clipboard is empty") {
		t.Fatalf("view missing empty notice: %q", m.View().Content)
	}
}

func TestCtrlV_InFlightBubbleHitsCap(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue(imageMarkers(m, 20))
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading {
		t.Fatal("submit did not start a turn")
	}
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("GetMessages filled without running Query")
	}
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "" {
		t.Fatalf("input = %q, want no new marker", got)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatalf("view missing cap notice: %q", m.View().Content)
	}
}

func TestCtrlV_InFlightToolResultHitsCap(t *testing.T) {
	m := newTestModel()
	m.input = m.input.SetValue("look")
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading {
		t.Fatal("submit did not start a turn")
	}
	id := "tool-inflight"
	m.messages = append(m.messages, types.Message{
		Role: types.RoleUser,
		Content: []types.ContentBlock{{
			Type:      types.ContentTypeToolResult,
			ToolUseID: &id,
			Content: []types.ContentBlock{{
				Type:   types.ContentTypeImage,
				Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="},
			}},
		}},
	})
	markers := imageMarkers(m, 19)
	m.input = m.input.SetValue(markers)
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != markers {
		t.Fatalf("input = %q, want unchanged", got)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatalf("view missing cap notice: %q", m.View().Content)
	}
}

func TestCtrlV_HistoricalBubblesNotDoubleCounted(t *testing.T) {
	m := newTestModel()
	m.queryEngine.SetMessages(messagesWithImages(19, 0))
	m.messages = append(m.messages, newUserMessage(imageMarkers(m, 19)))
	if m.isLoading {
		t.Fatal("idle paste must not be in a turn")
	}
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatal("historical bubbles were counted again")
	}
	if !m.pastes.isMarker(m.input.Value()) {
		t.Fatalf("input = %q, want a new image marker", m.input.Value())
	}
}

func TestCtrlV_QueuedPromptsStillCount(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.input = m.input.SetValue("turn")
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading {
		t.Fatal("submit did not start a turn")
	}

	m = queueUser(t, m, imageMarkers(m, 10))
	m = queueUser(t, m, imageMarkers(m, 10))
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("GetMessages filled without running Query")
	}

	// Drop the drain command so fakeEngine.Query does not write history.
	m = drainNextPrompt(t, m)
	if !m.isLoading {
		t.Fatal("drain did not start the queued prompt")
	}
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("drain wrote GetMessages")
	}

	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "" {
		t.Fatalf("input = %q, want no new marker", got)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatalf("view missing cap notice: %q", m.View().Content)
	}

	// The running prompt's images are in history. Production then starts the
	// next queued prompt, so the session is loading again. Those 10 blocks
	// must not be added to their markers, and the bubble still waiting counts.
	m.queryEngine.SetMessages(messagesWithImages(10, 0))
	m = drainNextPrompt(t, m)
	if !m.isLoading {
		t.Fatal("follow-up drain did not start")
	}
	if countImages(m.queryEngine.GetMessages()) != 10 {
		t.Fatalf("written-back images = %d, want 10", countImages(m.queryEngine.GetMessages()))
	}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "" {
		t.Fatalf("input = %q, want no new marker after write-back", got)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatal("queued bubble was dropped after write-back")
	}

	// 9 blocks + the waiting 10 markers is under the cap. Counting the
	// written-back markers as well would still refuse the paste.
	m.queryEngine.SetMessages(messagesWithImages(9, 0))
	m = applyCtrlV(t, m)
	if strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatal("written-back bubble was counted again")
	}
	if !m.pastes.isMarker(m.input.Value()) {
		t.Fatalf("input = %q, want a new image marker", m.input.Value())
	}
}

func TestCtrlV_QueuedDuplicateTextCountsBoth(t *testing.T) {
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.input = m.input.SetValue("turn")
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading {
		t.Fatal("submit did not start a turn")
	}
	text := imageMarkers(m, 10)
	m = queueUser(t, m, text)
	m = queueUser(t, m, text)
	m = drainNextPrompt(t, m)
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("drain wrote GetMessages")
	}
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "" {
		t.Fatalf("input = %q, want no new marker", got)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatalf("view missing cap notice: %q", m.View().Content)
	}
}

func TestCtrlV_MidTurnEchoCountsOnce(t *testing.T) {
	m := startQueuedTurn(t)
	text := imageMarkers(m, 10)
	m = queueUser(t, m, text)
	m = echoQueuedPrompt(t, m)
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("GetMessages filled without running Query")
	}
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatal("mid-turn echo was counted again")
	}
	if !m.pastes.isMarker(m.input.Value()) {
		t.Fatalf("input = %q, want a new image marker", m.input.Value())
	}
}

func TestCtrlV_ConsumedQueueDroppedOnDone(t *testing.T) {
	m := startQueuedTurn(t)
	m = queueUser(t, m, imageMarkers(m, 10))
	m = echoQueuedPrompt(t, m)
	m.queryEngine.SetMessages(messagesWithImages(10, 0))
	m = drainNextPrompt(t, m)
	if m.isLoading {
		t.Fatal("done path left the turn loading")
	}
	if countImages(m.queryEngine.GetMessages()) != 10 {
		t.Fatalf("history images = %d, want 10", countImages(m.queryEngine.GetMessages()))
	}
	m.input = m.input.SetValue("next")
	var cmd tea.Cmd
	m, cmd = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading {
		t.Fatal("follow-up submit did not start a turn")
	}
	if countImages(m.queryEngine.GetMessages()) != 10 {
		t.Fatal("follow-up submit wrote GetMessages")
	}
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatal("written-back queue bubble was counted again")
	}
	if !m.pastes.isMarker(m.input.Value()) {
		t.Fatalf("input = %q, want a new image marker", m.input.Value())
	}
	if len(m.unsentUsers) != 0 {
		t.Fatalf("stale unsent = %v", m.unsentUsers)
	}
}

func TestCtrlV_PromptQueuedAfterEchoStillCounts(t *testing.T) {
	m := startQueuedTurn(t)
	m = queueUser(t, m, imageMarkers(m, 10))
	m = echoQueuedPrompt(t, m)
	later := imageMarkers(m, 10)
	m = queueUser(t, m, later)
	m.queryEngine.SetMessages(messagesWithImages(10, 0))
	m = drainNextPrompt(t, m)
	if !m.isLoading {
		t.Fatal("prompt queued after the echo was not started")
	}
	if countImages(m.queryEngine.GetMessages()) != 10 {
		t.Fatal("drain wrote or cleared GetMessages")
	}
	if m.inflightUser < 0 || m.inflightUser >= len(m.messages) ||
		messagePlainText(m.messages[m.inflightUser]) != later {
		t.Fatal("claim kept the prompt drainMidTurn already consumed")
	}
	got := unsentTexts(m)
	if len(got) != 1 || got[0] != later {
		t.Fatalf("unsent = %q, want only the prompt queued after the echo", got)
	}
	m.clip = &fakeClip{image: png10(t)}
	m = applyCtrlV(t, m)
	if got := m.input.Value(); got != "" {
		t.Fatalf("input = %q, want no new marker", got)
	}
	if !strings.Contains(m.View().Content, "already has 20 images") {
		t.Fatal("prompt queued after the echo was not counted")
	}
}

func startQueuedTurn(t *testing.T) AppModel {
	t.Helper()
	m := newTestModel()
	m.msgQueue = msgqueue.NewMessageQueue()
	m.input = m.input.SetValue("turn")
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading {
		t.Fatal("submit did not start a turn")
	}
	if len(m.queryEngine.GetMessages()) != 0 {
		t.Fatal("GetMessages filled without running Query")
	}
	return m
}

// echoQueuedPrompt removes the one queued command, as drainMidTurn does, and
// appends the marker-text bubble StreamUserTurnMsg delivers.
func echoQueuedPrompt(t *testing.T, m AppModel) AppModel {
	t.Helper()
	snap := m.msgQueue.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("queue len = %d, want 1", len(snap))
	}
	if n := m.msgQueue.RemoveByIDs([]string{snap[0].ID}); n != 1 {
		t.Fatalf("removed %d commands, want 1", n)
	}
	m.streamCh = make(chan engine.Msg)
	msg := newUserMessage(snap[0].Value)
	updated, _ := m.Update(StreamUserTurnMsg{FinalMessage: &msg})
	m = updated.(AppModel)
	if messagePlainText(m.messages[len(m.messages)-1]) != snap[0].Value {
		t.Fatal("echo bubble was not appended")
	}
	return m
}

func unsentTexts(m AppModel) []string {
	out := make([]string, 0, len(m.unsentUsers))
	for _, u := range m.unsentUsers {
		if u.idx < 0 || u.idx >= len(m.messages) {
			continue
		}
		out = append(out, messagePlainText(m.messages[u.idx]))
	}
	return out
}

func queueUser(t *testing.T, m AppModel, text string) AppModel {
	t.Helper()
	if !m.isLoading {
		t.Fatal("queueUser requires a running turn")
	}
	m.input = m.input.SetValue(text)
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("queued submit started a query")
	}
	return m
}

func drainNextPrompt(t *testing.T, m AppModel) AppModel {
	t.Helper()
	updated, _ := m.Update(StreamDoneMsg{})
	return updated.(AppModel)
}

func imageMarkers(m AppModel, n int) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = m.pastes.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	}
	return strings.Join(parts, " ")
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
