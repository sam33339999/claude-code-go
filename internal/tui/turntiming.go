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

// clock returns the current time from the injected clock (time.Now by default).
func (m AppModel) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// beginQuery switches the model into the loading state and starts timing a
// new turn. Every query start point goes through here so the timer and the
// spinner can never disagree about when a turn began.
func (m AppModel) beginQuery() AppModel {
	m.isLoading = true
	m.showSpinner = true
	m.spinner = m.spinner.Reset()
	m.turnStart = m.clock()
	m.turnStats = turnStats{}
	return m
}

// noteTurnUsage folds one API response's usage into the running turn.
func (m AppModel) noteTurnUsage(u *TokenUsage) AppModel {
	if u == nil {
		return m
	}
	m.turnStats.apiRequests++
	m.turnStats.inputTokens += u.Input
	m.turnStats.outputTokens += u.Output
	return m
}

// finishTurn ends the timed turn with outcome (done | error | interrupted):
// it appends the display-only duration line (done/interrupted) and returns
// the Cmd that logs the record. Without a running turn it is a no-op, which
// also makes stale end events harmless. The line lives only in m.messages —
// never in engine messages or the transcript — so the prompt is unaffected.
func (m AppModel) finishTurn(outcome string) (AppModel, tea.Cmd) {
	if m.turnStart.IsZero() {
		return m, nil
	}
	end := m.clock()
	dur := end.Sub(m.turnStart)
	rec := turnRecord{
		Start:        m.turnStart,
		End:          end,
		DurationMs:   dur.Milliseconds(),
		Outcome:      outcome,
		SessionID:    m.sessionID,
		Cwd:          m.statusBar.cwd,
		Model:        m.statusBar.model,
		APIRequests:  m.turnStats.apiRequests,
		ToolCalls:    m.turnStats.toolCalls,
		InputTokens:  m.turnStats.inputTokens,
		OutputTokens: m.turnStats.outputTokens,
	}
	switch outcome {
	case "done":
		m.messages = append(m.messages, newSystemMessage("✻ Worked for "+formatDuration(dur)))
	case "interrupted":
		m.messages = append(m.messages, newSystemMessage("✻ Interrupted after "+formatDuration(dur)))
	}
	m.turnStart = time.Time{}
	m.turnStats = turnStats{}
	return m, writeTurnRecordCmd(m.turnLogPath, rec)
}
