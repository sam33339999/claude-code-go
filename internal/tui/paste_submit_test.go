package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/msgqueue"
	"github.com/tunsuy/glaude/pkg/types"
)

func pngPaste() *types.ImageSource {
	return &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="}
}

func lastQuery(t *testing.T, be *btwEngine) types.Message {
	t.Helper()
	if len(be.queries) == 0 {
		t.Fatal("no query")
	}
	msgs := be.queries[len(be.queries)-1].Messages
	if len(msgs) == 0 {
		t.Fatal("query has no messages")
	}
	return msgs[len(msgs)-1]
}

func TestSubmit_ExpandsTextAndAttachesImages(t *testing.T) {
	m, be := newBTWModel(t, nil)
	img := m.pastes.addImage(pngPaste())
	body := "pasted-body-text\n" + strings.Repeat("z", 20)
	txt := m.pastes.addText(body)
	input := "look " + img + " " + txt
	m, cmd := submit(m, input)
	runCmds(cmd)

	last := lastQuery(t, be)
	if len(last.Content) < 2 || last.Content[0].Type != types.ContentTypeImage {
		t.Fatalf("content = %+v, want image then text", last.Content)
	}
	gotText := ""
	if last.Content[1].Text != nil {
		gotText = *last.Content[1].Text
	}
	if !strings.Contains(gotText, body) || !strings.Contains(gotText, "[Image #1]") {
		t.Fatalf("text = %q, want pasted body and literal [Image #1]", gotText)
	}
	if got := lastDisplayText(m); got != input {
		t.Errorf("display = %q, want marker input %q", got, input)
	}
}

func TestSubmit_QueuedPromptExpands(t *testing.T) {
	m, be := newBTWModel(t, nil)
	img := m.pastes.addImage(pngPaste())
	body := "queued-body"
	txt := m.pastes.addText(body)
	input := "see " + img + " " + txt
	m.isLoading = true
	m, cmd := submit(m, input)
	if cmd != nil {
		t.Fatal("loading submit must queue, not start a query")
	}
	snap := m.msgQueue.Snapshot()
	if len(snap) != 1 || snap[0].Value != input || snap[0].Origin != "" {
		t.Fatalf("queue = %+v, want raw marker text and empty origin", snap)
	}
	if len(snap[0].Content) < 2 || snap[0].Content[0].Type != types.ContentTypeImage {
		t.Fatalf("snapshot = %+v, want image then text", snap[0].Content)
	}
	snapText := ""
	if snap[0].Content[1].Text != nil {
		snapText = *snap[0].Content[1].Text
	}
	if !strings.Contains(snapText, body) || strings.Contains(snap[0].Value, body) {
		t.Fatalf("snapshot text = %q, value = %q", snapText, snap[0].Value)
	}

	m.isLoading = false
	runCmds(processQueueCmd(&m))
	last := lastQuery(t, be)
	if len(last.Content) < 2 || last.Content[0].Type != types.ContentTypeImage {
		t.Fatalf("content = %+v, want image then text", last.Content)
	}
	gotText := ""
	if last.Content[1].Text != nil {
		gotText = *last.Content[1].Text
	}
	if !strings.Contains(gotText, body) || !strings.Contains(gotText, img) {
		t.Fatalf("text = %q, want expanded body and literal image marker", gotText)
	}
}

func TestSubmit_NonUserSourcesNotExpanded(t *testing.T) {
	assertLiteral := func(t *testing.T, be *btwEngine, marker string) {
		t.Helper()
		last := lastQuery(t, be)
		if len(last.Content) != 1 || last.Content[0].Type != types.ContentTypeText || last.Content[0].Text == nil {
			t.Fatalf("content = %+v, want one text block", last.Content)
		}
		if !strings.Contains(*last.Content[0].Text, marker) {
			t.Errorf("text = %q, want literal %s", *last.Content[0].Text, marker)
		}
	}

	t.Run("btw origin", func(t *testing.T) {
		m, be := newBTWModel(t, nil)
		marker := m.pastes.addImage(pngPaste())
		if marker != "[Image #1]" {
			t.Fatalf("marker = %q", marker)
		}
		value := "note " + marker
		queued := msgqueue.NewCommand(value, msgqueue.ModePrompt, msgqueue.PriorityNext)
		queued.Origin = msgqueue.OriginBTW
		m.msgQueue.Enqueue(queued)
		// Reclaim holds a btw note so it never starts a turn. The ModePrompt
		// branch would still send it with fromUser == (origin == "").
		runCmds(processQueueCmd(&m))
		if len(be.queries) != 0 {
			t.Fatal("OriginBTW must not start a turn")
		}
		if !strings.Contains(m.btwPending, "[Image #1]") {
			t.Errorf("reclaimed note = %q, want literal marker", m.btwPending)
		}
		// The reclaimed note is pending, not this send. fromUser follows origin.
		m.btwPending = ""
		runCmds(startQueryCmd(&m, value, queued.Origin == ""))
		assertLiteral(t, be, "[Image #1]")
	})

	t.Run("external", func(t *testing.T) {
		m, be := newBTWModel(t, nil)
		marker := m.pastes.addImage(pngPaste())
		value := "event " + marker
		m.msgQueue.Enqueue(msgqueue.NewCommand(value, msgqueue.ModeExternalPrompt, msgqueue.PriorityNext))
		runCmds(processQueueCmd(&m))
		assertLiteral(t, be, "[Image #1]")
	})
}

func TestSubmit_BangExpandsTextDropsImages(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	body := "echo hello-from-paste"
	txt := m.pastes.addText(body)
	img := m.pastes.addImage(pngPaste())
	m, cmd := submit(m, "!"+txt+" "+img)
	if cmd == nil {
		t.Fatal("expected a bash command")
	}
	if got := lastDisplayText(m); !strings.Contains(got, txt) || strings.Contains(got, body) {
		t.Errorf("bubble = %q, want text marker %s, not the pasted body", got, txt)
	}
	msg, ok := cmd().(bashResultMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want bashResultMsg", msg)
	}
	if !strings.Contains(msg.command, body) || !strings.Contains(msg.command, img) {
		t.Errorf("command = %q, want original text and literal image marker", msg.command)
	}
	if strings.Contains(msg.command, txt) {
		t.Errorf("text marker not expanded: %q", msg.command)
	}
	if !strings.Contains(m.View().Content, "Images can only be attached") {
		t.Error("notice missing from view")
	}
	m, _ = press(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if strings.Contains(m.View().Content, "Images can only be attached") {
		t.Error("notice must clear on the next key")
	}
}

func TestSubmit_SlashMarkerDoesNotExpandIntoCommand(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.messages = append(m.messages, newUserMessage("keep-this-history"))
	marker := m.pastes.addText("clear now")
	m, _ = submit(m, "/"+marker)
	found := false
	for _, msg := range m.messages {
		for _, b := range msg.Content {
			if b.Text != nil && strings.Contains(*b.Text, "keep-this-history") {
				found = true
			}
			if b.Text != nil && strings.Contains(*b.Text, "Conversation cleared") {
				t.Fatalf("expanded body ran /clear: %q", *b.Text)
			}
		}
	}
	if !found {
		t.Fatal("history was cleared")
	}
}

func TestSubmit_BTWAttachesImages(t *testing.T) {
	m, be := newBTWModel(t, nil)
	img := m.pastes.addImage(pngPaste())
	m, _ = submit(m, "/btw what is "+img)
	last := lastQuery(t, be)
	if len(last.Content) == 0 || last.Content[0].Type != types.ContentTypeImage {
		t.Fatalf("content = %+v, want image first", last.Content)
	}
	if !strings.Contains(btwBlock(m), img) {
		t.Errorf("display question expanded: %q", btwBlock(m))
	}
}
