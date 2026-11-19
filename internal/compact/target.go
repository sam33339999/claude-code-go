package compact

import (
	"sync"

	"github.com/tunsuy/glaude/internal/api"
)

// Target describes the model a query actually runs on. Thresholds and the
// summarisation request are derived from it, so a sub-agent bound to a
// tier compacts against its own window and summarises on its own client.
type Target struct {
	// Client receives the summarisation request.
	Client api.Client
	// Model is the query's model (also used for the summary).
	Model string
	// ContextWindow is the configured window; 0 resolves via the built-in table.
	ContextWindow int
	// MaxOutputTokens is the query's max_tokens; 0 means unknown.
	MaxOutputTokens int
}

// Window returns the effective context window for t.
func (t Target) Window() int {
	return ResolveContextWindow(t.Model, t.ContextWindow)
}

// Tracker holds one agent's auto-compact state (circuit breaker included).
// The main loop keeps one for the whole session; every sub-agent owns its
// own, so a failing sub-agent cannot disable the main loop's compaction.
// Safe for concurrent use.
type Tracker struct {
	mu    sync.Mutex
	state AutoCompactTrackingState
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{}
}

// Snapshot returns a copy of the current state.
func (t *Tracker) Snapshot() AutoCompactTrackingState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

func (t *Tracker) breakerOpen() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state.ConsecutiveFailures >= MaxConsecutiveFailures
}

func (t *Tracker) recordFailure() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.ConsecutiveFailures++
}

func (t *Tracker) recordSuccess() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.ConsecutiveFailures = 0
	t.state.Compacted = true
	t.state.TurnCounter = 0
}
