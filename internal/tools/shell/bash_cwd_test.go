package shell_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/internal/tools/shell"
)

// TestBashTool_WorkingDir verifies commands run against the session working
// directory carried by UseContext (delegated agents / worktree sessions),
// not the process CWD.
func TestBashTool_WorkingDir(t *testing.T) {
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	uctx := &tools.UseContext{Ctx: context.Background(), WorkingDir: dir}

	result, err := shell.BashTool.Call(tools.Input(`{"command":"pwd"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
	out, ok := result.Content.(shell.BashOutput)
	if !ok {
		t.Fatalf("content type %T, want shell.BashOutput", result.Content)
	}
	// pwd prints the logical path, which may still go through a symlink
	// (macOS: /var -> /private/var), so resolve it before comparing.
	got, err := filepath.EvalSymlinks(strings.TrimSpace(out.Stdout))
	if err != nil {
		t.Fatalf("resolve pwd output %q: %v", out.Stdout, err)
	}
	if got != resolved {
		t.Errorf("pwd output = %q (resolved %q), want %q (session WorkingDir)", out.Stdout, got, resolved)
	}
}

// TestBashTool_NoWorkingDirFallsBackToProcessCWD verifies the empty
// WorkingDir keeps the legacy behaviour (inherit process CWD).
func TestBashTool_NoWorkingDirFallsBackToProcessCWD(t *testing.T) {
	uctx := &tools.UseContext{Ctx: context.Background()}
	result, err := shell.BashTool.Call(tools.Input(`{"command":"pwd"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}
}
