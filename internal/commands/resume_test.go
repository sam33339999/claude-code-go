package commands_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/commands"
	"github.com/tunsuy/claude-code-go/internal/session"
	"github.com/tunsuy/claude-code-go/pkg/types"
	utilfs "github.com/tunsuy/claude-code-go/pkg/utils/fs"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// testSessionEntry combines the JSONL envelope type with the persisted
// message snapshot (same shape the CLI --resume path reads).
type testSessionEntry struct {
	Type string `json:"type"`
	types.SerializedMessage
}

// writeTestSession creates a session under cwd with alternating user/assistant
// text messages and returns its ID.
func writeTestSession(t *testing.T, cwd string, texts ...string) string {
	t.Helper()
	sid, mgr, err := session.New(cwd)
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	for i, text := range texts {
		role := types.RoleUser
		if i%2 == 1 {
			role = types.RoleAssistant
		}
		text := text
		entry := testSessionEntry{
			Type: string(role),
			SerializedMessage: types.SerializedMessage{
				Message: types.Message{
					Role: role,
					Content: []types.ContentBlock{
						{Type: types.ContentTypeText, Text: &text},
					},
				},
				SessionId: sid,
			},
		}
		if err := mgr.AppendEntry(entry); err != nil {
			t.Fatalf("AppendEntry: %v", err)
		}
	}
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close: %v", err)
	}
	return string(sid)
}

// sessionFilePath returns the JSONL path for a session ID under cwd.
func sessionFilePath(t *testing.T, cwd, sid string) string {
	t.Helper()
	hash, err := utilfs.ProjectHash(cwd)
	if err != nil {
		t.Fatalf("ProjectHash: %v", err)
	}
	return filepath.Join(cwd, ".claude", "projects", hash, sid+".jsonl")
}

// setSessionModTime forces a deterministic modification time on a session file.
func setSessionModTime(t *testing.T, cwd, sid string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(sessionFilePath(t, cwd, sid), ts, ts); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
}

func resumeCommand(t *testing.T) *commands.Command {
	t.Helper()
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Lookup("resume")
	if cmd == nil {
		t.Fatal("expected /resume to be registered")
	}
	return cmd
}

// ---------------------------------------------------------------------------
// Bare /resume — listing
// ---------------------------------------------------------------------------

func TestResumeCommand_NoSessions(t *testing.T) {
	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: t.TempDir()}, "")
	if result.Display != commands.DisplayMessage {
		t.Errorf("expected DisplayMessage, got %v", result.Display)
	}
	if !strings.Contains(result.Text, "No previous sessions") {
		t.Errorf("expected friendly empty message, got %q", result.Text)
	}
}

func TestResumeCommand_ListsSessionsNewestFirst(t *testing.T) {
	cwd := t.TempDir()
	older := writeTestSession(t, cwd, "older session question", "older answer")
	newer := writeTestSession(t, cwd, "newer session question")
	base := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	setSessionModTime(t, cwd, older, base.Add(-time.Hour))
	setSessionModTime(t, cwd, newer, base)

	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: cwd}, "")
	if result.Display != commands.DisplayMessage {
		t.Fatalf("expected DisplayMessage, got %v (text: %q)", result.Display, result.Text)
	}
	for _, want := range []string{older, newer, "newer session question", "2 msgs", "1 msgs", "/resume <id>"} {
		if !strings.Contains(result.Text, want) {
			t.Errorf("expected %q in listing, got %q", want, result.Text)
		}
	}
	if strings.Index(result.Text, newer) > strings.Index(result.Text, older) {
		t.Errorf("expected newest session first, got %q", result.Text)
	}
}

func TestResumeCommand_ListLimit(t *testing.T) {
	cwd := t.TempDir()
	base := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		sid := writeTestSession(t, cwd, "question")
		setSessionModTime(t, cwd, sid, base.Add(time.Duration(i)*time.Minute))
	}
	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: cwd}, "")
	// Each listing row starts with two spaces followed by the session ID.
	rows := 0
	for _, line := range strings.Split(result.Text, "\n") {
		if strings.HasPrefix(line, "  ") && strings.Contains(line, "msgs") {
			rows++
		}
	}
	if rows != 10 {
		t.Errorf("expected listing capped at 10 sessions, got %d", rows)
	}
}

func TestResumeCommand_EmptySessionListed(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTestSession(t, cwd) // no messages
	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: cwd}, "")
	if !strings.Contains(result.Text, sid) {
		t.Errorf("expected empty session %s in listing, got %q", sid, result.Text)
	}
	if !strings.Contains(result.Text, "(no messages)") {
		t.Errorf("expected empty-session marker, got %q", result.Text)
	}
}

// ---------------------------------------------------------------------------
// /resume <id> — loading history
// ---------------------------------------------------------------------------

func TestResumeCommand_ResumeByID(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTestSession(t, cwd, "hello there", "hi, how can I help?")

	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: cwd}, sid)
	if result.Display != commands.DisplayMessage {
		t.Fatalf("expected DisplayMessage, got %v (text: %q)", result.Display, result.Text)
	}
	if result.NewSessionID != sid {
		t.Errorf("expected NewSessionID %q, got %q", sid, result.NewSessionID)
	}
	msgs, ok := result.NewMessages.([]types.Message)
	if !ok {
		t.Fatalf("expected NewMessages to carry []types.Message, got %T", result.NewMessages)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != types.RoleUser || msgs[1].Role != types.RoleAssistant {
		t.Errorf("unexpected roles: %s, %s", msgs[0].Role, msgs[1].Role)
	}
	if msgs[0].Content[0].Text == nil || *msgs[0].Content[0].Text != "hello there" {
		t.Errorf("unexpected first message content: %+v", msgs[0].Content)
	}
	if !strings.Contains(result.Text, "Resumed session") {
		t.Errorf("expected resume confirmation, got %q", result.Text)
	}
}

func TestResumeCommand_UnknownID(t *testing.T) {
	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: t.TempDir()}, "does-not-exist")
	if result.Display != commands.DisplayError {
		t.Errorf("expected DisplayError, got %v", result.Display)
	}
	if !strings.Contains(result.Text, "not found") {
		t.Errorf("expected not-found message, got %q", result.Text)
	}
	if result.NewMessages != nil {
		t.Error("failed resume must not carry messages")
	}
}

func TestResumeCommand_RejectsPathTraversal(t *testing.T) {
	cmd := resumeCommand(t)
	for _, id := range []string{"../../evil", "a/b", `a\b`, ".."} {
		result := cmd.Execute(commands.CommandContext{WorkingDir: t.TempDir()}, id)
		if result.Display != commands.DisplayError {
			t.Errorf("id %q: expected DisplayError, got %v", id, result.Display)
		}
		if !strings.Contains(result.Text, "invalid session ID") {
			t.Errorf("id %q: expected invalid-ID message, got %q", id, result.Text)
		}
	}
}

func TestResumeCommand_SkipsCorruptEntries(t *testing.T) {
	cwd := t.TempDir()
	sid := writeTestSession(t, cwd, "valid question", "valid answer")

	// Append a corrupt line and a non-message entry directly to the JSONL file.
	path := sessionFilePath(t, cwd, sid)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open session file: %v", err)
	}
	if _, err := f.WriteString("{corrupt json\n" + `{"type":"summary","summary":"irrelevant"}` + "\n"); err != nil {
		t.Fatalf("write corrupt line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close session file: %v", err)
	}

	cmd := resumeCommand(t)
	result := cmd.Execute(commands.CommandContext{WorkingDir: cwd}, sid)
	msgs, ok := result.NewMessages.([]types.Message)
	if !ok {
		t.Fatalf("expected []types.Message, got %T", result.NewMessages)
	}
	if len(msgs) != 2 {
		t.Errorf("expected corrupt/unknown entries skipped, got %d messages", len(msgs))
	}
}
