// Package codemode provides the CodeMode tool (CM-01/CM-02): one tool that
// executes model-written JavaScript/TypeScript composing other tools inside a
// permission-gated sandbox. See internal/codemode for the engine and catalog.
package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tunsuy/claude-code-go/internal/codemode"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// maxResultBytes caps the inline tool result (window-small, cache-first gate
// Q2). Larger outputs spill to a file with a pointer + preview (CM-02 v2).
const maxResultBytes = 32 << 10 // 32 KiB

// spillPreviewBytes is the inline preview kept when a result spills to disk.
const spillPreviewBytes = 4 << 10 // 4 KiB

// maxModuleBytes caps one imported module's source (CM-02 v2).
const maxModuleBytes = 256 << 10 // 256 KiB

// historyCodeCap bounds the code stored/returned per history entry.
const historyCodeCap = 2000

// codeModeTool is the CodeMode tool instance.
type codeModeTool struct {
	tools.BaseTool
	reg     *tools.Registry
	checker permissions.Checker
	engine  codemode.Runtime
	catalog []codemode.CatalogEntry
	dataDir string // history/spill/modules root ("" disables persistence features)
	callSeq atomic.Int64
}

// NewCodeModeTool constructs the CodeMode tool. reg supplies the callable
// tool surface and the discovery catalog; checker enforces the full
// permission pipeline on EVERY inner call (sandboxing is not authorization);
// engine is the sandbox runtime (moejs primary); dataDir roots execution
// history, result spill files and the controlled import module directory
// ("" disables those v2 features). The catalog is snapshotted at construction
// — the tool surface is session-pinned (cache-stable).
func NewCodeModeTool(reg *tools.Registry, checker permissions.Checker, engine codemode.Runtime, dataDir string) tools.Tool {
	return &codeModeTool{
		reg:     reg,
		checker: checker,
		engine:  engine,
		catalog: codemode.BuildCatalog(reg),
		dataDir: dataDir,
	}
}

func (t *codeModeTool) Name() string { return "CodeMode" }

func (t *codeModeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Execute JavaScript (or TypeScript with typescript=true) code that composes
tool calls in one sandboxed run. Write the BODY of an async function: use await,
loops, branches, and return the final value. Only the returned value and
console.log output come back to you — intermediate data stays in the sandbox
(fewer round trips, less context).

Available inside the sandbox:
- tools.<Name>(input) — call any registered tool (async; each call goes through
  the normal permission pipeline). Promise.all runs calls in parallel.
  Batch approval: when a tool prompts and you approve it, later calls to the
  SAME tool within this execution are pre-approved (execution scope only).
- codemode.search(query) — rank tool names by keyword (progressive discovery).
- codemode.describe(name) — full description + input schema of one tool.
- codemode.history(limit) — recent CodeMode executions (id, code, outcome) for
  reuse: read a past entry and adapt its code.
- import { x } from "name" — controlled shared modules from the host module
  directory (if configured by the host; otherwise imports are disabled).
- console.log(...) — captured output.
No filesystem, network, env, or eval access. Execution is bounded by a timeout
and a memory limit. Results over 32 KiB spill to a file you can Read.

Tool catalog (call codemode.describe(name) for input schemas):
` + codemode.CatalogText(t.catalog)
}

func (t *codeModeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"code": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "JavaScript/TypeScript async function body composing tools.* calls; return the final result",
			}),
			"typescript": tools.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "Compile the code as TypeScript (type stripping); default false",
			}),
		},
		[]string{"code"},
	)
}

func (t *codeModeTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *codeModeTool) IsReadOnly(_ tools.Input) bool        { return false }
func (t *codeModeTool) UserFacingName(_ tools.Input) string  { return "CodeMode" }

// codeModeInput is the tool input.
type codeModeInput struct {
	Code       string `json:"code"`
	Typescript bool   `json:"typescript,omitempty"`
}

// historyEntry is one recorded execution (CM-02 v2).
type historyEntry struct {
	ID         string   `json:"id"`
	Timestamp  string   `json:"ts"`
	OK         bool     `json:"ok"`
	DurationMS int64    `json:"duration_ms"`
	Typescript bool     `json:"typescript,omitempty"`
	Code       string   `json:"code"`
	Tools      []string `json:"tools,omitempty"`
	Preview    string   `json:"preview,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func (t *codeModeTool) Call(input tools.Input, uctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in codeModeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("CodeMode: invalid input: %v", err)}, nil
	}
	if strings.TrimSpace(in.Code) == "" {
		return &tools.Result{IsError: true, Content: "CodeMode: code is required"}, nil
	}
	if t.engine == nil {
		return &tools.Result{IsError: true, Content: "CodeMode: no sandbox engine configured"}, nil
	}

	ctx := context.Background()
	var abortCh <-chan struct{}
	if uctx != nil {
		if uctx.Ctx != nil {
			ctx = uctx.Ctx
		}
		abortCh = uctx.AbortCh
	}
	if abortCh != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-abortCh:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	execID := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), t.callSeq.Add(1))
	var (
		mu        sync.Mutex
		usedTools []string
		seen      = make(map[string]bool)
		// execAllow: tools the user explicitly approved via Ask during THIS
		// execution (CM-02 v2 batch approval). Direct rule-Allows are NOT
		// cached, so live mode changes (Shift+Tab) still apply to them.
		execAllow = make(map[string]bool)
	)
	recordTool := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		if !seen[name] {
			seen[name] = true
			usedTools = append(usedTools, name)
		}
	}

	opts := codemode.Options{
		Typescript: in.Typescript,
		Search: func(query string) (any, error) {
			return codemode.Search(t.catalog, query), nil
		},
		Describe: func(name string) (any, error) {
			e, ok := codemode.Describe(t.catalog, name)
			if !ok {
				return nil, fmt.Errorf("unknown tool %q", name)
			}
			return e, nil
		},
		History: func(limit int) (any, error) {
			return t.readHistory(limit), nil
		},
	}
	if t.dataDir != "" {
		opts.ImportModule = t.resolveModule
	}

	start := time.Now()
	result, execErr := t.engine.Execute(ctx, in.Code, t.bridge(ctx, uctx, execAllow, recordTool), opts)
	duration := time.Since(start).Milliseconds()

	var content string
	if execErr != nil {
		content = execErr.Error()
	} else {
		content = t.formatResult(execID, result)
	}
	t.recordHistory(historyEntry{
		ID:         execID,
		Timestamp:  start.UTC().Format(time.RFC3339),
		OK:         execErr == nil,
		DurationMS: duration,
		Typescript: in.Typescript,
		Code:       capString(in.Code, historyCodeCap),
		Tools:      usedTools,
		Preview:    capString(content, 200),
		Error:      errorString(execErr),
	})

	if execErr != nil {
		return &tools.Result{IsError: true, Content: content}, nil
	}
	return &tools.Result{Content: content}, nil
}

// bridge builds the CallFunc dispatching one inner tool call through the SAME
// flow the engine uses (orchestration.go), plus CM-02 v2 batch approval:
// an Ask that the user approves pre-approves the SAME tool for the rest of
// this execution (execution scope; never persisted).
func (t *codeModeTool) bridge(ctx context.Context, outer *tools.UseContext, execAllow map[string]bool, recordTool func(string)) codemode.CallFunc {
	var allowMu sync.Mutex
	return func(_ context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		tool, ok := t.reg.Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown tool %q (use codemode.search to discover tools)", name)
		}
		recordTool(name)
		inner := &tools.UseContext{Ctx: ctx}
		if outer != nil {
			inner.AbortCh = outer.AbortCh
			inner.PermCtx = outer.PermCtx
			inner.Coordinator = outer.Coordinator
			inner.AgentID = outer.AgentID
			inner.MemoryStore = outer.MemoryStore
		}

		vr, err := tool.ValidateInput(tools.Input(input), inner)
		if err != nil {
			return nil, fmt.Errorf("%s: input validation: %w", name, err)
		}
		if !vr.OK {
			return nil, fmt.Errorf("%s: invalid input: %s", name, vr.Reason)
		}

		prelim, err := tool.CheckPermissions(tools.Input(input), inner)
		if err != nil {
			return nil, fmt.Errorf("%s: permission check: %w", name, err)
		}
		if prelim.Behavior == tools.PermissionDeny {
			return nil, fmt.Errorf("%s: denied: %s", name, prelim.Reason)
		}

		if t.checker != nil {
			allowMu.Lock()
			preApproved := execAllow[name]
			allowMu.Unlock()
			if !preApproved {
				req := permissions.PermissionRequest{
					ToolName:   name,
					ToolUseID:  fmt.Sprintf("codemode-%d", t.callSeq.Add(1)),
					Input:      tools.Input(input),
					ToolResult: prelim,
				}
				// CanUseTool first: only an explicit Ask→user-approval populates
				// the execution cache; direct rule-Allows are re-checked every
				// call so live permission-mode changes keep applying.
				can, cerr := t.checker.CanUseTool(ctx, name, tools.Input(input), inner)
				if cerr != nil {
					return nil, fmt.Errorf("%s: permission check: %w", name, cerr)
				}
				if can.Behavior == tools.PermissionDeny {
					return nil, fmt.Errorf("%s: denied: %s", name, can.Reason)
				}
				if can.Behavior == tools.PermissionAsk {
					perm, perr := t.checker.RequestPermission(ctx, req, inner)
					if perr != nil {
						return nil, fmt.Errorf("%s: permission check: %w", name, perr)
					}
					if perm.Behavior == tools.PermissionDeny {
						reason := perm.Reason
						if reason == "" {
							reason = "permission denied by user or policy"
						}
						return nil, fmt.Errorf("%s: denied: %s", name, reason)
					}
					allowMu.Lock()
					execAllow[name] = true
					allowMu.Unlock()
				}
			}
		}

		res, err := tool.Call(tools.Input(input), inner, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if res.IsError {
			return nil, fmt.Errorf("%s: %v", name, res.Content)
		}
		switch c := res.Content.(type) {
		case string:
			return json.Marshal(c)
		case nil:
			return json.Marshal(nil)
		default:
			return json.Marshal(c)
		}
	}
}

// formatResult renders logs + return value; results over maxResultBytes spill
// to dataDir/spill/<execID>.txt with a pointer + preview (CM-02 v2).
func (t *codeModeTool) formatResult(execID string, r *codemode.Result) string {
	var sb strings.Builder
	if len(r.Logs) > 0 {
		sb.WriteString(strings.Join(r.Logs, "\n"))
		sb.WriteString("\n")
	}
	if r.Value != nil {
		if s, ok := r.Value.(string); ok {
			if sb.Len() > 0 {
				sb.WriteString("Return value:\n")
			}
			sb.WriteString(s)
		} else {
			raw, err := json.MarshalIndent(r.Value, "", "  ")
			if err != nil {
				raw = []byte(fmt.Sprintf("%v", r.Value))
			}
			if sb.Len() > 0 {
				sb.WriteString("Return value:\n")
			}
			sb.Write(raw)
		}
	}
	out := sb.String()
	if out == "" {
		return "(code completed with no return value or output)"
	}
	if len(out) <= maxResultBytes {
		return out
	}
	if t.dataDir != "" {
		spillDir := filepath.Join(t.dataDir, "spill")
		if err := os.MkdirAll(spillDir, 0o755); err == nil {
			path := filepath.Join(spillDir, execID+".txt")
			if werr := os.WriteFile(path, []byte(out), 0o644); werr == nil {
				return out[:spillPreviewBytes] + fmt.Sprintf(
					"\n…(result is %d bytes; full output saved to %s — use tools.Read or tools.Grep on that file)", len(out), path)
			}
		}
	}
	return out[:maxResultBytes] + "\n…(truncated: result exceeds 32 KiB; return less data or write it to a file via tools)"
}

// resolveModule serves controlled imports from dataDir/modules (CM-02 v2).
// Specifiers are flat names (no paths); the host controls the directory.
func (t *codeModeTool) resolveModule(specifier string) (string, error) {
	name := strings.TrimSuffix(specifier, ".js")
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid module specifier %q (flat names only)", specifier)
	}
	path := filepath.Join(t.dataDir, "modules", name+".js")
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("module %q not available (host module directory)", name)
	}
	if info.Size() > maxModuleBytes {
		return "", fmt.Errorf("module %q exceeds %d bytes", name, maxModuleBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read module %q: %w", name, err)
	}
	return string(raw), nil
}

// recordHistory appends one execution entry (best-effort, never fatal).
func (t *codeModeTool) recordHistory(e historyEntry) {
	if t.dataDir == "" {
		return
	}
	if err := os.MkdirAll(t.dataDir, 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(t.dataDir, "history.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
}

// readHistory returns the most recent entries (newest first) for
// codemode.history(limit) inside the sandbox.
func (t *codeModeTool) readHistory(limit int) []historyEntry {
	if t.dataDir == "" {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	raw, err := os.ReadFile(filepath.Join(t.dataDir, "history.jsonl"))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var entries []historyEntry
	for i := len(lines) - 1; i >= 0 && len(entries) < limit; i-- {
		var e historyEntry
		if json.Unmarshal([]byte(lines[i]), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

func capString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return capString(err.Error(), 200)
}
