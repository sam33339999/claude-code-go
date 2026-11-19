package interact_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/tunsuy/glaude/internal/state"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

func todoInput(t *testing.T, items ...types.TodoItem) tools.Input {
	t.Helper()
	b, err := json.Marshal(map[string]any{"todos": items})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func appendCtx() *tools.UseContext { return &tools.UseContext{TodoAppendOnly: true} }

func seedTodos(store *state.AppStateStore, items ...types.TodoItem) {
	store.SetState(func(s state.AppState) state.AppState { s.Todos = items; return s })
}

func TestTodoWrite_AppendOnly(t *testing.T) {
	tool, store := newTodoTool()
	seedTodos(store, types.TodoItem{ID: "a", Content: "main work", Status: types.TodoInProgress})
	res, err := tool.Call(todoInput(t, types.TodoItem{ID: "x", Content: "check retry", Status: types.TodoPending}), appendCtx(), nil)
	if err != nil || res.IsError {
		t.Fatalf("call: %v %+v", err, res)
	}
	got := store.GetState().Todos
	if len(got) != 2 || got[0].ID != "a" || got[0].Status != types.TodoInProgress || got[1].Content != "[btw] check retry" || got[1].Status != types.TodoPending {
		t.Errorf("todos = %+v", got)
	}
}

func TestTodoWrite_AppendSkipsDuplicates(t *testing.T) {
	tool, store := newTodoTool()
	seedTodos(store, types.TodoItem{ID: "b1", Content: "[btw] check retry", Status: types.TodoPending})
	_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "x", Content: "check retry", Status: types.TodoPending}), appendCtx(), nil)
	if got := store.GetState().Todos; len(got) != 1 {
		t.Errorf("duplicate appended: %+v", got)
	}
}

func TestTodoWrite_AppendIDCollision(t *testing.T) {
	tool, store := newTodoTool()
	seedTodos(store, types.TodoItem{ID: "1", Content: "main", Status: types.TodoPending})
	_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "1", Content: "side", Status: types.TodoPending}), appendCtx(), nil)
	got := store.GetState().Todos
	if len(got) != 2 || got[0].Content != "main" || got[1].ID == "1" {
		t.Errorf("todos = %+v (existing item must not be overwritten; new item needs a fresh ID)", got)
	}
}

func TestTodoWrite_AppendCompletedDoesNotClear(t *testing.T) {
	tool, store := newTodoTool()
	seedTodos(store, types.TodoItem{ID: "a", Content: "main", Status: types.TodoPending})
	_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "x", Content: "done thing", Status: types.TodoCompleted}), appendCtx(), nil)
	if got := store.GetState().Todos; len(got) != 2 {
		t.Errorf("all-completed append must not clear the list: %+v", got)
	}
}

func TestTodoWrite_AppendConcurrentWithReplace(t *testing.T) {
	tool, store := newTodoTool()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 50 {
			_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "m", Content: fmt.Sprintf("main %d", i), Status: types.TodoPending}), &tools.UseContext{}, nil)
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 50 {
			_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "b", Content: fmt.Sprintf("side %d", i), Status: types.TodoPending}), appendCtx(), nil)
		}
	}()
	wg.Wait()
	_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "b", Content: "final side", Status: types.TodoPending}), appendCtx(), nil)
	found := false
	for _, it := range store.GetState().Todos {
		if strings.Contains(it.Content, "final side") {
			found = true
		}
	}
	if !found {
		t.Error("last appended item lost")
	}
}

func TestTodoWrite_ReplaceModeUnchanged(t *testing.T) {
	tool, store := newTodoTool()
	seedTodos(store, types.TodoItem{ID: "a", Content: "old", Status: types.TodoPending})
	_, _ = tool.Call(todoInput(t, types.TodoItem{ID: "n", Content: "new", Status: types.TodoPending}), &tools.UseContext{}, nil)
	if got := store.GetState().Todos; len(got) != 1 || got[0].Content != "new" {
		t.Errorf("replace mode changed: %+v", got)
	}
}
