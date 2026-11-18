package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tunsuy/agtkeel/config"
	"github.com/tunsuy/agtkeel/mcp"
	"github.com/tunsuy/agtkeel/tools"
)

// ── Minimal stub MCP server for wiring tests ─────────────────────────────────
//
// Speaks just enough of the protocol (initialize, tools/list, tools/call) over
// the Streamable HTTP transport to prove the startup wiring connects a real
// client and registers the discovered tools. The full-featured stub lives in
// internal/tools/mcp/mcp_test.go.

type wiringStubServer struct {
	url    string
	done   chan struct{} // closed at cleanup to release long-polling GETs
	respCh chan *mcp.JSONRPCMessage
}

func newWiringStubServer(t *testing.T) *wiringStubServer {
	t.Helper()
	s := &wiringStubServer{done: make(chan struct{}), respCh: make(chan *mcp.JSONRPCMessage, 16)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "text/event-stream")
			select {
			case msg := <-s.respCh:
				data, err := json.Marshal(msg)
				if err != nil {
					t.Errorf("wiring stub: marshal: %v", err)
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-s.done:
			case <-r.Context().Done():
			}
			return
		}
		var req mcp.JSONRPCMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Mcp-Session-Id", "wiring-stub")
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo":      map[string]any{"name": "wiring-stub", "version": "0.1.0"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{
				"name":        "echo",
				"description": "echo tool",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			}}}
		case "tools/call":
			result = map[string]any{"content": []map[string]any{{"type": "text", "text": "wired"}}}
		default:
			s.respCh <- &mcp.JSONRPCMessage{JSONRPC: "2.0", ID: req.ID,
				Error: &mcp.JSONRPCError{Code: -32601, Message: "method not found"}}
			w.WriteHeader(http.StatusAccepted)
			return
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Errorf("wiring stub: marshal result: %v", err)
			return
		}
		s.respCh <- &mcp.JSONRPCMessage{JSONRPC: "2.0", ID: req.ID, Result: raw}
		w.WriteHeader(http.StatusAccepted)
	}))
	// Release any in-flight long-poll GET before Close: httptest.Server.Close
	// waits for active requests to complete.
	t.Cleanup(func() {
		close(s.done)
		srv.Close()
	})
	s.url = srv.URL
	return s
}

func addHTTPServerEntry(t *testing.T, store *config.MCPStore, scope, name, url string) {
	t.Helper()
	entry := config.NewOrderedMap()
	entry.Set("type", "http")
	entry.Set("url", url)
	if err := store.AddMCPServer(scope, name, entry); err != nil {
		t.Fatalf("add %s server %q: %v", scope, name, err)
	}
}

// ── RegisterBuiltinTools ─────────────────────────────────────────────────────

func TestRegisterBuiltinTools_NilPool(t *testing.T) {
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg, nil)
	if reg.Len() != 34 {
		t.Fatalf("expected 34 builtin tools, got %d", reg.Len())
	}
	list, ok := reg.Get("ListMcpResources")
	if !ok {
		t.Fatal("ListMcpResources not registered")
	}
	result, err := list.Call(nil, &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content.(string), "no MCP pool") {
		t.Errorf("nil-pool ListMcpResources should report honest error, got: %+v", result)
	}
	read, ok := reg.Get("ReadMcpResource")
	if !ok {
		t.Fatal("ReadMcpResource not registered")
	}
	in, _ := json.Marshal(map[string]string{"server_name": "s", "uri": "file://x"})
	result, err = read.Call(in, &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("nil-pool ReadMcpResource should report honest error")
	}
}

// ── connectConfiguredMCPServers ──────────────────────────────────────────────

func TestConnectConfiguredMCPServers_NoConfig(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg, nil)
	pool := mcp.NewPool()

	err := connectConfiguredMCPServers(context.Background(), reg, pool, nil, home, work)
	if err != nil {
		t.Fatalf("expected no error with no configured servers, got: %v", err)
	}
	if len(pool.GetAll()) != 0 {
		t.Errorf("expected empty pool, got %d connections", len(pool.GetAll()))
	}
	if reg.Len() != 34 {
		t.Errorf("expected only builtin tools, got %d", reg.Len())
	}
}

func TestConnectConfiguredMCPServers_ConnectsAndRegisters(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	stub := newWiringStubServer(t)
	store := config.NewMCPStore(home, work)
	addHTTPServerEntry(t, store, config.MCPScopeUser, "stub", stub.url)

	pool := mcp.NewPool()
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg, pool)

	err := connectConfiguredMCPServers(context.Background(), reg, pool, nil, home, work)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	conns := pool.GetConnected()
	if len(conns) != 1 || conns[0].Name != "stub" {
		t.Fatalf("expected one connected server 'stub', got %+v", conns)
	}
	tool, ok := reg.Get("stub__echo")
	if !ok {
		t.Fatal("expected discovered tool stub__echo to be registered")
	}
	result, err := tool.Call(json.RawMessage(`{}`), &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError || result.Content != "wired" {
		t.Errorf("expected forwarded call result 'wired', got: %+v", result)
	}
	// Resource tools see the live pool too.
	list, _ := reg.Get("ListMcpResources")
	in, _ := json.Marshal(map[string]string{"server_name": "stub"})
	result, err = list.Call(in, &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	// The wiring stub does not implement resources/list; the tool must
	// surface the upstream failure honestly instead of panicking.
	if !result.IsError {
		t.Errorf("expected honest error for unsupported resources/list, got: %+v", result)
	}
}

func TestConnectConfiguredMCPServers_ConnectFailureNonFatal(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	store := config.NewMCPStore(home, work)
	entry := config.NewOrderedMap()
	entry.Set("type", "stdio")
	entry.Set("command", "ccg-nonexistent-mcp-binary-xyz")
	if err := store.AddMCPServer(config.MCPScopeUser, "broken", entry); err != nil {
		t.Fatalf("add server: %v", err)
	}

	pool := mcp.NewPool()
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg, pool)

	err := connectConfiguredMCPServers(context.Background(), reg, pool, nil, home, work)
	if err == nil {
		t.Fatal("expected joined error for failed connection")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the failing server, got: %v", err)
	}
	conns := pool.GetAll()
	if len(conns) != 1 || conns[0].Status != mcp.StatusFailed {
		t.Fatalf("expected failed connection record for /mcp display, got %+v", conns)
	}
	if reg.Len() != 34 {
		t.Errorf("builtin tools must stay registered, got %d", reg.Len())
	}
}

func TestConnectConfiguredMCPServers_ProjectApprovalGate(t *testing.T) {
	home, work := t.TempDir(), t.TempDir()
	stub := newWiringStubServer(t)
	store := config.NewMCPStore(home, work)
	addHTTPServerEntry(t, store, config.MCPScopeProject, "proj", stub.url)

	// Pending approval: the server must NOT be connected.
	pool := mcp.NewPool()
	reg := tools.NewRegistry()
	RegisterBuiltinTools(reg, pool)
	if err := connectConfiguredMCPServers(context.Background(), reg, pool, nil, home, work); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pool.GetAll()) != 0 {
		t.Fatalf("pending project server should be skipped, got %+v", pool.GetAll())
	}

	// Approved via enabledMcpjsonServers: a NEW session (fresh registry —
	// registration is pinned per registry/session) connects the server.
	settings := &config.LayeredSettings{Merged: &config.SettingsJson{
		EnabledMCPServers: []string{"proj"},
	}}
	reg2 := tools.NewRegistry()
	RegisterBuiltinTools(reg2, pool)
	if err := connectConfiguredMCPServers(context.Background(), reg2, pool, settings, home, work); err != nil {
		t.Fatalf("unexpected error with approval: %v", err)
	}
	if len(pool.GetConnected()) != 1 {
		t.Fatalf("approved project server should connect, got %+v", pool.GetAll())
	}
	if _, ok := reg2.Get("proj__echo"); !ok {
		t.Error("expected proj__echo registered after approval")
	}
	// The first session's registry stays pinned (no mid-session tool changes).
	if _, ok := reg.Get("proj__echo"); ok {
		t.Error("first session registry must not gain tools mid-session")
	}
}

func TestProjectMCPApproved(t *testing.T) {
	yes := true
	cases := []struct {
		name     string
		settings *config.LayeredSettings
		server   string
		want     bool
	}{
		{"nil settings", nil, "s", false},
		{"nil merged", &config.LayeredSettings{}, "s", false},
		{"empty", &config.LayeredSettings{Merged: &config.SettingsJson{}}, "s", false},
		{"enabled list", &config.LayeredSettings{Merged: &config.SettingsJson{EnabledMCPServers: []string{"s"}}}, "s", true},
		{"other enabled", &config.LayeredSettings{Merged: &config.SettingsJson{EnabledMCPServers: []string{"other"}}}, "s", false},
		{"enable all", &config.LayeredSettings{Merged: &config.SettingsJson{EnableAllProjectMCP: &yes}}, "s", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectMCPApproved(tc.settings, tc.server); got != tc.want {
				t.Errorf("projectMCPApproved = %v, want %v", got, tc.want)
			}
		})
	}
}
