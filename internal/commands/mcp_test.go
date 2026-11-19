package commands_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/mcp"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakePool is a commands.MCPServerLister returning canned connections.
type fakePool struct {
	conns []*mcp.ServerConnection
}

func (f *fakePool) GetAll() []*mcp.ServerConnection { return f.conns }

// fakeMCPClient implements mcp.MCPClient with a canned ListTools result.
type fakeMCPClient struct {
	tools   []mcp.MCPToolDef
	listErr error
}

func (f *fakeMCPClient) ListTools(_ context.Context) ([]mcp.MCPToolDef, error) {
	return f.tools, f.listErr
}
func (f *fakeMCPClient) CallTool(_ context.Context, _ string, _ map[string]any) (*mcp.MCPToolResult, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeMCPClient) ListResources(_ context.Context) ([]mcp.MCPResource, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeMCPClient) ReadResource(_ context.Context, _ string) (*mcp.MCPResourceContent, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeMCPClient) Ping(_ context.Context) error { return nil }
func (f *fakeMCPClient) Close() error                 { return nil }
func (f *fakeMCPClient) ServerInfo() mcp.ServerInfo   { return mcp.ServerInfo{} }

// ---------------------------------------------------------------------------
// FormatMCPStatus
// ---------------------------------------------------------------------------

func TestFormatMCPStatus_NilPool(t *testing.T) {
	got := commands.FormatMCPStatus(context.Background(), nil)
	if !strings.Contains(got, "not available") {
		t.Errorf("expected friendly nil-pool message, got %q", got)
	}
}

func TestFormatMCPStatus_NoServers(t *testing.T) {
	got := commands.FormatMCPStatus(context.Background(), &fakePool{})
	if !strings.Contains(got, "No MCP servers configured") {
		t.Errorf("expected empty-pool message, got %q", got)
	}
}

func TestFormatMCPStatus_ConnectedWithTools(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{
			Name:   "filesystem",
			Status: mcp.StatusConnected,
			Config: mcp.ServerConfig{Transport: mcp.TransportStdio},
			Client: &fakeMCPClient{tools: []mcp.MCPToolDef{
				{Name: "read_file"}, {Name: "write_file"},
			}},
		},
	}}
	got := commands.FormatMCPStatus(context.Background(), pool)
	for _, want := range []string{"filesystem", "stdio", "connected", "2 tools"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in report, got %q", want, got)
		}
	}
}

func TestFormatMCPStatus_FailedShowsError(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{
			Name:   "broken",
			Status: mcp.StatusFailed,
			Config: mcp.ServerConfig{Transport: mcp.TransportHTTP},
			Error:  "dial tcp: connection refused",
		},
	}}
	got := commands.FormatMCPStatus(context.Background(), pool)
	for _, want := range []string{"broken", "http", "failed", "connection refused"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in report, got %q", want, got)
		}
	}
}

func TestFormatMCPStatus_AllStatusLabels(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{Name: "a-needs-auth", Status: mcp.StatusNeedsAuth},
		{Name: "b-pending", Status: mcp.StatusPending},
		{Name: "c-disabled", Status: mcp.StatusDisabled},
	}}
	got := commands.FormatMCPStatus(context.Background(), pool)
	for _, want := range []string{"needs authentication", "pending", "disabled"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in report, got %q", want, got)
		}
	}
}

func TestFormatMCPStatus_ListToolsErrorDegrades(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{
			Name:   "slow",
			Status: mcp.StatusConnected,
			Config: mcp.ServerConfig{Transport: mcp.TransportSSE},
			Client: &fakeMCPClient{listErr: errors.New("timeout")},
		},
	}}
	got := commands.FormatMCPStatus(context.Background(), pool)
	if !strings.Contains(got, "connected") {
		t.Errorf("server should still show connected, got %q", got)
	}
	if !strings.Contains(got, "tools unavailable") {
		t.Errorf("expected graceful tool-count degradation, got %q", got)
	}
}

func TestFormatMCPStatus_SortedByName(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{Name: "zeta", Status: mcp.StatusDisabled},
		{Name: "alpha", Status: mcp.StatusDisabled},
	}}
	got := commands.FormatMCPStatus(context.Background(), pool)
	if strings.Index(got, "alpha") > strings.Index(got, "zeta") {
		t.Errorf("expected alphabetical order, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// /mcp command wiring
// ---------------------------------------------------------------------------

func TestMCPCommand_RequestsAsyncFetch(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Lookup("mcp")
	if cmd == nil {
		t.Fatal("expected /mcp to be registered")
	}
	result := cmd.Execute(commands.CommandContext{}, "")
	if !result.FetchMCPStatus {
		t.Error("expected /mcp to set FetchMCPStatus = true")
	}
	if result.Display != commands.DisplayNone {
		t.Errorf("expected DisplayNone (async report), got %v", result.Display)
	}
	if strings.Contains(result.Text, "not yet implemented") {
		t.Error("stub text must be gone")
	}
}

// TestFormatMCPStatus_ConnectedNilClient covers mcpToolCountSuffix's
// no-client branch (CR-BF03-6): a connected-status connection without an
// attached client renders the status line with no tool suffix and no panic.
func TestFormatMCPStatus_ConnectedNilClient(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{Name: "ghost", Status: mcp.StatusConnected, Client: nil},
	}}
	out := commands.FormatMCPStatus(context.Background(), pool)
	if !strings.Contains(out, "ghost") || !strings.Contains(out, "✓ connected") {
		t.Errorf("expected connected status line for ghost, got %q", out)
	}
	if strings.Contains(out, "tools") {
		t.Errorf("nil client must not produce a tool suffix, got %q", out)
	}
}

// blockingMCPClient blocks ListTools until the context is done, then returns
// the context error — it drives the per-server timeout/degradation path
// deterministically (CR-BF03-6) without waiting the production 2s.
type blockingMCPClient struct{ fakeMCPClient }

func (b *blockingMCPClient) ListTools(ctx context.Context) ([]mcp.MCPToolDef, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestFormatMCPStatus_ListToolsTimeoutDegrades proves a hung tools/list is
// bounded by the caller context and degrades to an honest "tools unavailable"
// suffix instead of blocking the report or failing it.
func TestFormatMCPStatus_ListToolsTimeoutDegrades(t *testing.T) {
	pool := &fakePool{conns: []*mcp.ServerConnection{
		{Name: "slow", Status: mcp.StatusConnected, Client: &blockingMCPClient{}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already expired: ListTools must return promptly via ctx.Done
	out := commands.FormatMCPStatus(ctx, pool)
	if !strings.Contains(out, "slow") || !strings.Contains(out, "✓ connected") {
		t.Errorf("expected connected status line for slow, got %q", out)
	}
	if !strings.Contains(out, "tools unavailable") {
		t.Errorf("expected honest degradation suffix, got %q", out)
	}
}
