package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tunsuy/agtkeel/session"
	"github.com/tunsuy/agtkeel/types"
	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/internal/transcript"
)

// runCmdFully executes a Cmd, draining tea.BatchMsg children, and returns
// the first non-nil message produced (if any).
func runCmdFully(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if m := runCmdFully(c); m != nil {
				return m
			}
		}
		return nil
	}
	return msg
}

func countEntries(t *testing.T, cwd, sid string) int {
	t.Helper()
	_, mgr, err := session.Resume(sid, cwd)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	defer mgr.Close()
	entries, err := mgr.ReadAll()
	if err != nil {
		t.Fatalf("readall: %v", err)
	}
	return len(entries)
}

// TestStreamDone_PersistsTurn proves the BF-18 turn-boundary wiring: a
// completed query returns a Cmd that writes the engine history to JSONL.
func TestStreamDone_PersistsTurn(t *testing.T) {
	cwd := t.TempDir()
	rec, err := transcript.New(cwd, "test")
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer rec.Close()

	m := newTestModel()
	m.transcript = rec
	text := "hello"
	m.queryEngine.SetMessages([]types.Message{
		newUserMessage(text),
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: &text}}},
	})

	_, cmd := m.Update(StreamDoneMsg{})
	if cmd == nil {
		t.Fatal("expected a persistence Cmd after StreamDone")
	}
	if msg := runCmdFully(cmd); msg != nil {
		if te, ok := msg.(TranscriptErrMsg); ok {
			t.Fatalf("persistence failed: %v", te.Err)
		}
	}
	if n := countEntries(t, cwd, rec.SessionID()); n != 2 {
		t.Errorf("expected 2 persisted entries, got %d", n)
	}
}

// TestStreamDone_NilRecorderNoOp proves tests/sessions without persistence
// still work (nil recorder is a safe no-op).
func TestStreamDone_NilRecorderNoOp(t *testing.T) {
	m := newTestModel() // transcript == nil
	m.queryEngine.SetMessages([]types.Message{newUserMessage("x")})
	_, cmd := m.Update(StreamDoneMsg{})
	if msg := runCmdFully(cmd); msg != nil {
		if te, ok := msg.(TranscriptErrMsg); ok {
			t.Fatalf("nil recorder must be a no-op, got %v", te.Err)
		}
	}
}

// TestResume_SwitchesTranscript proves mid-session /resume adopts the loaded
// session file: later turns append there, not to the original session.
func TestResume_SwitchesTranscript(t *testing.T) {
	cwd := t.TempDir()
	// Target session with one history message.
	target, err := transcript.New(cwd, "test")
	if err != nil {
		t.Fatal(err)
	}
	targetSid := target.SessionID()
	if err := target.Record([]types.Message{newUserMessage("old question")}); err != nil {
		t.Fatal(err)
	}
	target.Close()

	// Live session A.
	recA, err := transcript.New(cwd, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer recA.Close()

	m := newTestModel()
	m.transcript = recA
	m.appState.SetState(func(prev state.AppState) state.AppState {
		prev.WorkingDir = cwd
		return prev
	})

	updated, cmd := m.handleSlashCommand("/resume " + targetSid)
	if cmd == nil {
		t.Fatal("expected a switch Cmd from /resume")
	}
	if msg := runCmdFully(cmd); msg != nil {
		if te, ok := msg.(TranscriptErrMsg); ok {
			t.Fatalf("switch failed: %v", te.Err)
		}
	}
	if recA.SessionID() != targetSid {
		t.Fatalf("recorder should adopt %q, got %q", targetSid, recA.SessionID())
	}
	if updated.sessionID != targetSid {
		t.Errorf("model sessionID should adopt %q, got %q", targetSid, updated.sessionID)
	}

	// A new turn after resume appends to the target file (history + new msg).
	turn := append(updated.queryEngine.GetMessages(), newUserMessage("follow-up"))
	m.queryEngine.SetMessages(turn)
	_, cmd = m.Update(StreamDoneMsg{})
	runCmdFully(cmd)
	if n := countEntries(t, cwd, targetSid); n != 2 {
		t.Errorf("expected history+follow-up=2 entries in target session, got %d", n)
	}
}
