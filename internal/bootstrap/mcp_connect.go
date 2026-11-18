package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tunsuy/agtkeel/config"
	"github.com/tunsuy/agtkeel/mcp"
	"github.com/tunsuy/agtkeel/tools"
	mcptools "github.com/tunsuy/claude-code-go/internal/tools/mcp"
)

// Startup MCP wiring: connect the pool to every configured server and register
// the discovered per-server tools. Registration is one-shot and pinned for the
// session (provider prefix-cache stability — see mcp.RegisterServerTools).
// Connection failures are non-fatal — they are recorded on the pool's
// connection records (surfaced by `mcp list` and the /mcp command) and joined
// into the returned error for debug logging.

// mcpConnectTimeout bounds a single startup connection attempt.
const mcpConnectTimeout = 30 * time.Second

// connectConfiguredMCPServers connects pool to the configured MCP servers and
// registers each connected server's tools into reg via mcptools.RegisterServerTools.
//
// Server selection mirrors `mcp list` semantics (config.MCPStore.ListMCPServers:
// deduplicated across scopes, rejected and non-listable entries omitted), with
// one addition: project-scoped (.mcp.json) servers are only connected when the
// user has approved them (enableAllProjectMcpServers or enabledMcpjsonServers
// in the merged settings). Pending-approval servers are left untouched.
//
// The returned error joins all per-server connect/list failures; callers treat
// it as non-fatal (startup must not fail because an MCP server is down).
func connectConfiguredMCPServers(
	ctx context.Context,
	reg *tools.Registry,
	pool *mcp.Pool,
	settings *config.LayeredSettings,
	homeDir, workingDir string,
) error {
	store := config.NewMCPStore(homeDir, workingDir)
	var errs []error
	for _, e := range store.ListMCPServers() {
		if _, scope, ok := store.GetMCPServer(e.Name); ok && scope == config.MCPScopeProject &&
			!projectMCPApproved(settings, e.Name) {
			continue
		}
		cfg, err := mcpServerConfigFromEntry(e)
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp: server %q config: %w", e.Name, err))
			continue
		}
		if err := connectWithWatchdog(ctx, pool, e.Name, cfg); err != nil {
			errs = append(errs, fmt.Errorf("mcp: connect %q: %w", e.Name, err))
		}
	}

	if _, err := mcptools.RegisterServerTools(ctx, reg, pool); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// connectWithWatchdog connects one server, bounding the handshake with
// mcpConnectTimeout.
//
// The connect itself runs with a detached background context: the agtkeel
// jsonRPCClient binds its background reader goroutine to the context of the
// first call (the initialize handshake), so a timeout context cancelled after
// Connect would tear the connection down right after startup. The watchdog
// only bounds how long startup waits; a handshake that completes later still
// lands in the pool (its tools are picked up by the next RegisterServerTools
// pass, e.g. after a /mcp reconnect).
func connectWithWatchdog(ctx context.Context, pool *mcp.Pool, name string, cfg mcp.ServerConfig) error {
	done := make(chan error, 1)
	go func() {
		done <- pool.Connect(context.Background(), name, cfg)
	}()
	timer := time.NewTimer(mcpConnectTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("handshake timed out after %s", mcpConnectTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// projectMCPApproved reports whether a project-scoped (.mcp.json) server has
// been approved for use via the merged settings.
func projectMCPApproved(settings *config.LayeredSettings, name string) bool {
	if settings == nil || settings.Merged == nil {
		return false
	}
	if settings.Merged.EnableAllProjectMCP != nil && *settings.Merged.EnableAllProjectMCP {
		return true
	}
	for _, n := range settings.Merged.EnabledMCPServers {
		if n == name {
			return true
		}
	}
	return false
}
