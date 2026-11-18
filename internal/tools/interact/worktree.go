package interact

import (
	"encoding/json"
	"fmt"

	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// ── EnterPlanMode ─────────────────────────────────────────────────────────────

// EnterPlanModeInput is the input schema for EnterPlanMode.
type EnterPlanModeInput struct {
	// Plan is a human-readable plan summary to display.
	Plan string `json:"plan,omitempty"`
}

// NewEnterPlanModeTool constructs the EnterPlanMode tool backed by the app
// state store (BF-09). It switches ToolPermissionContext.Mode to plan; the
// permission checker reads the live mode (CheckerConfig.ModeFn) and denies
// write operations while allowing reads. Registered by wire.go after the
// store exists. A nil store makes every call report an honest error.
func NewEnterPlanModeTool(store *state.AppStateStore) tools.Tool {
	return &enterPlanModeTool{store: store}
}

type enterPlanModeTool struct {
	tools.BaseTool
	store *state.AppStateStore
}

func (t *enterPlanModeTool) Name() string { return "EnterPlanMode" }

func (t *enterPlanModeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Enters plan mode, pausing execution until the plan is approved by the user.

Usage notes:
- Use when you want to present a plan and get user approval before proceeding
- Execution resumes only after ExitPlanMode is called (typically by the user)`
}

func (t *enterPlanModeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"plan": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Human-readable plan summary to display to the user",
			}),
		},
		[]string{},
	)
}

func (t *enterPlanModeTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *enterPlanModeTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *enterPlanModeTool) InterruptBehavior() tools.InterruptBehavior {
	return tools.InterruptBehaviorBlock
}
func (t *enterPlanModeTool) UserFacingName(_ tools.Input) string { return "EnterPlanMode" }

func (t *enterPlanModeTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if t.store == nil {
		return &tools.Result{IsError: true, Content: "EnterPlanMode: no app state store configured"}, nil
	}
	var in EnterPlanModeInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return &tools.Result{IsError: true, Content: fmt.Sprintf("EnterPlanMode: invalid input: %v", err)}, nil
		}
	}
	t.store.SetState(func(prev state.AppState) state.AppState {
		prev.ToolPermissionContext.Mode = types.PermissionModePlan
		return prev
	})
	text := "Entered plan mode. Research and plan only — write operations are blocked until ExitPlanMode is called. The user can also cycle modes with Shift+Tab."
	if in.Plan != "" {
		text += "\n\nPlan:\n" + in.Plan
	}
	return &tools.Result{Content: text}, nil
}

// ── ExitPlanMode ──────────────────────────────────────────────────────────────

// ExitPlanModeInput is the input schema for ExitPlanMode.
type ExitPlanModeInput struct {
	// Approved is true if the user approved the plan, false to abort.
	Approved bool `json:"approved"`
}

// NewExitPlanModeTool constructs the ExitPlanMode tool backed by the app
// state store (BF-09). It returns the mode to default (normal permission
// rules). Divergence from oracle (documented in tools.md): no interactive
// plan-approval dialog — approved=false simply exits without executing.
func NewExitPlanModeTool(store *state.AppStateStore) tools.Tool {
	return &exitPlanModeTool{store: store}
}

type exitPlanModeTool struct {
	tools.BaseTool
	store *state.AppStateStore
}

func (t *exitPlanModeTool) Name() string { return "ExitPlanMode" }

func (t *exitPlanModeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Exits plan mode. Set approved=true to proceed with the plan, false to abort.`
}

func (t *exitPlanModeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"approved": tools.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "True to approve and proceed; false to abort",
			}),
		},
		[]string{"approved"},
	)
}

func (t *exitPlanModeTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *exitPlanModeTool) IsReadOnly(_ tools.Input) bool        { return true }
func (t *exitPlanModeTool) UserFacingName(_ tools.Input) string  { return "ExitPlanMode" }

func (t *exitPlanModeTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if t.store == nil {
		return &tools.Result{IsError: true, Content: "ExitPlanMode: no app state store configured"}, nil
	}
	var in ExitPlanModeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("ExitPlanMode: invalid input: %v", err)}, nil
	}
	t.store.SetState(func(prev state.AppState) state.AppState {
		prev.ToolPermissionContext.Mode = types.PermissionModeDefault
		return prev
	})
	if in.Approved {
		return &tools.Result{Content: "Exited plan mode. The plan is approved — proceed with implementation under normal permission rules."}, nil
	}
	return &tools.Result{Content: "Exited plan mode without approval. Do not execute the plan; ask the user how to proceed."}, nil
}

// ── EnterWorktree ─────────────────────────────────────────────────────────────

// EnterWorktreeInput is the input schema for EnterWorktree.
type EnterWorktreeInput struct {
	// Name is an optional name for the new worktree branch.
	Name string `json:"name,omitempty"`
}

// EnterWorktreeTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core worktree manager.
var EnterWorktreeTool tools.Tool = &enterWorktreeTool{}

type enterWorktreeTool struct{ tools.BaseTool }

func (t *enterWorktreeTool) Name() string { return "EnterWorktree" }

func (t *enterWorktreeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Creates and switches to an isolated git worktree for the current session.

Usage notes:
- Only usable when the user explicitly asks to work in a worktree
- Creates a new branch based on HEAD inside .claude/worktrees/
- Use ExitWorktree to leave the worktree`
}

func (t *enterWorktreeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"name": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "Optional name for the worktree; random name generated if omitted",
			}),
		},
		[]string{},
	)
}

func (t *enterWorktreeTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *enterWorktreeTool) IsReadOnly(_ tools.Input) bool        { return false }

func (t *enterWorktreeTool) UserFacingName(input tools.Input) string {
	var in EnterWorktreeInput
	if json.Unmarshal(input, &in) == nil && in.Name != "" {
		return fmt.Sprintf("EnterWorktree(%s)", in.Name)
	}
	return "EnterWorktree"
}

func (t *enterWorktreeTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core worktree manager.
	return &tools.Result{IsError: true, Content: "EnterWorktree not yet implemented (TODO(dep))"}, nil
}

// ── ExitWorktree ──────────────────────────────────────────────────────────────

// ExitWorktreeInput is the input schema for ExitWorktree.
type ExitWorktreeInput struct {
	// Action is "keep" or "remove".
	Action string `json:"action"`
	// DiscardChanges, when true, removes a worktree even if it has uncommitted changes.
	DiscardChanges bool `json:"discard_changes,omitempty"`
}

// ExitWorktreeTool is the exported singleton instance.
// TODO(dep): Requires Agent-Core worktree manager.
var ExitWorktreeTool tools.Tool = &exitWorktreeTool{}

type exitWorktreeTool struct{ tools.BaseTool }

func (t *exitWorktreeTool) Name() string { return "ExitWorktree" }

func (t *exitWorktreeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Exits the current worktree session and returns to the original directory.

Usage notes:
- action="keep" leaves the worktree on disk; action="remove" deletes it
- Set discard_changes=true to force removal even with uncommitted changes`
}

func (t *exitWorktreeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"action": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "\"keep\" or \"remove\"",
				"enum":        []string{"keep", "remove"},
			}),
			"discard_changes": tools.PropSchema(map[string]any{
				"type":        "boolean",
				"description": "If true, removes even when there are uncommitted changes",
			}),
		},
		[]string{"action"},
	)
}

func (t *exitWorktreeTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *exitWorktreeTool) IsReadOnly(_ tools.Input) bool        { return false }
func (t *exitWorktreeTool) IsDestructive(_ tools.Input) bool     { return true }
func (t *exitWorktreeTool) UserFacingName(_ tools.Input) string  { return "ExitWorktree" }

func (t *exitWorktreeTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	// TODO(dep): Implement via Agent-Core worktree manager.
	return &tools.Result{IsError: true, Content: "ExitWorktree not yet implemented (TODO(dep))"}, nil
}
