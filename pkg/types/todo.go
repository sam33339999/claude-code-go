package types

// TodoItem is a single session TODO entry, maintained by the TodoWrite tool
// and rendered by the TUI todo panel (BF-07).
type TodoItem struct {
	// ID uniquely identifies the item within the list.
	ID string `json:"id"`
	// Content is the task description.
	Content string `json:"content"`
	// Status is one of TodoPending, TodoInProgress, TodoCompleted.
	Status string `json:"status"`
	// Priority is optional; higher number = higher priority.
	Priority int `json:"priority,omitempty"`
}

// Todo status values (oracle TodoWriteTool parity).
const (
	TodoPending    = "pending"
	TodoInProgress = "in_progress"
	TodoCompleted  = "completed"
)
