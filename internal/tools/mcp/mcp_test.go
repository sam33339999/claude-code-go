package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	agtmcp "github.com/tunsuy/agtkeel/mcp"
	"github.com/tunsuy/agtkeel/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/mcp"
)

// ── Stub MCP server (Streamable HTTP transport) ──────────────────────────────
//
// A lightweight in-process MCP server speaking JSON-RPC 2.0 over the agtkeel
// HTTP transport (POST requests, GET SSE responses). Tests connect a real
// agtmcp.Pool to it, so the whole client stack (handshake, tools/list,
// tools/call, resources/*) is exercised end to end.

type stubMCPServer struct {
	t    *testing.T
	srv  *httptest.Server
	done chan struct{} // closed at cleanup to release long-polling GETs

	mu           sync.Mutex
	sessions     map[string]chan *agtmcp.JSONRPCMessage // Mcp-Session-Id → response queue
	nextSession  int
	toolDefs     []map[string]any
	resources    []map[string]any
	readContent  map[string]any
	callContent  []map[string]any
	callIsError  bool
	failMethods  map[string]string // method → RPC error message
	hangMethods  map[string]bool   // method → acknowledge (202) but never respond
	lastCallName string
	lastCallArgs map[string]any
}

func newStubMCPServer(t *testing.T) *stubMCPServer {
	t.Helper()
	s := &stubMCPServer{
		t:           t,
		done:        make(chan struct{}),
		sessions:    map[string]chan *agtmcp.JSONRPCMessage{},
		failMethods: map[string]string{},
		hangMethods: map[string]bool{},
		callContent: []map[string]any{{"type": "text", "text": "ok"}},
		readContent: map[string]any{"uri": "stub://default", "mimeType": "text/plain", "text": "default content"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			s.handlePOST(w, r)
		case http.MethodGet:
			s.handleGET(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	s.srv = httptest.NewServer(mux)
	// Release any in-flight long-poll GET before Close: httptest.Server.Close
	// waits for active requests to complete.
	t.Cleanup(func() {
		close(s.done)
		s.srv.Close()
	})
	return s
}

func (s *stubMCPServer) url() string { return s.srv.URL }

func (s *stubMCPServer) handlePOST(w http.ResponseWriter, r *http.Request) {
	var req agtmcp.JSONRPCMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Session routing: the first POST (initialize) carries no session header;
	// mint one so reconnects get isolated response queues.
	sid := r.Header.Get("Mcp-Session-Id")
	s.mu.Lock()
	if sid == "" {
		s.nextSession++
		sid = fmt.Sprintf("session-%d", s.nextSession)
		s.sessions[sid] = make(chan *agtmcp.JSONRPCMessage, 64)
	}
	ch := s.sessions[sid]
	s.mu.Unlock()
	w.Header().Set("Mcp-Session-Id", sid)
	if req.ID == nil { // notification (e.g. notifications/initialized)
		w.WriteHeader(http.StatusAccepted)
		return
	}
	s.mu.Lock()
	hang := s.hangMethods[req.Method]
	s.mu.Unlock()
	if hang {
		// Acknowledge but never deliver a response: the caller blocks on the
		// GET stream until its context deadline (slow-tools/list stub).
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp := &agtmcp.JSONRPCMessage{JSONRPC: "2.0", ID: req.ID}
	result, rpcErr := s.dispatch(req.Method, req.Params)
	if rpcErr != nil {
		resp.Error = rpcErr
	} else {
		resp.Result = result
	}
	ch <- resp
	w.WriteHeader(http.StatusAccepted)
}

func (s *stubMCPServer) handleGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	ch := s.sessions[r.Header.Get("Mcp-Session-Id")]
	s.mu.Unlock()
	if ch == nil {
		// Unknown session: hold until cleanup instead of spinning.
		select {
		case <-s.done:
		case <-r.Context().Done():
		}
		return
	}
	select {
	case msg := <-ch:
		data, err := json.Marshal(msg)
		if err != nil {
			s.t.Errorf("stub: marshal response: %v", err)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	case <-s.done:
	case <-r.Context().Done():
	}
}

func (s *stubMCPServer) dispatch(method string, params json.RawMessage) (json.RawMessage, *agtmcp.JSONRPCError) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if msg, fail := s.failMethods[method]; fail {
		return nil, &agtmcp.JSONRPCError{Code: -32603, Message: msg}
	}
	var result any
	switch method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "stub", "version": "0.1.0"},
			"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
		}
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": s.toolDefs}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &agtmcp.JSONRPCError{Code: -32602, Message: err.Error()}
		}
		s.lastCallName = p.Name
		s.lastCallArgs = p.Arguments
		result = map[string]any{"content": s.callContent, "isError": s.callIsError}
	case "resources/list":
		result = map[string]any{"resources": s.resources}
	case "resources/read":
		result = map[string]any{"contents": []map[string]any{s.readContent}}
	default:
		return nil, &agtmcp.JSONRPCError{Code: -32601, Message: "method not found: " + method}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, &agtmcp.JSONRPCError{Code: -32603, Message: err.Error()}
	}
	return raw, nil
}

func (s *stubMCPServer) lastCall() (string, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCallName, s.lastCallArgs
}

// connectStubPool connects a fresh pool to the stub server and registers
// cleanup. The connection uses the real agtkeel client stack.
func connectStubPool(t *testing.T, s *stubMCPServer, name string) *agtmcp.Pool {
	t.Helper()
	pool := agtmcp.NewPool()
	// Background context: the agtkeel client binds its reader goroutine to
	// the handshake context, so it must outlive the connect call.
	cfg := agtmcp.ServerConfig{
		Transport: agtmcp.TransportHTTP,
		HTTP:      &agtmcp.HTTPTransportConfig{URL: s.url()},
	}
	if err := pool.Connect(context.Background(), name, cfg); err != nil {
		t.Fatalf("connect stub server %q: %v", name, err)
	}
	t.Cleanup(func() {
		if err := pool.Disconnect(name); err != nil {
			t.Errorf("disconnect %q: %v", name, err)
		}
	})
	return pool
}

func echoToolDef() map[string]any {
	return map[string]any{
		"name":        "echo",
		"description": "Echoes the input back",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"x": map[string]any{"type": "string"}},
			"required":   []string{"x"},
		},
	}
}

func useCtx() *tools.UseContext {
	return &tools.UseContext{Ctx: context.Background()}
}

// ── RegisterServerTools ──────────────────────────────────────────────────────

func TestRegisterServerTools_EmptyPool(t *testing.T) {
	reg := tools.NewRegistry()
	n, err := mcp.RegisterServerTools(context.Background(), reg, agtmcp.NewPool())
	if err != nil || n != 0 {
		t.Fatalf("expected (0, nil), got (%d, %v)", n, err)
	}
	if reg.Len() != 0 {
		t.Errorf("expected empty registry, got %d tools", reg.Len())
	}
}

func TestRegisterServerTools_NilPoolAndRegistry(t *testing.T) {
	if n, err := mcp.RegisterServerTools(context.Background(), tools.NewRegistry(), nil); n != 0 || err != nil {
		t.Errorf("nil pool: expected (0, nil), got (%d, %v)", n, err)
	}
	if n, err := mcp.RegisterServerTools(context.Background(), nil, agtmcp.NewPool()); n != 0 || err != nil {
		t.Errorf("nil registry: expected (0, nil), got (%d, %v)", n, err)
	}
}

func TestRegisterServerTools_DiscoveredToolsRegistered(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{
		echoToolDef(),
		{
			"name":        "do/thing",
			"description": "Does a thing",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
	pool := connectStubPool(t, stub, "myserver")
	reg := tools.NewRegistry()

	n, err := mcp.RegisterServerTools(context.Background(), reg, pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 registered tools, got %d", n)
	}

	echo, ok := reg.Get("myserver__echo")
	if !ok {
		t.Fatal("expected tool myserver__echo to be registered")
	}
	if echo.Description(nil, nil) != "Echoes the input back" {
		t.Errorf("unexpected description: %q", echo.Description(nil, nil))
	}
	schema := echo.InputSchema()
	if _, ok := schema.Properties["x"]; !ok {
		t.Error("schema missing upstream property 'x'")
	}
	// Special characters in upstream names are normalized for the API.
	if _, ok := reg.Get("myserver__do_thing"); !ok {
		t.Error("expected normalized tool name myserver__do_thing")
	}
	// MCP metadata sub-interface is preserved by the kernel adapter.
	info, ok := echo.(tools.MCPToolInfo)
	if !ok {
		t.Fatal("adapted tool should implement tools.MCPToolInfo")
	}
	if info.MCPInfo().ServerName != "myserver" || info.MCPInfo().ToolName != "echo" {
		t.Errorf("unexpected MCPInfo: %+v", info.MCPInfo())
	}
}

func TestRegisterServerTools_ListToolsError(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.failMethods["tools/list"] = "boom"
	pool := connectStubPool(t, stub, "broken")
	reg := tools.NewRegistry()

	n, err := mcp.RegisterServerTools(context.Background(), reg, pool)
	if err == nil {
		t.Fatal("expected error when tools/list fails")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("error should name the failing server, got: %v", err)
	}
	if n != 0 || reg.Len() != 0 {
		t.Errorf("expected no tools registered, got n=%d len=%d", n, reg.Len())
	}
}

// TestRegisterServerTools_PinnedAcrossReconnect is the cache-stability guard
// required by the PM followup (docs/PROMPT_CACHE_AND_COST.md, "Pinned MCP
// placeholders"): once registered at boot, the session's tool name set and
// schema bytes must not change — even when a reconnect brings a different
// live tool list. Differences take effect at the next session startup.
func TestRegisterServerTools_PinnedAcrossReconnect(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err != nil {
		t.Fatalf("boot registration: %v", err)
	}

	// Snapshot the boot-time tool surface: names, schema bytes, descriptions.
	names := reg.Names()
	schemas := map[string]string{}
	descs := map[string]string{}
	for _, name := range names {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("tool %q vanished from registry", name)
		}
		raw, err := json.Marshal(tool.InputSchema())
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", name, err)
		}
		schemas[name] = string(raw)
		descs[name] = tool.Description(nil, nil)
	}

	// The server changes its tool list (renamed schema, new tool), and the
	// pool reconnects.
	stub.mu.Lock()
	stub.toolDefs = []map[string]any{
		{
			"name":        "echo",
			"description": "CHANGED DESCRIPTION",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"y": map[string]any{"type": "integer"}},
			},
		},
		{
			"name":        "brand-new",
			"description": "appeared after reconnect",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
	stub.mu.Unlock()
	if err := pool.Reconnect(context.Background(), "srv"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	// A second registration pass in the same session must be a no-op.
	n, err := mcp.RegisterServerTools(context.Background(), reg, pool)
	if err != nil || n != 0 {
		t.Fatalf("expected pinned no-op (0, nil), got (%d, %v)", n, err)
	}

	gotNames := reg.Names()
	if len(gotNames) != len(names) {
		t.Fatalf("tool name set changed: boot=%v now=%v", names, gotNames)
	}
	for i := range names {
		if gotNames[i] != names[i] {
			t.Fatalf("tool name set changed: boot=%v now=%v", names, gotNames)
		}
	}
	for name, wantSchema := range schemas {
		tool, _ := reg.Get(name)
		raw, err := json.Marshal(tool.InputSchema())
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", name, err)
		}
		if string(raw) != wantSchema {
			t.Errorf("schema bytes changed for %q:\nboot: %s\nnow:  %s", name, wantSchema, raw)
		}
		if got := tool.Description(nil, nil); got != descs[name] {
			t.Errorf("description changed for %q: boot=%q now=%q", name, descs[name], got)
		}
	}
}

func TestRegisterServerTools_MultipleServers(t *testing.T) {
	stubA := newStubMCPServer(t)
	stubA.toolDefs = []map[string]any{echoToolDef()}
	stubB := newStubMCPServer(t)
	stubB.toolDefs = []map[string]any{echoToolDef()}
	pool := connectStubPool(t, stubA, "alpha")
	if err := pool.Connect(context.Background(), "beta", agtmcp.ServerConfig{
		Transport: agtmcp.TransportHTTP,
		HTTP:      &agtmcp.HTTPTransportConfig{URL: stubB.url()},
	}); err != nil {
		t.Fatalf("connect beta: %v", err)
	}
	t.Cleanup(func() {
		if err := pool.Disconnect("beta"); err != nil {
			t.Errorf("disconnect beta: %v", err)
		}
	})

	reg := tools.NewRegistry()
	n, err := mcp.RegisterServerTools(context.Background(), reg, pool)
	if err != nil || n != 2 {
		t.Fatalf("expected (2, nil), got (%d, %v)", n, err)
	}
	if _, ok := reg.Get("alpha__echo"); !ok {
		t.Error("missing alpha__echo")
	}
	if _, ok := reg.Get("beta__echo"); !ok {
		t.Error("missing beta__echo")
	}
}

// ── Proxy call forwarding (adapted per-server tools) ─────────────────────────

func TestProxyTool_CallForwardsToServer(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	stub.callContent = []map[string]any{{"type": "text", "text": "echo: hello"}}
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err != nil {
		t.Fatalf("register: %v", err)
	}
	tool, _ := reg.Get("srv__echo")

	result, err := tool.Call(json.RawMessage(`{"x":"hello"}`), useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %v", result.Content)
	}
	if result.Content != "echo: hello" {
		t.Errorf("unexpected content: %v", result.Content)
	}
	name, args := stub.lastCall()
	if name != "echo" {
		t.Errorf("expected raw tool name 'echo' forwarded, got %q", name)
	}
	if args["x"] != "hello" {
		t.Errorf("expected args forwarded verbatim, got %v", args)
	}
}

func TestProxyTool_CallServerSideError(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	stub.callIsError = true
	stub.callContent = []map[string]any{{"type": "text", "text": "upstream failure"}}
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err != nil {
		t.Fatalf("register: %v", err)
	}
	tool, _ := reg.Get("srv__echo")

	result, err := tool.Call(json.RawMessage(`{"x":"hi"}`), useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for upstream isError result")
	}
	if result.Content != "upstream failure" {
		t.Errorf("unexpected content: %v", result.Content)
	}
}

func TestProxyTool_CallRPCError(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	stub.failMethods["tools/call"] = "call exploded"
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err != nil {
		t.Fatalf("register: %v", err)
	}
	tool, _ := reg.Get("srv__echo")

	result, err := tool.Call(json.RawMessage(`{"x":"hi"}`), useCtx(), nil)
	if err != nil {
		t.Fatalf("client errors must surface as IsError results, got hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for RPC error")
	}
	if !strings.Contains(fmt.Sprint(result.Content), "call exploded") {
		t.Errorf("expected upstream message in content, got: %v", result.Content)
	}
}

func TestProxyTool_CallInvalidInput(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err != nil {
		t.Fatalf("register: %v", err)
	}
	tool, _ := reg.Get("srv__echo")

	result, err := tool.Call(json.RawMessage(`not json`), useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON input")
	}
}

func TestProxyTool_CallCancelledContext(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err != nil {
		t.Fatalf("register: %v", err)
	}
	tool, _ := reg.Get("srv__echo")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := tool.Call(json.RawMessage(`{"x":"hi"}`), &tools.UseContext{Ctx: ctx}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when the call context is cancelled")
	}
}

// ── ListMcpResources ─────────────────────────────────────────────────────────

func TestListMcpResources_Metadata(t *testing.T) {
	tool := mcp.NewListMcpResourcesTool(nil)
	if tool.Name() != "ListMcpResources" {
		t.Errorf("expected ListMcpResources, got %q", tool.Name())
	}
	if !tool.IsConcurrencySafe(nil) || !tool.IsReadOnly(nil) {
		t.Error("ListMcpResources must be concurrency-safe and read-only")
	}
	schema := tool.InputSchema()
	if _, ok := schema.Properties["server_name"]; !ok {
		t.Error("schema missing 'server_name'")
	}
	if len(schema.Required) != 0 {
		t.Errorf("expected no required fields, got %v", schema.Required)
	}
}

func TestListMcpResources_EmptyPool(t *testing.T) {
	tool := mcp.NewListMcpResourcesTool(agtmcp.NewPool())
	result, err := tool.Call(nil, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Errorf("empty pool should be a graceful empty result, got: %v", result.Content)
	}
	if !strings.Contains(fmt.Sprint(result.Content), "no MCP servers") {
		t.Errorf("expected honest empty-pool message, got: %v", result.Content)
	}
}

func TestListMcpResources_NilPool(t *testing.T) {
	tool := mcp.NewListMcpResourcesTool(nil)
	result, err := tool.Call(nil, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when no pool is configured")
	}
}

func TestListMcpResources_ListsAllConnectedServers(t *testing.T) {
	stubA := newStubMCPServer(t)
	stubA.resources = []map[string]any{
		{"uri": "file://a.txt", "name": "a", "description": "resource a", "mimeType": "text/plain"},
	}
	stubB := newStubMCPServer(t)
	stubB.resources = []map[string]any{
		{"uri": "file://b.txt", "name": "b", "mimeType": "text/plain"},
	}
	pool := connectStubPool(t, stubA, "alpha")
	if err := pool.Connect(context.Background(), "beta", agtmcp.ServerConfig{
		Transport: agtmcp.TransportHTTP,
		HTTP:      &agtmcp.HTTPTransportConfig{URL: stubB.url()},
	}); err != nil {
		t.Fatalf("connect beta: %v", err)
	}
	t.Cleanup(func() {
		if err := pool.Disconnect("beta"); err != nil {
			t.Errorf("disconnect beta: %v", err)
		}
	})

	tool := mcp.NewListMcpResourcesTool(pool)
	result, err := tool.Call(nil, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %v", result.Content)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(result.Content.(string)), &entries); err != nil {
		t.Fatalf("output should be a JSON array: %v (content=%v)", err, result.Content)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(entries))
	}
	// Servers are walked in name order: alpha first.
	if entries[0]["server"] != "alpha" || entries[0]["uri"] != "file://a.txt" {
		t.Errorf("unexpected first entry: %v", entries[0])
	}
	if entries[1]["server"] != "beta" || entries[1]["uri"] != "file://b.txt" {
		t.Errorf("unexpected second entry: %v", entries[1])
	}
}

func TestListMcpResources_FilterByServer(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.resources = []map[string]any{{"uri": "file://only.txt", "name": "only"}}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewListMcpResourcesTool(pool)

	in, _ := json.Marshal(mcp.ListMcpResourcesInput{ServerName: "srv"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %v", result.Content)
	}
	if !strings.Contains(result.Content.(string), "file://only.txt") {
		t.Errorf("expected filtered resource in output, got: %v", result.Content)
	}
}

func TestListMcpResources_UnknownServer(t *testing.T) {
	pool := agtmcp.NewPool()
	tool := mcp.NewListMcpResourcesTool(pool)
	in, _ := json.Marshal(mcp.ListMcpResourcesInput{ServerName: "ghost"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content.(string), "unknown MCP server") {
		t.Errorf("expected unknown-server error, got: %+v", result)
	}
}

func TestListMcpResources_DisconnectedServer(t *testing.T) {
	stub := newStubMCPServer(t)
	pool := connectStubPool(t, stub, "srv")
	if err := pool.Disconnect("srv"); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	tool := mcp.NewListMcpResourcesTool(pool)
	in, _ := json.Marshal(mcp.ListMcpResourcesInput{ServerName: "srv"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content.(string), "not connected") {
		t.Errorf("expected not-connected error, got: %+v", result)
	}
}

func TestListMcpResources_ServerError(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.failMethods["resources/list"] = "list failed"
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewListMcpResourcesTool(pool)

	result, err := tool.Call(nil, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when the only server fails")
	}
	if !strings.Contains(result.Content.(string), "list failed") {
		t.Errorf("expected upstream error message, got: %v", result.Content)
	}
}

func TestListMcpResources_InvalidInput(t *testing.T) {
	tool := mcp.NewListMcpResourcesTool(agtmcp.NewPool())
	result, err := tool.Call(json.RawMessage(`{bad`), useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON input")
	}
}

// ── ReadMcpResource ──────────────────────────────────────────────────────────

func TestReadMcpResource_Metadata(t *testing.T) {
	tool := mcp.NewReadMcpResourceTool(nil)
	if tool.Name() != "ReadMcpResource" {
		t.Errorf("expected ReadMcpResource, got %q", tool.Name())
	}
	if !tool.IsConcurrencySafe(nil) || !tool.IsReadOnly(nil) {
		t.Error("ReadMcpResource must be concurrency-safe and read-only")
	}
	schema := tool.InputSchema()
	reqMap := map[string]bool{}
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	if !reqMap["server_name"] || !reqMap["uri"] {
		t.Errorf("expected server_name and uri required, got %v", schema.Required)
	}
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "s1", URI: "file://test"})
	if name := tool.UserFacingName(in); name != "ReadMcpResource(file://test)" {
		t.Errorf("unexpected UserFacingName: %q", name)
	}
}

func TestReadMcpResource_ReadsTextContent(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.readContent = map[string]any{"uri": "file://doc.txt", "mimeType": "text/plain", "text": "resource body"}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewReadMcpResourceTool(pool)

	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "file://doc.txt"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %v", result.Content)
	}
	if result.Content != "resource body" {
		t.Errorf("expected resource text verbatim, got: %v", result.Content)
	}
}

func TestReadMcpResource_BinaryContent(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.readContent = map[string]any{"uri": "img://x", "mimeType": "image/png", "blob": "aGVsbG8="}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewReadMcpResourceTool(pool)

	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "img://x"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %v", result.Content)
	}
	content := fmt.Sprint(result.Content)
	if !strings.Contains(content, "image/png") || !strings.Contains(content, "base64") {
		t.Errorf("expected binary placeholder describing mime type and encoding, got: %s", content)
	}
}

func TestReadMcpResource_MissingFields(t *testing.T) {
	pool := agtmcp.NewPool()
	tool := mcp.NewReadMcpResourceTool(pool)
	cases := []struct {
		name string
		in   mcp.ReadMcpResourceInput
		want string
	}{
		{"missing server", mcp.ReadMcpResourceInput{URI: "file://x"}, "server_name"},
		{"missing uri", mcp.ReadMcpResourceInput{ServerName: "srv"}, "uri"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, _ := json.Marshal(tc.in)
			result, err := tool.Call(in, useCtx(), nil)
			if err != nil {
				t.Fatalf("unexpected hard error: %v", err)
			}
			if !result.IsError || !strings.Contains(result.Content.(string), tc.want) {
				t.Errorf("expected missing-field error naming %q, got: %+v", tc.want, result)
			}
		})
	}
}

func TestReadMcpResource_UnknownServer(t *testing.T) {
	tool := mcp.NewReadMcpResourceTool(agtmcp.NewPool())
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "ghost", URI: "file://x"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Content.(string), "unknown MCP server") {
		t.Errorf("expected unknown-server error, got: %+v", result)
	}
}

func TestReadMcpResource_NilPool(t *testing.T) {
	tool := mcp.NewReadMcpResourceTool(nil)
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "file://x"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when no pool is configured")
	}
}

func TestReadMcpResource_ServerError(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.failMethods["resources/read"] = "read failed"
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewReadMcpResourceTool(pool)

	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "file://x"})
	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for upstream read failure")
	}
	content := result.Content.(string)
	if !strings.Contains(content, "read failed") || !strings.Contains(content, "file://x") {
		t.Errorf("expected error to carry uri and upstream message, got: %s", content)
	}
}

func TestReadMcpResource_InvalidInput(t *testing.T) {
	tool := mcp.NewReadMcpResourceTool(agtmcp.NewPool())
	result, err := tool.Call(json.RawMessage(`not json`), useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON input")
	}
}

func TestReadMcpResource_CancelledContext(t *testing.T) {
	stub := newStubMCPServer(t)
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewReadMcpResourceTool(pool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "file://x"})
	result, err := tool.Call(in, &tools.UseContext{Ctx: ctx}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when the call context is cancelled")
	}
}

// ── Interface compliance ─────────────────────────────────────────────────────

func TestAllMCPTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = mcp.NewListMcpResourcesTool(nil)
	var _ tools.Tool = mcp.NewReadMcpResourceTool(nil)
}

// ── Boot ListTools deadline (CR-BF01-2) ──────────────────────────────────────

// TestRegisterServerTools_SlowListTimesOutNonFatal: a server that handshakes
// but hangs on tools/list must not block registration forever; its ListTools
// times out (per-server deadline) non-fatally while other servers register.
func TestRegisterServerTools_SlowListTimesOutNonFatal(t *testing.T) {
	restore := mcp.SetListToolsTimeoutForTest(100 * time.Millisecond)
	t.Cleanup(restore)

	hang := newStubMCPServer(t)
	hang.toolDefs = []map[string]any{echoToolDef()}
	hang.hangMethods["tools/list"] = true
	okStub := newStubMCPServer(t)
	okStub.toolDefs = []map[string]any{echoToolDef()}

	pool := connectStubPool(t, hang, "a-hang")
	if err := pool.Connect(context.Background(), "b-ok", agtmcp.ServerConfig{
		Transport: agtmcp.TransportHTTP,
		HTTP:      &agtmcp.HTTPTransportConfig{URL: okStub.url()},
	}); err != nil {
		t.Fatalf("connect b-ok: %v", err)
	}
	t.Cleanup(func() {
		if err := pool.Disconnect("b-ok"); err != nil {
			t.Errorf("disconnect b-ok: %v", err)
		}
	})

	reg := tools.NewRegistry()
	start := time.Now()
	n, err := mcp.RegisterServerTools(context.Background(), reg, pool)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "a-hang") {
		t.Fatalf("expected non-fatal error naming a-hang, got: %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("registration not bounded by ListTools deadline: took %v", elapsed)
	}
	if n != 1 {
		t.Errorf("expected 1 tool from b-ok, got %d", n)
	}
	if _, ok := reg.Get("b-ok__echo"); !ok {
		t.Error("b-ok tools should still register despite a-hang hang")
	}
	if _, ok := reg.Get("a-hang__echo"); ok {
		t.Error("a-hang tools must not register when tools/list times out")
	}
}

// ── Pinning guard strengthening (CR-BF01-3) ──────────────────────────────────

// TestRegisterServerTools_FreshRegistrySeesReconnectChanges is the contrast the
// original guard lacked: it proves the reconnect genuinely changed the live
// tool list (a FRESH registry sees the new surface) while the boot registry
// stays pinned — i.e. pinning is per-registry, not a silently broken stub.
func TestRegisterServerTools_FreshRegistrySeesReconnectChanges(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.toolDefs = []map[string]any{echoToolDef()}
	pool := connectStubPool(t, stub, "srv")
	reg1 := tools.NewRegistry()
	if _, err := mcp.RegisterServerTools(context.Background(), reg1, pool); err != nil {
		t.Fatalf("boot registration: %v", err)
	}

	stub.mu.Lock()
	stub.toolDefs = []map[string]any{
		{
			"name":        "echo",
			"description": "CHANGED DESCRIPTION",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"y": map[string]any{"type": "integer"}},
			},
		},
		{
			"name":        "brand-new",
			"description": "appeared after reconnect",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
	stub.mu.Unlock()
	if err := pool.Reconnect(context.Background(), "srv"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}

	// Same-session registry stays pinned.
	n, err := mcp.RegisterServerTools(context.Background(), reg1, pool)
	if err != nil || n != 0 {
		t.Fatalf("expected pinned no-op (0, nil), got (%d, %v)", n, err)
	}
	if _, ok := reg1.Get("srv__brand-new"); ok {
		t.Error("boot registry must not gain brand-new mid-session")
	}

	// A fresh registry (new session) sees the changed live surface.
	reg2 := tools.NewRegistry()
	n2, err2 := mcp.RegisterServerTools(context.Background(), reg2, pool)
	if err2 != nil {
		t.Fatalf("fresh registration: %v", err2)
	}
	if n2 != 2 {
		t.Fatalf("fresh registry should see 2 tools, got %d", n2)
	}
	if _, ok := reg2.Get("srv__brand-new"); !ok {
		t.Error("fresh registry should see the brand-new tool (proves live list changed)")
	}
	echo2, _ := reg2.Get("srv__echo")
	if got := echo2.Description(nil, nil); got != "CHANGED DESCRIPTION" {
		t.Errorf("fresh registry should see changed description, got %q", got)
	}
	schema := echo2.InputSchema()
	if _, ok := schema.Properties["y"]; !ok {
		t.Error("fresh registry should see new schema property 'y'")
	}
	if _, ok := schema.Properties["x"]; ok {
		t.Error("fresh registry schema should no longer carry boot property 'x'")
	}
}

// TestRegisterServerTools_FailureAlsoPins verifies the documented lifecycle:
// even a wholly failed registration pins the registry, so a later re-pass is a
// no-op (boot failure is not recoverable mid-session by retrying).
func TestRegisterServerTools_FailureAlsoPins(t *testing.T) {
	mcp.ResetPinnedRegistriesForTest()
	t.Cleanup(mcp.ResetPinnedRegistriesForTest)

	stub := newStubMCPServer(t)
	stub.failMethods["tools/list"] = "boom"
	pool := connectStubPool(t, stub, "srv")
	reg := tools.NewRegistry()

	if _, err := mcp.RegisterServerTools(context.Background(), reg, pool); err == nil {
		t.Fatal("expected error when tools/list fails")
	}
	// Repair the server, then re-pass: still a pinned no-op.
	stub.mu.Lock()
	delete(stub.failMethods, "tools/list")
	stub.toolDefs = []map[string]any{echoToolDef()}
	stub.mu.Unlock()
	n, err := mcp.RegisterServerTools(context.Background(), reg, pool)
	if err != nil || n != 0 {
		t.Fatalf("failure should still pin: expected (0, nil), got (%d, %v)", n, err)
	}
	if reg.Len() != 0 {
		t.Errorf("no tools should be registered after failed-then-pinned, got %d", reg.Len())
	}
}

// ── Resource-tool test gaps (CR-BF01-4) ──────────────────────────────────────

// TestListMcpResources_PartialFailureNote covers the entries>0 && errs>0 branch:
// one server succeeds, one fails; output carries the good resources plus an
// honest Note appendix naming the failed server.
func TestListMcpResources_PartialFailureNote(t *testing.T) {
	good := newStubMCPServer(t)
	good.resources = []map[string]any{{"uri": "file://a.txt", "name": "a", "mimeType": "text/plain"}}
	bad := newStubMCPServer(t)
	bad.failMethods["resources/list"] = "list failed"
	pool := connectStubPool(t, good, "alpha")
	if err := pool.Connect(context.Background(), "beta", agtmcp.ServerConfig{
		Transport: agtmcp.TransportHTTP,
		HTTP:      &agtmcp.HTTPTransportConfig{URL: bad.url()},
	}); err != nil {
		t.Fatalf("connect beta: %v", err)
	}
	t.Cleanup(func() {
		if err := pool.Disconnect("beta"); err != nil {
			t.Errorf("disconnect beta: %v", err)
		}
	})

	tool := mcp.NewListMcpResourcesTool(pool)
	result, err := tool.Call(nil, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("partial failure with some resources should not be an error result: %v", result.Content)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "file://a.txt") {
		t.Errorf("expected the successful server's resource, got: %s", content)
	}
	if !strings.Contains(content, "Note: some servers failed") || !strings.Contains(content, "beta") {
		t.Errorf("expected honest Note appendix naming beta, got: %s", content)
	}
}

// TestListMcpResources_NilUseContext covers callContext's nil fallbacks (nil
// UseContext and UseContext with nil Ctx) — both must behave like Background.
func TestListMcpResources_NilUseContext(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.resources = []map[string]any{{"uri": "file://nil.txt", "name": "n"}}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewListMcpResourcesTool(pool)

	for _, uctx := range []*tools.UseContext{nil, {}} {
		result, err := tool.Call(nil, uctx, nil)
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if result.IsError {
			t.Fatalf("nil-ctx fallback should succeed, got: %v", result.Content)
		}
		if !strings.Contains(result.Content.(string), "file://nil.txt") {
			t.Errorf("expected resource with nil-ctx fallback, got: %v", result.Content)
		}
	}
}

func TestListMcpResources_CancelledContext(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.resources = []map[string]any{{"uri": "file://x.txt"}}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewListMcpResourcesTool(pool)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := tool.Call(nil, &tools.UseContext{Ctx: ctx}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when the call context is cancelled")
	}
}

func TestReadMcpResource_NilUseContext(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.readContent = map[string]any{"uri": "file://doc.txt", "mimeType": "text/plain", "text": "nil ctx body"}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewReadMcpResourceTool(pool)
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "file://doc.txt"})

	for _, uctx := range []*tools.UseContext{nil, {}} {
		result, err := tool.Call(in, uctx, nil)
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if result.IsError || result.Content != "nil ctx body" {
			t.Errorf("nil-ctx fallback should read content, got: %+v", result)
		}
	}
}

// TestReadMcpResource_EmptyContent covers the branch where the server returns a
// content item with neither text nor blob.
func TestReadMcpResource_EmptyContent(t *testing.T) {
	stub := newStubMCPServer(t)
	stub.readContent = map[string]any{"uri": "file://empty", "mimeType": "text/plain", "text": "", "blob": ""}
	pool := connectStubPool(t, stub, "srv")
	tool := mcp.NewReadMcpResourceTool(pool)
	in, _ := json.Marshal(mcp.ReadMcpResourceInput{ServerName: "srv", URI: "file://empty"})

	result, err := tool.Call(in, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("empty content should be a graceful result, got: %v", result.Content)
	}
	if !strings.Contains(result.Content.(string), "empty content") {
		t.Errorf("expected empty-content message, got: %v", result.Content)
	}
}

// TestListMcpResources_TruncatedAtCap covers the aggregated-output cap
// (CR-BF01-4 optional item; cache-first gate Q2 window-small): a resource-heavy
// server yields at most maxResourceEntries entries plus an honest truncation
// note, never an unbounded tool result.
func TestListMcpResources_TruncatedAtCap(t *testing.T) {
	stub := newStubMCPServer(t)
	for i := 0; i < 250; i++ {
		stub.resources = append(stub.resources, map[string]any{
			"uri":  fmt.Sprintf("file://r%d.txt", i),
			"name": fmt.Sprintf("r%d", i),
		})
	}
	pool := connectStubPool(t, stub, "heavy")

	tool := mcp.NewListMcpResourcesTool(pool)
	result, err := tool.Call(nil, useCtx(), nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	content := result.Content.(string)
	jsonPart := content
	if idx := strings.Index(content, "\n\nNote:"); idx >= 0 {
		jsonPart = content[:idx]
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &entries); err != nil {
		t.Fatalf("unmarshal entries: %v", err)
	}
	if len(entries) != 200 {
		t.Errorf("expected exactly 200 entries (cap), got %d", len(entries))
	}
	if !strings.Contains(content, "truncated at 200") {
		t.Errorf("expected honest truncation note, got tail: %s", content[len(content)-200:])
	}
}
