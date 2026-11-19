package tui

import (
	"time"

	"github.com/tunsuy/glaude/internal/memdir"
	"github.com/tunsuy/glaude/pkg/types"
)

// --- User input ---

// InputSubmittedMsg is sent when the user presses Enter.
type InputSubmittedMsg struct{ Text string }

// InputChangedMsg is sent when the input text changes (e.g. Tab completion).
type InputChangedMsg struct{ Text string }

// SlashCommandMsg is sent when the user submits a slash command.
type SlashCommandMsg struct {
	Name string
	Args string
}

// --- LLM streaming events ---

// StreamTokenMsg carries a text delta from the streaming response.
type StreamTokenMsg struct{ Delta string }

// StreamThinkingMsg carries a thinking block delta.
type StreamThinkingMsg struct{ Delta string }

// StreamToolUseStartMsg is emitted when the LLM begins a tool call.
type StreamToolUseStartMsg struct {
	ToolUseID  string
	ToolName   string
	InputDelta string
}

// StreamToolUseInputDeltaMsg is emitted for each incremental JSON fragment of
// a tool call's input while the LLM is still streaming it.
type StreamToolUseInputDeltaMsg struct {
	ToolUseID  string
	ToolName   string
	InputDelta string
}

// StreamToolUseCompleteMsg is emitted when the LLM completes a tool call block.
type StreamToolUseCompleteMsg struct {
	ToolUseID string
	ToolInput string
}

// StreamToolResultMsg is emitted after a tool finishes executing.
type StreamToolResultMsg struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// StreamAssistantTurnMsg is emitted when one assistant turn completes
// (i.e. the LLM finishes one streaming response). In a tool_use scenario
// this does NOT mean the whole query is done — the engine will continue with
// tool execution and a subsequent LLM call. The TUI must keep pulling events.
type StreamAssistantTurnMsg struct {
	FinalMessage *types.Message // the completed assistant message for this turn
}

// StreamUserTurnMsg is emitted when a user turn is added to the conversation,
// typically containing tool_result blocks after tools have executed.
type StreamUserTurnMsg struct {
	FinalMessage *types.Message // the completed user message (tool results)
}

// StreamDoneMsg is emitted when the entire query cycle is finished.
// No more events will arrive on the stream channel after this.
type StreamDoneMsg struct {
	FinalMessage *types.Message // the completed assistant message (legacy; may be nil)
}

// StreamErrorMsg is emitted on an unrecoverable streaming error.
type StreamErrorMsg struct{ Err error }

// --- Permission request ---

// PermissionRequestMsg is sent when a tool call requires user confirmation.
type PermissionRequestMsg struct {
	RequestID   string
	ToolName    string
	ToolUseID   string
	Message     string
	Input       string // JSON
	ProjectPath string // Project path for "always allow" option
	// FromCodeMode marks requests from inside a CodeMode execution (CM-03):
	// the dialog offers CodeMode-scoped persistent approval instead of the
	// project-wide option.
	FromCodeMode bool
	// RespFn is called by the TUI once the user makes a decision.
	// allow is the approve/deny choice; alwaysAllow is true only when the user
	// picked the "yes, and don't ask again" option (allow must be true too).
	// Using a function avoids channel-send blocking issues.
	RespFn func(allow, alwaysAllow bool)
}

// --- System events ---

// TermResizedMsg wraps tea.WindowSizeMsg.
type TermResizedMsg struct{ Width, Height int }

// TickMsg is the periodic tick for spinner animation / task eviction.
type TickMsg struct{ Time time.Time }

// CompactDoneMsg is sent when /compact finishes.
type CompactDoneMsg struct{ Summary string }

// DreamDoneMsg is sent when /dream finishes.
type DreamDoneMsg struct {
	Err string
}

// MCPStatusMsg carries the rendered /mcp server status report, produced
// asynchronously by fetchMCPStatusCmd.
type MCPStatusMsg struct{ Text string }

// TranscriptErrMsg reports a session-persistence failure (BF-18). The TUI
// surfaces it as a system warning; the conversation itself is unaffected.
type TranscriptErrMsg struct{ Err error }

// QuestionRequestMsg is sent when the AskUserQuestion tool needs user input
// (BF-08). RespFn delivers the answer back to the waiting tool through the
// HIL response channel (same callback pattern as PermissionRequestMsg).
type QuestionRequestMsg struct {
	RequestID string
	Question  string
	Options   []string
	RespFn    func(answer string, dismissed bool)
}

// AgentStatusMsg is pushed from Agent-Core to update a sub-agent's status.
type AgentStatusMsg struct {
	TaskID      string
	Status      AgentStatus
	Description string // human-readable task description
	Result      string // final output on completion/failure (empty while running)
	Summary     string // human-readable status summary
}

// AgentProgressMsg is pushed from a running sub-agent to report real-time
// progress updates (streaming text, tool calls, etc.) to the coordinator panel.
type AgentProgressMsg struct {
	TaskID   string
	Activity string // short label, e.g. "Streaming", "Running Bash", "Reading file"
	Detail   string // one-line detail, e.g. tool input summary or partial text
}

// CommandResultMsg carries the result of a slash command execution.
type CommandResultMsg struct {
	Text    string
	IsError bool
}

// MemdirLoadedMsg is sent when the initial GLAUDE.md files have been loaded.
type MemdirLoadedMsg struct {
	Paths       []string
	ScopedFiles []memdir.DiscoveredFile // Scoped discovery results (new multi-layer)
}

// SystemTextMsg surfaces informational text from the engine (e.g. max turns).
type SystemTextMsg struct{ Text string }

// --- Mid-session message queue events ---

// queueChangedMsg is sent when the MessageQueue has new commands.
// The Update loop checks this to trigger between-turn drain when idle.
type queueChangedMsg struct{}

// queuedSlashMsg is sent when a queued slash command is ready for dispatch.
type queuedSlashMsg struct{ Text string }
