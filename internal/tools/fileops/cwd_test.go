package fileops_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/fileops"
)

// TestFileOps_WorkingDir verifies relative paths resolve against the session
// working directory carried by UseContext, not the process CWD.
func TestFileOps_WorkingDir(t *testing.T) {
	dir := t.TempDir()
	uctx := &tools.UseContext{Ctx: context.Background(), WorkingDir: dir}

	// Write a relative path.
	wresult, err := fileops.FileWriteTool.Call(tools.Input(`{"file_path":"sub/note.txt","content":"hello cwd"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Write Call: %v", err)
	}
	if wresult.IsError {
		t.Fatalf("Write error result: %v", wresult.Content)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "note.txt")); err != nil {
		t.Fatalf("file must land under WorkingDir: %v", err)
	}

	// Read the relative path back.
	rresult, err := fileops.FileReadTool.Call(tools.Input(`{"file_path":"sub/note.txt"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Read Call: %v", err)
	}
	if rresult.IsError {
		t.Fatalf("Read error result: %v", rresult.Content)
	}
	if !strings.Contains(resultText(rresult.Content), "hello cwd") {
		t.Errorf("Read content = %v, want hello cwd", rresult.Content)
	}

	// Glob with no explicit path searches the WorkingDir (** glob semantics).
	gresult, err := fileops.GlobTool.Call(tools.Input(`{"pattern":"**/*.txt"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Glob Call: %v", err)
	}
	if gresult.IsError {
		t.Fatalf("Glob error result: %v", gresult.Content)
	}
	if !strings.Contains(resultText(gresult.Content), "note.txt") {
		t.Errorf("Glob must find note.txt under WorkingDir: %v", gresult.Content)
	}

	// Grep with no explicit path searches the WorkingDir.
	grresult, err := fileops.GrepTool.Call(tools.Input(`{"pattern":"hello cwd"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Grep Call: %v", err)
	}
	if grresult.IsError {
		t.Fatalf("Grep error result: %v", grresult.Content)
	}
	if !strings.Contains(resultText(grresult.Content), "note.txt") {
		t.Errorf("Grep must match note.txt under WorkingDir: %v", grresult.Content)
	}
}

// resultText normalises a tool result content for substring assertions.
func resultText(content any) string {
	if s, ok := content.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", content)
}
