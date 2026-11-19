package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	agtmcp "github.com/tunsuy/glaude/internal/mcp"
	"github.com/tunsuy/glaude/internal/tools"
)

// ── Dynamic per-server tool registration ─────────────────────────────────────

// pinnedRegistries marks registries whose MCP server-tool registration has
// already run. Keys are process-lifetime boot registries; the marker is what
// enforces session pinning (see RegisterServerTools).
//
// Hygiene: entries are never pruned and there is no production reset API — a
// registry's lifetime is its session, and the marker intentionally persists
// for the process lifetime so a re-pass can never re-register mid-session.
// Long-lived processes / many-container test runs accumulate markers slowly;
// tests may clear them via the test-only reset hook (export_test.go).
var pinnedRegistries sync.Map // map[*tools.Registry]struct{}

// listToolsTimeout bounds a single server's boot-time tools/list request so a
// server that handshakes but hangs on tools/list cannot block startup forever
// (CR-BF01-2). It is a var (not a const) so tests can inject a short timeout;
// production keeps 10s. A timeout is a per-server, non-fatal ListTools failure.
var listToolsTimeout = 10 * time.Second

// maxResourceEntries caps the aggregated ListMcpResources output so a
// resource-heavy server cannot produce an unbounded tool result (cache-first
// gate Q2: keep the window small). Excess entries are dropped with an honest
// truncation note; the provider-facing schema stays byte-stable.
const maxResourceEntries = 200

// RegisterServerTools discovers the tools exposed by every connected MCP
// server in pool and registers them into reg using the agtkeel kernel adapter
// (agtmcp.AdaptToTool). Each tool is named "{serverName}__{normalizedToolName}"
// and forwards Call to the server's MCPClient.CallTool.
//
// Session pinning (provider prefix-cache stability): registration is a
// boot-time, one-shot operation per registry. The first call registers the
// discovered tools; every later call is a no-op returning (0, nil), so the
// session's tool name set and schema bytes never change mid-session — even
// after a reconnect whose live tool list differs. Differences take effect at
// the next session startup (dynamic connection state belongs in /mcp output,
// never in Description/InputSchema). See docs/PROMPT_CACHE_AND_COST.md
// ("Pinned MCP placeholders", Boundaries §4).
//
// Servers are processed in name order for deterministic registration.
// A nil pool or empty pool registers nothing and returns (0, nil). Per-server
// ListTools failures are collected into the returned error (errors.Join)
// without preventing registration of the remaining servers' tools.
//
// Each server's ListTools runs under a per-server deadline (listToolsTimeout,
// default 10s) so a server that completes the handshake but hangs on
// tools/list cannot block startup indefinitely; a timeout is recorded as a
// non-fatal per-server failure (CR-BF01-2).
//
// Pinning lifecycle: a registry's lifetime is its session; the pinning marker
// is retained for the process lifetime; and a registration that fails (even
// wholly) still pins the registry — a later re-pass is a no-op, so a boot-time
// failure is not recoverable mid-session by retrying this function.
func RegisterServerTools(ctx context.Context, reg *tools.Registry, pool *agtmcp.Pool) (int, error) {
	if reg == nil || pool == nil {
		return 0, nil
	}
	if _, pinned := pinnedRegistries.LoadOrStore(reg, struct{}{}); pinned {
		return 0, nil
	}
	conns := pool.GetConnected()
	sort.Slice(conns, func(i, j int) bool { return conns[i].Name < conns[j].Name })

	registered := 0
	var errs []error
	for _, conn := range conns {
		if conn.Client == nil {
			continue
		}
		lctx, cancel := context.WithTimeout(ctx, listToolsTimeout)
		defs, err := conn.Client.ListTools(lctx)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("mcp: list tools for server %q: %w", conn.Name, err))
			continue
		}
		for _, def := range defs {
			reg.Replace(agtmcp.AdaptToTool(conn.Name, def, conn.Client))
			registered++
		}
	}
	return registered, errors.Join(errs...)
}

// callContext extracts the cancellation context from a UseContext, tolerating
// a nil UseContext or nil Ctx (tests and early-boot callers pass nil).
func callContext(uctx *tools.UseContext) context.Context {
	if uctx != nil && uctx.Ctx != nil {
		return uctx.Ctx
	}
	return context.Background()
}

// serverClient resolves a named server's live client from the pool.
// It returns an honest error when the pool is missing, the server is unknown,
// or the server is not currently connected.
func serverClient(pool *agtmcp.Pool, name string) (agtmcp.MCPClient, error) {
	if pool == nil {
		return nil, fmt.Errorf("no MCP pool configured")
	}
	for _, conn := range pool.GetAll() {
		if conn.Name != name {
			continue
		}
		if conn.Status != agtmcp.StatusConnected || conn.Client == nil {
			return nil, fmt.Errorf("MCP server %q is not connected (status: %s)", name, conn.Status)
		}
		return conn.Client, nil
	}
	return nil, fmt.Errorf("unknown MCP server %q", name)
}

// ── ListMcpResources ──────────────────────────────────────────────────────────

// ListMcpResourcesInput is the input schema for ListMcpResources.
type ListMcpResourcesInput struct {
	// ServerName filters to a specific MCP server (optional).
	ServerName string `json:"server_name,omitempty"`
}

// mcpResourceEntry is one resource in the ListMcpResources output, annotated
// with the server it came from.
type mcpResourceEntry struct {
	Server      string `json:"server"`
	URI         string `json:"uri"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// NewListMcpResourcesTool constructs the ListMcpResources tool backed by the
// given MCP connection pool. The pool is injected at registration time
// (bootstrap wires it); a nil pool makes every call report that no MCP pool
// is configured. With an empty pool the tool reports an empty resource list.
func NewListMcpResourcesTool(pool *agtmcp.Pool) tools.Tool {
	return &listMcpResourcesTool{pool: pool}
}

type listMcpResourcesTool struct {
	tools.BaseTool
	pool *agtmcp.Pool
}

func (t *listMcpResourcesTool) Name() string { return "ListMcpResources" }

func (t *listMcpResourcesTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Lists all available resources from connected MCP servers.

Usage notes:
- Optionally filter by server_name
- Returns resource URIs, names, and descriptions`
}

func (t *listMcpResourcesTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"server_name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional MCP server name to filter results",
			}),
		},
		[]string{},
	)
}

func (t *listMcpResourcesTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *listMcpResourcesTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *listMcpResourcesTool) UserFacingName(_ tools.Input) string  { return "ListMcpResources" }

func (t *listMcpResourcesTool) Call(input tools.Input, uctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in ListMcpResourcesInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return &tools.Result{IsError: true, Content: fmt.Sprintf("ListMcpResources: invalid input: %v", err)}, nil
		}
	}
	if t.pool == nil {
		return &tools.Result{IsError: true, Content: "ListMcpResources: no MCP pool configured"}, nil
	}
	ctx := callContext(uctx)

	// Collect the servers to query: one named server, or all connected ones.
	var conns []*agtmcp.ServerConnection
	if in.ServerName != "" {
		client, err := serverClient(t.pool, in.ServerName)
		if err != nil {
			return &tools.Result{IsError: true, Content: "ListMcpResources: " + err.Error()}, nil
		}
		conns = []*agtmcp.ServerConnection{{Name: in.ServerName, Client: client}}
	} else {
		conns = t.pool.GetConnected()
		sort.Slice(conns, func(i, j int) bool { return conns[i].Name < conns[j].Name })
		if len(conns) == 0 {
			return &tools.Result{Content: "No MCP resources available: no MCP servers are connected."}, nil
		}
	}

	var entries []mcpResourceEntry
	var errs []string
	truncated := false
	for _, conn := range conns {
		resources, err := conn.Client.ListResources(ctx)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", conn.Name, err))
			continue
		}
		for _, r := range resources {
			if len(entries) >= maxResourceEntries {
				truncated = true
				break
			}
			entries = append(entries, mcpResourceEntry{
				Server:      conn.Name,
				URI:         r.URI,
				Name:        r.Name,
				Description: r.Description,
				MIMEType:    r.MIMEType,
			})
		}
		if truncated {
			break
		}
	}

	if len(entries) == 0 && len(errs) > 0 {
		return &tools.Result{IsError: true, Content: "ListMcpResources: " + strings.Join(errs, "\n")}, nil
	}
	if len(entries) == 0 {
		return &tools.Result{Content: "No MCP resources found on the connected servers."}, nil
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("ListMcpResources: marshal resources: %w", err)
	}
	content := string(raw)
	if truncated {
		content += fmt.Sprintf("\n\nNote: listing truncated at %d entries; use server_name to narrow the result.", maxResourceEntries)
	}
	if len(errs) > 0 {
		content += "\n\nNote: some servers failed to list resources:\n- " + strings.Join(errs, "\n- ")
	}
	return &tools.Result{Content: content}, nil
}

// ── ReadMcpResource ───────────────────────────────────────────────────────────

// ReadMcpResourceInput is the input schema for ReadMcpResource.
type ReadMcpResourceInput struct {
	// ServerName is the MCP server to query (required).
	ServerName string `json:"server_name"`
	// URI is the resource URI to read (required).
	URI string `json:"uri"`
}

// NewReadMcpResourceTool constructs the ReadMcpResource tool backed by the
// given MCP connection pool. The pool is injected at registration time
// (bootstrap wires it); a nil pool makes every call report that no MCP pool
// is configured.
func NewReadMcpResourceTool(pool *agtmcp.Pool) tools.Tool {
	return &readMcpResourceTool{pool: pool}
}

type readMcpResourceTool struct {
	tools.BaseTool
	pool *agtmcp.Pool
}

func (t *readMcpResourceTool) Name() string { return "ReadMcpResource" }

func (t *readMcpResourceTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Reads the contents of a specific MCP resource by URI.`
}

func (t *readMcpResourceTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"server_name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The MCP server name",
			}),
			"uri": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The resource URI to read",
			}),
		},
		[]string{"server_name", "uri"},
	)
}

func (t *readMcpResourceTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *readMcpResourceTool) IsReadOnly(_ tools.Input) bool        { return true }

func (t *readMcpResourceTool) UserFacingName(input tools.Input) string {
	var in ReadMcpResourceInput
	if json.Unmarshal(input, &in) == nil && in.URI != "" {
		return fmt.Sprintf("ReadMcpResource(%s)", in.URI)
	}
	return "ReadMcpResource"
}

func (t *readMcpResourceTool) Call(input tools.Input, uctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in ReadMcpResourceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("ReadMcpResource: invalid input: %v", err)}, nil
	}
	if in.ServerName == "" {
		return &tools.Result{IsError: true, Content: "ReadMcpResource: missing required field 'server_name'"}, nil
	}
	if in.URI == "" {
		return &tools.Result{IsError: true, Content: "ReadMcpResource: missing required field 'uri'"}, nil
	}
	client, err := serverClient(t.pool, in.ServerName)
	if err != nil {
		return &tools.Result{IsError: true, Content: "ReadMcpResource: " + err.Error()}, nil
	}

	content, err := client.ReadResource(callContext(uctx), in.URI)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("ReadMcpResource: read %q from server %q: %v", in.URI, in.ServerName, err)}, nil
	}
	if content.Text != "" {
		return &tools.Result{Content: content.Text}, nil
	}
	if content.Blob != "" {
		return &tools.Result{Content: fmt.Sprintf("Resource %s (%s): binary content, %d base64 characters", in.URI, content.MIMEType, len(content.Blob))}, nil
	}
	return &tools.Result{Content: fmt.Sprintf("Resource %s: empty content", in.URI)}, nil
}
