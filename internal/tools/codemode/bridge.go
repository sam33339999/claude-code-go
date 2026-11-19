package codemode

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tunsuy/glaude/internal/codemode"
	"github.com/tunsuy/glaude/internal/permissions"
	"github.com/tunsuy/glaude/internal/tools"
)

// bridge builds the CallFunc dispatching one inner tool call through the SAME
// flow the engine uses (orchestration.go). Permission layers (CM-02/CM-03):
//  1. tool CheckPermissions + checker CanUseTool — Deny always wins, live
//     rule-Allows are re-checked every call (Shift+Tab mode changes apply).
//  2. execution-scope batch approval (execAllow): an Ask the user approved
//     pre-approves the SAME tool for the rest of this execution.
//  3. persisted CodeMode-scoped approvals (CM-03): consulted only on Ask.
//  4. batch-writes gate (CM-03): one approval covers ALL writes of the
//     execution when input batch_writes=true.
func (t *codeModeTool) bridge(ctx context.Context, outer *tools.UseContext, st *execState) codemode.CallFunc {
	return func(_ context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
		tool, ok := t.reg.Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown tool %q (use codemode.search to discover tools)", name)
		}
		st.recordTool(name)
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

		isWrite := !tool.IsReadOnly(tools.Input(input))
		if t.checker != nil {
			if err := t.authorize(ctx, name, input, inner, prelim, st, isWrite); err != nil {
				return nil, err
			}
		}

		res, err := tool.Call(tools.Input(input), inner, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if res.IsError {
			return nil, fmt.Errorf("%s: %v", name, res.Content)
		}
		if isWrite {
			st.recordSideEffect(name, input) // audit: only executed writes
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

// authorize runs the checker layers for one inner call (CM-03 v3).
func (t *codeModeTool) authorize(
	ctx context.Context, name string, input json.RawMessage,
	inner *tools.UseContext, prelim tools.PermissionResult, st *execState, isWrite bool,
) error {
	st.mu.Lock()
	preApproved := st.execAllow[name]
	st.mu.Unlock()
	if preApproved {
		return nil
	}

	// CanUseTool first: Deny wins over everything (including persisted
	// approvals); direct rule-Allows are never cached so live permission-mode
	// changes keep applying.
	can, err := t.checker.CanUseTool(ctx, name, tools.Input(input), inner)
	if err != nil {
		return fmt.Errorf("%s: permission check: %w", name, err)
	}
	if can.Behavior == tools.PermissionDeny {
		return fmt.Errorf("%s: denied: %s", name, can.Reason)
	}
	if can.Behavior == tools.PermissionAllow {
		return nil
	}

	// Ask path.
	if st.batchWrites && isWrite {
		return t.writeGate(ctx, name, input, inner, prelim, st)
	}
	if t.hasApproval(name) {
		return nil // persisted CodeMode-scoped approval (user-granted, revocable)
	}
	return t.requestUser(ctx, name, input, inner, prelim, st, "")
}

// requestUser runs the interactive Ask flow and caches an approval for this
// execution; a CodeMode-scoped "always allow" choice is also persisted.
func (t *codeModeTool) requestUser(
	ctx context.Context, name string, input json.RawMessage,
	inner *tools.UseContext, prelim tools.PermissionResult, st *execState, note string,
) error {
	req := permissions.PermissionRequest{
		ToolName:     name,
		ToolUseID:    fmt.Sprintf("codemode-%d", t.callSeq.Add(1)),
		Input:        tools.Input(input),
		ToolResult:   prelim,
		FromCodeMode: true,
		Note:         note,
	}
	perm, err := t.checker.RequestPermission(ctx, req, inner)
	if err != nil {
		return fmt.Errorf("%s: permission check: %w", name, err)
	}
	if perm.Behavior == tools.PermissionDeny {
		reason := perm.Reason
		if reason == "" {
			reason = "permission denied by user or policy"
		}
		return fmt.Errorf("%s: denied: %s", name, reason)
	}
	st.mu.Lock()
	st.execAllow[name] = true
	st.mu.Unlock()
	if perm.AlwaysInCodeMode {
		t.saveApproval(name)
	}
	return nil
}

// writeGate implements the CM-03 batch-writes gate: the FIRST write call of
// the execution asks once (with a scope note); the decision covers ALL later
// write calls. Concurrent writes wait for the decision. Reads never enter the
// gate. Full write-plan preview (two-phase execution) is deferred — the dialog
// shows the first concrete write, and the note states the execution-wide scope.
func (t *codeModeTool) writeGate(
	ctx context.Context, name string, input json.RawMessage,
	inner *tools.UseContext, prelim tools.PermissionResult, st *execState,
) error {
	st.mu.Lock()
	if st.gateCh == nil {
		st.gateCh = make(chan struct{})
		st.mu.Unlock()
		note := "CodeMode batch-writes gate: approving allows ALL write tool calls " +
			"in this CodeMode execution (reads keep their normal permission flow). " +
			"First write shown below."
		err := t.requestUser(ctx, name, input, inner, prelim, st, note)
		st.mu.Lock()
		st.gateApproved = err == nil
		close(st.gateCh)
		st.mu.Unlock()
		return err
	}
	ch := st.gateCh
	st.mu.Unlock()

	select {
	case <-ch:
	case <-ctx.Done():
		return fmt.Errorf("%s: write gate: %w", name, ctx.Err())
	}
	st.mu.Lock()
	approved := st.gateApproved
	st.mu.Unlock()
	if !approved {
		return fmt.Errorf("%s: denied: CodeMode write gate was rejected for this execution", name)
	}
	return nil
}
