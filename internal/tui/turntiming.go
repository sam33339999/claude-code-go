package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

// turnStats accumulates per-turn counters between query start and finish.
type turnStats struct {
	apiRequests  int
	toolCalls    int
	inputTokens  int
	outputTokens int
}

// turnRecord is one JSONL line in the turn-timing log: the wall-clock span
// of a turn (prompt submitted → turn fully finished) plus what it did.
// It never carries prompt text.
type turnRecord struct {
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	DurationMs   int64     `json:"duration_ms"`
	Outcome      string    `json:"outcome"` // done | error | interrupted
	SessionID    string    `json:"session_id"`
	Cwd          string    `json:"cwd"`
	Model        string    `json:"model"`
	APIRequests  int       `json:"api_requests"`
	ToolCalls    int       `json:"tool_calls"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
}

// appendTurnRecord appends rec as one JSON line to path, creating the
// parent directory and file when missing.
func appendTurnRecord(path string, rec turnRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("turn log: mkdir: %w", err)
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("turn log: marshal: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("turn log: open: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("turn log: write: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("turn log: close: %w", err)
	}
	return nil
}

// writeTurnRecordCmd appends rec off the Update loop. An empty path disables
// logging (nil Cmd); write failures are dropped so timing never disturbs
// the conversation.
func writeTurnRecordCmd(path string, rec turnRecord) tea.Cmd {
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		_ = appendTurnRecord(path, rec)
		return nil
	}
}

// defaultTurnLogPath returns ~/.glaude/turn-timings.jsonl, or "" when the
// home directory cannot be resolved (logging is then disabled).
func defaultTurnLogPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".glaude", "turn-timings.jsonl")
}
