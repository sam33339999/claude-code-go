package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tunsuy/glaude/internal/mcp"
)

// MCPServerLister is the read-only view of the MCP connection pool required
// by the /mcp command. *mcp.Pool implements it; tests may supply a fake.
//
// Concurrency constraint (CR-BF03-2): GetAll returns a fresh slice whose
// elements are LIVE *mcp.ServerConnection pointers shared with the pool.
// The caller must ensure that no pool mutation (Connect/Reconnect/Disconnect
// or any other writer of connection fields) runs concurrently with reading
// the returned connections. A value-snapshot GetAll is pending on the agtkeel
// side (cross-repo task BF-19); until it lands, every call site must uphold
// this no-concurrent-mutation constraint.
type MCPServerLister interface {
	// GetAll returns all server connections regardless of status. The
	// returned pointers alias the pool's live connections; see the
	// interface-level no-concurrent-mutation constraint.
	GetAll() []*mcp.ServerConnection
}

// Compile-time check: the agtkeel pool satisfies the /mcp dependency.
var _ MCPServerLister = (*mcp.Pool)(nil)

// mcpToolListTimeout bounds each per-server ListTools call so a hung MCP
// server cannot stall the /mcp status report indefinitely.
const mcpToolListTimeout = 2 * time.Second

// FormatMCPStatus renders the MCP server status report shown by /mcp:
// one line per configured server with its name, transport, connection
// status and (for connected servers) the number of tools it exposes.
//
// It may perform network I/O (ListTools on connected servers), so callers
// in the TUI must invoke it from a tea.Cmd, never from Update() directly.
// A nil pool or an empty pool yields a friendly informational message.
//
// Concurrency constraint (CR-BF03-2): it reads live connection fields
// (Status/Error/Client/Config) without synchronization. The caller must
// guarantee that no pool mutation (Connect/Reconnect/Disconnect) runs
// concurrently with this call; see MCPServerLister. Snapshot semantics
// are pending agtkeel BF-19.
func FormatMCPStatus(ctx context.Context, pool MCPServerLister) string {
	if pool == nil {
		return "MCP is not available in this session."
	}
	conns := pool.GetAll()
	if len(conns) == 0 {
		return "No MCP servers configured. Use `glaude mcp add` to add a server."
	}

	// Deterministic order: alphabetical by server name.
	sort.Slice(conns, func(i, j int) bool { return conns[i].Name < conns[j].Name })

	var sb strings.Builder
	sb.WriteString("MCP servers:\n\n")
	for _, live := range conns {
		// Defensive snapshot (CR-BF03-2): copy the connection value once so
		// all fields of one report line are read at a single instant and the
		// line stays internally consistent. This narrows the read window but
		// is NOT synchronization: the no-concurrent-mutation constraint above
		// still applies until agtkeel GetAll returns value snapshots (BF-19).
		c := *live
		sb.WriteString(fmt.Sprintf("  %s (%s): %s", c.Name, mcpTransportLabel(&c), mcpStatusLabel(&c)))
		if c.Status == mcp.StatusConnected {
			sb.WriteString(mcpToolCountSuffix(ctx, &c))
		}
		if c.Status == mcp.StatusFailed && c.Error != "" {
			sb.WriteString(" — " + c.Error)
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// mcpTransportLabel renders the configured transport type of a connection.
func mcpTransportLabel(c *mcp.ServerConnection) string {
	if c.Config.Transport == "" {
		return "unknown"
	}
	return string(c.Config.Transport)
}

// mcpStatusLabel maps a connection status to a user-facing marker.
func mcpStatusLabel(c *mcp.ServerConnection) string {
	switch c.Status {
	case mcp.StatusConnected:
		return "✓ connected"
	case mcp.StatusFailed:
		return "✗ failed"
	case mcp.StatusNeedsAuth:
		return "⚠ needs authentication"
	case mcp.StatusPending:
		return "… pending"
	case mcp.StatusDisabled:
		return "○ disabled"
	default:
		return string(c.Status)
	}
}

// mcpToolCountSuffix queries the server's tool list and renders " — N tools".
// On error (or when no client is attached) it degrades gracefully instead of
// failing the whole report.
func mcpToolCountSuffix(ctx context.Context, c *mcp.ServerConnection) string {
	if c.Client == nil {
		return ""
	}
	toolCtx, cancel := context.WithTimeout(ctx, mcpToolListTimeout)
	defer cancel()
	tools, err := c.Client.ListTools(toolCtx)
	if err != nil {
		return " — tools unavailable (" + err.Error() + ")"
	}
	return fmt.Sprintf(" — %d tools", len(tools))
}
