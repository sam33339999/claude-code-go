// Package codemode provides the CodeMode tool (CM-01): one tool that executes
// model-written JavaScript composing other tools inside a permission-gated
// sandbox. See internal/codemode for the engine and catalog.
package codemode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/tunsuy/claude-code-go/internal/codemode"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// maxResultBytes caps the tool result fed back to the model (window-small,
// cache-first gate Q2). Larger outputs are truncated with an honest note.
const maxResultBytes = 32 << 10 // 32 KiB

// codeModeTool is the CodeMode tool instance.
type codeModeTool struct {
	tools.BaseTool
	reg     *tools.Registry
	checker permissions.Checker
	engine  codemode.Runtime
	catalog []codemode.CatalogEntry
	callSeq atomic.Int64
}

// NewCodeModeTool constructs the CodeMode tool. reg supplies the callable
// tool surface and the discovery catalog; checker enforces the full
// permission pipeline on EVERY inner call (sandboxing is not authorization);
// engine is the sandbox runtime (moejs primary). The catalog is snapshotted
// at construction — the tool surface is session-pinned (cache-stable).
func NewCodeModeTool(reg *tools.Registry, checker permissions.Checker, engine codemode.Runtime) tools.Tool {
	return &codeModeTool{
		reg:     reg,
		checker: checker,
		engine:  engine,
		catalog: codemode.BuildCatalog(reg),
	}
}

func (t *codeModeTool) Name() string { return "CodeMode" }

func (t *codeModeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Execute JavaScript code that composes tool calls in one sandboxed run.
Write the BODY of an async function: use await, loops, branches, and return the
final value. Only the returned value and console.log output come back to you —
intermediate data stays in the sandbox (fewer round trips, less context).

Available inside the sandbox:
- tools.<Name>(input) — call any registered tool (async; each call goes through
  the normal permission pipeline and may prompt the user). Promise.all runs
  calls in parallel.
- codemode.search(query) — rank tool names by keyword (progressive discovery).
- codemode.describe(name) — full description + input schema of one tool.
- console.log(...) — captured output.
No filesystem, network, env, or eval access. Execution is bounded by a timeout
and a memory limit.

Tool catalog (call codemode.describe(name) for input schemas):
` + codemode.CatalogText(t.catalog)
}

func (t *codeModeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"code": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "JavaScript async function body composing tools.* calls; return the final result",
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
	Code string `json:"code"`
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
	// AbortCh propagation: cancel a derived context when the abort fires.
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

	opts := codemode.Options{
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
	}

	result, err := t.engine.Execute(ctx, in.Code, t.bridge(ctx, uctx), opts)
	if err != nil {
		return &tools.Result{IsError: true, Content: err.Error()}, nil
	}
	return &tools.Result{Content: formatResult(result)}, nil
}

// bridge builds the CallFunc that dispatches one inner tool call through the
// SAME flow the engine uses (orchestration.go): validate → tool-specific
// CheckPermissions → checker.RequestPermission (HIL) → Call.
func (t *codeModeTool) bridge(ctx context.Context, outer *tools.UseContext) codemode.CallFunc {
	return func(_ context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		tool, ok := t.reg.Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown tool %q (use codemode.search to discover tools)", name)
		}
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
			req := permissions.PermissionRequest{
				ToolName:   name,
				ToolUseID:  fmt.Sprintf("codemode-%d", t.callSeq.Add(1)),
				Input:      tools.Input(input),
				ToolResult: prelim,
			}
			perm, err := t.checker.RequestPermission(ctx, req, inner)
			if err != nil {
				return nil, fmt.Errorf("%s: permission check: %w", name, err)
			}
			if perm.Behavior == tools.PermissionDeny {
				reason := perm.Reason
				if reason == "" {
					reason = "permission denied by user or policy"
				}
				return nil, fmt.Errorf("%s: denied: %s", name, reason)
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

// formatResult renders logs + return value, capped at maxResultBytes.
func formatResult(r *codemode.Result) string {
	var sb strings.Builder
	if len(r.Logs) > 0 {
		sb.WriteString(strings.Join(r.Logs, "\n"))
		sb.WriteString("\n")
	}
	if r.Value != nil {
		if s, ok := r.Value.(string); ok {
			// String return values pass through raw (no JSON quoting).
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
	if len(out) > maxResultBytes {
		return out[:maxResultBytes] + "\n…(truncated: result exceeds 32 KiB; return less data or write it to a file via tools)"
	}
	return out
}
