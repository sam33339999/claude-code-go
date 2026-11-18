package interact_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/state"
	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/interact"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// ── TodoWriteTool ─────────────────────────────────────────────────────────────

func newTodoTool() (tools.Tool, *state.AppStateStore) {
	store := state.NewAppStateStore(state.AppState{})
	return interact.NewTodoWriteTool(store), store
}

func callTodo(t *testing.T, tool tools.Tool, items []types.TodoItem) *tools.Result {
	t.Helper()
	in, err := json.Marshal(interact.TodoWriteInput{Todos: items})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	return result
}

func TestTodoWriteTool_Metadata(t *testing.T) {
	tool, _ := newTodoTool()
	if tool.Name() != "TodoWrite" {
		t.Errorf("expected TodoWrite, got %q", tool.Name())
	}
	if tool.IsConcurrencySafe(nil) {
		t.Error("TodoWrite must not be concurrency-safe")
	}
	if tool.IsReadOnly(nil) {
		t.Error("TodoWrite must not be read-only")
	}
	schema := tool.InputSchema()
	if _, ok := schema.Properties["todos"]; !ok {
		t.Error("schema missing 'todos'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "todos" {
		t.Errorf("expected Required=[todos], got %v", schema.Required)
	}
}

func TestTodoWriteTool_WritesAndReplacesList(t *testing.T) {
	tool, store := newTodoTool()
	first := []types.TodoItem{
		{ID: "1", Content: "task one", Status: types.TodoInProgress},
		{ID: "2", Content: "task two", Status: types.TodoPending},
	}
	result := callTodo(t, tool, first)
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	if !strings.Contains(result.Content.(string), "Todos have been modified successfully") {
		t.Errorf("expected oracle confirmation text, got %v", result.Content)
	}
	got := store.GetState().Todos
	if len(got) != 2 || got[0].Content != "task one" {
		t.Fatalf("expected 2 todos stored, got %+v", got)
	}
	// Replacement semantics: a second call replaces, not appends.
	callTodo(t, tool, []types.TodoItem{{ID: "3", Content: "only task", Status: types.TodoPending}})
	got = store.GetState().Todos
	if len(got) != 1 || got[0].ID != "3" {
		t.Errorf("expected wholesale replacement, got %+v", got)
	}
}

func TestTodoWriteTool_AllCompletedClearsList(t *testing.T) {
	tool, store := newTodoTool()
	callTodo(t, tool, []types.TodoItem{
		{ID: "1", Content: "a", Status: types.TodoCompleted},
		{ID: "2", Content: "b", Status: types.TodoCompleted},
	})
	if got := store.GetState().Todos; len(got) != 0 {
		t.Errorf("oracle semantics: all-completed list must clear, got %+v", got)
	}
}

func TestTodoWriteTool_Validation(t *testing.T) {
	tool, store := newTodoTool()
	cases := []struct {
		name  string
		items []types.TodoItem
	}{
		{"missing id", []types.TodoItem{{Content: "x", Status: types.TodoPending}}},
		{"missing content", []types.TodoItem{{ID: "1", Status: types.TodoPending}}},
		{"bad status", []types.TodoItem{{ID: "1", Content: "x", Status: "done"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callTodo(t, tool, tc.items)
			if !result.IsError {
				t.Errorf("expected IsError for %s", tc.name)
			}
			if len(store.GetState().Todos) != 0 {
				t.Error("invalid input must not modify state")
			}
		})
	}
}

func TestTodoWriteTool_NilStore(t *testing.T) {
	tool := interact.NewTodoWriteTool(nil)
	result := callTodo(t, tool, []types.TodoItem{{ID: "1", Content: "x", Status: types.TodoPending}})
	if !result.IsError {
		t.Error("nil store must report an honest error")
	}
}

// ── AskUserQuestionTool ───────────────────────────────────────────────────────

func newQuestionTool() (tools.Tool, chan types.QuestionRequest, chan types.QuestionResponse) {
	ask := make(chan types.QuestionRequest, 1)
	resp := make(chan types.QuestionResponse, 1)
	return interact.NewAskUserQuestionTool(ask, resp), ask, resp
}

func TestAskUserQuestionTool_Metadata(t *testing.T) {
	tool, _, _ := newQuestionTool()
	if tool.Name() != "AskUserQuestion" {
		t.Errorf("expected AskUserQuestion, got %q", tool.Name())
	}
	if tool.IsConcurrencySafe(nil) {
		t.Error("AskUserQuestion must not be concurrency-safe")
	}
	if !tool.IsReadOnly(nil) {
		t.Error("AskUserQuestion should be read-only")
	}
	schema := tool.InputSchema()
	if _, ok := schema.Properties["question"]; !ok {
		t.Error("schema missing 'question'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "question" {
		t.Errorf("expected Required=[question], got %v", schema.Required)
	}
}

func TestAskUserQuestionTool_AnswerFlow(t *testing.T) {
	tool, ask, resp := newQuestionTool()
	in, _ := json.Marshal(interact.AskUserQuestionInput{Question: "which db?", Options: []string{"postgres", "sqlite"}})
	done := make(chan *tools.Result, 1)
	go func() {
		result, err := tool.Call(in, nil, nil)
		if err != nil {
			t.Errorf("hard error: %v", err)
		}
		done <- result
	}()
	req := <-ask
	if req.Question != "which db?" || len(req.Options) != 2 || req.ID == "" {
		t.Fatalf("unexpected request: %+v", req)
	}
	resp <- types.QuestionResponse{ID: req.ID, Answer: "postgres"}
	result := <-done
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	if !strings.Contains(result.Content.(string), "User answered: postgres") {
		t.Errorf("expected answer in result, got %v", result.Content)
	}
}

func TestAskUserQuestionTool_Dismissed(t *testing.T) {
	tool, ask, resp := newQuestionTool()
	in, _ := json.Marshal(interact.AskUserQuestionInput{Question: "q?"})
	done := make(chan *tools.Result, 1)
	go func() {
		result, _ := tool.Call(in, nil, nil)
		done <- result
	}()
	req := <-ask
	resp <- types.QuestionResponse{ID: req.ID, Dismissed: true}
	result := <-done
	if result.IsError {
		t.Error("dismissal is a normal outcome, not an error")
	}
	if !strings.Contains(result.Content.(string), "dismissed") {
		t.Errorf("expected dismissal text, got %v", result.Content)
	}
}

func TestAskUserQuestionTool_ContextCancel(t *testing.T) {
	tool, ask, _ := newQuestionTool()
	ctx, cancel := context.WithCancel(context.Background())
	in, _ := json.Marshal(interact.AskUserQuestionInput{Question: "q?"})
	done := make(chan *tools.Result, 1)
	go func() {
		result, _ := tool.Call(in, &tools.UseContext{Ctx: ctx}, nil)
		done <- result
	}()
	<-ask // question delivered; now cancel while waiting for the answer
	cancel()
	result := <-done
	if !result.IsError || !strings.Contains(result.Content.(string), "cancelled") {
		t.Errorf("expected cancellation error, got %+v", result)
	}
}

func TestAskUserQuestionTool_HeadlessNilChannels(t *testing.T) {
	tool := interact.NewAskUserQuestionTool(nil, nil)
	in, _ := json.Marshal(interact.AskUserQuestionInput{Question: "q?"})
	result, err := tool.Call(in, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content.(string), "non-interactive") {
		t.Errorf("headless must report honest error, got %+v", result)
	}
}

func TestAskUserQuestionTool_EmptyQuestion(t *testing.T) {
	tool, _, _ := newQuestionTool()
	in, _ := json.Marshal(interact.AskUserQuestionInput{})
	result, _ := tool.Call(in, nil, nil)
	if !result.IsError {
		t.Error("empty question must be rejected")
	}
}

// ── EnterPlanModeTool / ExitPlanModeTool（BF-09）─────────────────────────────

func TestPlanModeTools_Metadata(t *testing.T) {
	store := state.NewAppStateStore(state.AppState{})
	enter := interact.NewEnterPlanModeTool(store)
	exit := interact.NewExitPlanModeTool(store)
	if enter.Name() != "EnterPlanMode" || exit.Name() != "ExitPlanMode" {
		t.Errorf("names: %q / %q", enter.Name(), exit.Name())
	}
	if enter.IsConcurrencySafe(nil) || exit.IsConcurrencySafe(nil) {
		t.Error("plan mode tools must not be concurrency-safe")
	}
	if !enter.IsReadOnly(nil) || !exit.IsReadOnly(nil) {
		t.Error("plan mode tools should be read-only")
	}
	schema := exit.InputSchema()
	if _, ok := schema.Properties["approved"]; !ok {
		t.Error("schema missing 'approved'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "approved" {
		t.Errorf("expected Required=[approved], got %v", schema.Required)
	}
}

func TestPlanModeTools_SwitchLiveMode(t *testing.T) {
	store := state.NewAppStateStore(state.AppState{})
	enter := interact.NewEnterPlanModeTool(store)
	exit := interact.NewExitPlanModeTool(store)

	in, _ := json.Marshal(interact.EnterPlanModeInput{Plan: "step 1, step 2"})
	result, err := enter.Call(in, nil, nil)
	if err != nil || result.IsError {
		t.Fatalf("enter: err=%v result=%+v", err, result)
	}
	if got := store.GetState().ToolPermissionContext.Mode; got != types.PermissionModePlan {
		t.Errorf("expected plan mode, got %q", got)
	}
	if !strings.Contains(result.Content.(string), "step 1, step 2") {
		t.Error("plan summary should be echoed in the result")
	}

	in, _ = json.Marshal(interact.ExitPlanModeInput{Approved: true})
	result, err = exit.Call(in, nil, nil)
	if err != nil || result.IsError {
		t.Fatalf("exit: err=%v result=%+v", err, result)
	}
	if got := store.GetState().ToolPermissionContext.Mode; got != types.PermissionModeDefault {
		t.Errorf("expected default mode after exit, got %q", got)
	}
	if !strings.Contains(result.Content.(string), "approved") {
		t.Error("approval should be reflected in the result")
	}

	// Unapproved exit also returns to default but instructs not to execute.
	store.SetState(func(prev state.AppState) state.AppState {
		prev.ToolPermissionContext.Mode = types.PermissionModePlan
		return prev
	})
	in, _ = json.Marshal(interact.ExitPlanModeInput{Approved: false})
	result, _ = exit.Call(in, nil, nil)
	if !strings.Contains(result.Content.(string), "without approval") {
		t.Errorf("expected non-approval text, got %v", result.Content)
	}
}

func TestPlanModeTools_NilStore(t *testing.T) {
	for _, tool := range []tools.Tool{interact.NewEnterPlanModeTool(nil), interact.NewExitPlanModeTool(nil)} {
		in, _ := json.Marshal(map[string]any{"approved": true})
		result, err := tool.Call(in, nil, nil)
		if err != nil || !result.IsError {
			t.Errorf("%s: nil store must report honest error, got %+v", tool.Name(), result)
		}
	}
}

// ── EnterWorktreeTool ─────────────────────────────────────────────────────────

func TestEnterWorktreeTool_Name(t *testing.T) {
	if interact.EnterWorktreeTool.Name() != "EnterWorktree" {
		t.Errorf("expected EnterWorktree, got %q", interact.EnterWorktreeTool.Name())
	}
}

func TestEnterWorktreeTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.EnterWorktreeTool.IsConcurrencySafe(nil) {
		t.Error("EnterWorktreeTool must not be concurrency-safe")
	}
}

func TestEnterWorktreeTool_IsReadOnly_False(t *testing.T) {
	if interact.EnterWorktreeTool.IsReadOnly(nil) {
		t.Error("EnterWorktreeTool must not be read-only")
	}
}

func TestEnterWorktreeTool_UserFacingName_WithName(t *testing.T) {
	in, _ := json.Marshal(interact.EnterWorktreeInput{Name: "my-branch"})
	name := interact.EnterWorktreeTool.UserFacingName(in)
	if name != "EnterWorktree(my-branch)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestEnterWorktreeTool_UserFacingName_NoName(t *testing.T) {
	in, _ := json.Marshal(interact.EnterWorktreeInput{})
	name := interact.EnterWorktreeTool.UserFacingName(in)
	if name != "EnterWorktree" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestEnterWorktreeTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.EnterWorktreeInput{Name: "test"})
	result, err := interact.EnterWorktreeTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── ExitWorktreeTool ──────────────────────────────────────────────────────────

func TestExitWorktreeTool_Name(t *testing.T) {
	if interact.ExitWorktreeTool.Name() != "ExitWorktree" {
		t.Errorf("expected ExitWorktree, got %q", interact.ExitWorktreeTool.Name())
	}
}

func TestExitWorktreeTool_IsDestructive_True(t *testing.T) {
	if !interact.ExitWorktreeTool.IsDestructive(nil) {
		t.Error("ExitWorktreeTool must be destructive")
	}
}

func TestExitWorktreeTool_IsConcurrencySafe_False(t *testing.T) {
	if interact.ExitWorktreeTool.IsConcurrencySafe(nil) {
		t.Error("ExitWorktreeTool must not be concurrency-safe")
	}
}

func TestExitWorktreeTool_InputSchema(t *testing.T) {
	schema := interact.ExitWorktreeTool.InputSchema()
	if _, ok := schema.Properties["action"]; !ok {
		t.Error("schema missing 'action'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "action" {
		t.Errorf("expected Required=[action], got %v", schema.Required)
	}
}

func TestExitWorktreeTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(interact.ExitWorktreeInput{Action: "keep"})
	result, err := interact.ExitWorktreeTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for stub tool")
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestAllInteractTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = interact.NewTodoWriteTool(nil)
	var _ tools.Tool = interact.NewAskUserQuestionTool(nil, nil)
	var _ tools.Tool = interact.NewEnterPlanModeTool(nil)
	var _ tools.Tool = interact.NewExitPlanModeTool(nil)
	var _ tools.Tool = interact.EnterWorktreeTool
	var _ tools.Tool = interact.ExitWorktreeTool
}
