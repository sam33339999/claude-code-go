package compact

import (
	"os"
	"strconv"
	"strings"
)

// ResolveContextWindow returns the context window (in tokens) for model.
// Precedence: CLAUDE_CODE_AUTO_COMPACT_WINDOW (global override) > configured
// (> 0; from a tier binding, provider model table or [main]) > built-in
// table > defaultContextWindow.
func ResolveContextWindow(model string, configured int) int {
	if v, err := strconv.Atoi(os.Getenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW")); err == nil && v > 0 {
		return v
	}
	if configured > 0 {
		return configured
	}
	return builtinContextWindow(model)
}

// ContextWindow returns the context window used for model with no configured
// value — the same value auto-compaction budgets against. Exposed for display
// (the TUI status bar's context-usage meter).
func ContextWindow(model string) int {
	return ResolveContextWindow(model, 0)
}

// builtinContextWindow is the table of well-known model windows. Models the
// table does not know (most non-Claude models) get defaultContextWindow;
// users set the real value in ~/.glaude/config.toml.
func builtinContextWindow(model string) int {
	m := strings.ToLower(model)
	switch {
	case strings.HasPrefix(m, "claude-2"):
		return 100_000
	case strings.HasPrefix(m, "claude-"):
		return 200_000
	default:
		return defaultContextWindow
	}
}
