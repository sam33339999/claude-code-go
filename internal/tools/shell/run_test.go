package shell_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/glaude/internal/tools/shell"
)

func TestRun_Stdout(t *testing.T) {
	res, err := shell.Run(context.Background(), "", "echo hi", time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stdout != "hi\n" || res.ExitCode != 0 {
		t.Fatalf("got stdout=%q exit=%d", res.Stdout, res.ExitCode)
	}
}

func TestRun_ExitCodeAndStderr(t *testing.T) {
	res, err := shell.Run(context.Background(), "", "echo e >&2; exit 3", time.Second)
	if err != nil {
		t.Fatalf("non-zero exit must not be an error: %v", err)
	}
	if res.Stderr != "e\n" || res.ExitCode != 3 {
		t.Fatalf("got stderr=%q exit=%d", res.Stderr, res.ExitCode)
	}
}

func TestRun_Dir(t *testing.T) {
	dir := t.TempDir()
	res, err := shell.Run(context.Background(), dir, "pwd", time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	got, _ := filepath.EvalSymlinks(strings.TrimSpace(res.Stdout))
	if got != want {
		t.Fatalf("pwd = %q, want %q", got, want)
	}
}

func TestRun_Timeout(t *testing.T) {
	res, _ := shell.Run(context.Background(), "", "sleep 5", 50*time.Millisecond)
	if !res.TimedOut || res.Canceled || res.ExitCode != -1 {
		t.Fatalf("got %+v, want TimedOut, exit -1", res)
	}
}

func TestRun_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	res, _ := shell.Run(ctx, "", "sleep 5", 5*time.Second)
	if !res.Canceled || res.TimedOut {
		t.Fatalf("got %+v, want Canceled", res)
	}
}
