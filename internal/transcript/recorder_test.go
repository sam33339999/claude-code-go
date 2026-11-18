package transcript_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/session"
	"github.com/tunsuy/claude-code-go/internal/transcript"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

func textMsg(role types.Role, text string) types.Message {
	return types.Message{
		Role:    role,
		Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: &text}},
	}
}

// readBack decodes entries exactly like the production readers
// (bootstrap/session.go, commands/resume.go): filter user/assistant,
// unmarshal SerializedMessage, collect messages.
func readBack(t *testing.T, cwd, sid string) []types.Message {
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
	var msgs []types.Message
	for _, env := range entries {
		if env.Type != types.EntryTypeUser && env.Type != types.EntryTypeAssistant {
			continue
		}
		var sm types.SerializedMessage
		if err := json.Unmarshal(env.Raw, &sm); err != nil {
			t.Fatalf("decode entry: %v", err)
		}
		if sm.SessionId != types.SessionId(sid) {
			t.Errorf("entry sessionId: want %q, got %q", sid, sm.SessionId)
		}
		msgs = append(msgs, sm.Message)
	}
	return msgs
}

func TestRecorder_RoundTripCompatibleWithReaders(t *testing.T) {
	cwd := t.TempDir()
	rec, err := transcript.New(cwd, "test-version")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	sid := rec.SessionID()
	if sid == "" {
		t.Fatal("expected a session id")
	}
	msgs := []types.Message{textMsg(types.RoleUser, "hello"), textMsg(types.RoleAssistant, "hi there")}
	if err := rec.Record(msgs); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	got := readBack(t, cwd, sid)
	if len(got) != 2 || *got[0].Content[0].Text != "hello" || *got[1].Content[0].Text != "hi there" {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestRecorder_BaselineNoDuplicates(t *testing.T) {
	cwd := t.TempDir()
	rec, _ := transcript.New(cwd, "v")
	defer rec.Close()
	first := []types.Message{textMsg(types.RoleUser, "one")}
	full := append(first, textMsg(types.RoleAssistant, "two"))
	if err := rec.Record(first); err != nil {
		t.Fatal(err)
	}
	if err := rec.Record(full); err != nil {
		t.Fatal(err)
	}
	if err := rec.Record(full); err != nil { // idempotent re-pass
		t.Fatal(err)
	}
	got := readBack(t, cwd, rec.SessionID())
	if len(got) != 2 {
		t.Errorf("expected exactly 2 entries (no duplicates), got %d", len(got))
	}
}

func TestRecorder_SkipsNonConversationRoles(t *testing.T) {
	cwd := t.TempDir()
	rec, _ := transcript.New(cwd, "v")
	defer rec.Close()
	msgs := []types.Message{
		textMsg(types.RoleUser, "q"),
		{Role: types.Role("system"), Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: ptr("s")}}},
		textMsg(types.RoleAssistant, "a"),
	}
	if err := rec.Record(msgs); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, cwd, rec.SessionID())
	if len(got) != 2 {
		t.Errorf("system role must be skipped, got %d entries", len(got))
	}
}

func ptr(s string) *string { return &s }

func TestRecorder_ShrinkResetsBaseline(t *testing.T) {
	cwd := t.TempDir()
	rec, _ := transcript.New(cwd, "v")
	defer rec.Close()
	if err := rec.Record([]types.Message{textMsg(types.RoleUser, "old")}); err != nil {
		t.Fatal(err)
	}
	// /clear-style reset: engine history emptied, then a fresh turn.
	if err := rec.Record(nil); err != nil {
		t.Fatal(err)
	}
	if err := rec.Record([]types.Message{textMsg(types.RoleUser, "new")}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, cwd, rec.SessionID())
	if len(got) != 2 || *got[1].Content[0].Text != "new" {
		t.Errorf("expected old+new append-only transcript, got %+v", got)
	}
}

func TestRecorder_ResumeBaselineSkipsLoadedHistory(t *testing.T) {
	cwd := t.TempDir()
	rec, _ := transcript.New(cwd, "v")
	sid := rec.SessionID()
	history := []types.Message{textMsg(types.RoleUser, "h1"), textMsg(types.RoleAssistant, "h2")}
	if err := rec.Record(history); err != nil {
		t.Fatal(err)
	}
	rec.Close()

	resumed, err := transcript.Resume(cwd, sid, "v", len(history))
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	defer resumed.Close()
	if err := resumed.Record(history); err != nil { // re-pass loaded history: no-op
		t.Fatal(err)
	}
	if err := resumed.Record(append(history, textMsg(types.RoleUser, "h3"))); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, cwd, sid)
	if len(got) != 3 {
		t.Errorf("expected 3 entries (history not duplicated), got %d", len(got))
	}
}

func TestRecorder_SwitchSession(t *testing.T) {
	cwd := t.TempDir()
	a, _ := transcript.New(cwd, "v")
	defer a.Close()
	b, _ := transcript.New(cwd, "v")
	bSid := b.SessionID()
	b.Record([]types.Message{textMsg(types.RoleUser, "b-history")})
	b.Close()

	if err := a.SwitchSession(bSid, 1); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if a.SessionID() != bSid {
		t.Errorf("session id after switch: want %q, got %q", bSid, a.SessionID())
	}
	if err := a.Record([]types.Message{textMsg(types.RoleUser, "b-history"), textMsg(types.RoleAssistant, "b-new")}); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, cwd, bSid)
	if len(got) != 2 || *got[1].Content[0].Text != "b-new" {
		t.Errorf("switched session should append past loaded history, got %+v", got)
	}
}

func TestRecorder_NilSafe(t *testing.T) {
	var rec *transcript.Recorder
	if err := rec.Record([]types.Message{textMsg(types.RoleUser, "x")}); err != nil {
		t.Errorf("nil Record: %v", err)
	}
	if err := rec.SwitchSession("any", 0); err != nil {
		t.Errorf("nil SwitchSession: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Errorf("nil Close: %v", err)
	}
	if rec.SessionID() != "" {
		t.Error("nil SessionID should be empty")
	}
}

func TestRecorder_ConcurrentRecord(t *testing.T) {
	cwd := t.TempDir()
	rec, _ := transcript.New(cwd, "v")
	defer rec.Close()
	base := []types.Message{textMsg(types.RoleUser, "seed")}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rec.Record(base)
		}()
	}
	wg.Wait()
	got := readBack(t, cwd, rec.SessionID())
	if len(got) != 1 {
		t.Errorf("concurrent identical Records must not duplicate, got %d", len(got))
	}
}
