package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// runWaitDelay bounds how long Run waits for a killed command's output pipes
// to close (background children may keep them open).
const runWaitDelay = 500 * time.Millisecond

// RunResult is the outcome of one shell command.
type RunResult struct {
	// Stdout is the command's standard output.
	Stdout string
	// Stderr is the command's standard error.
	Stderr string
	// ExitCode is the exit status; -1 when the process did not exit normally.
	ExitCode int
	// TimedOut is true when the command was killed for exceeding the timeout.
	TimedOut bool
	// Canceled is true when the parent context was canceled (not a timeout).
	Canceled bool
}

// Run executes command with `bash -c` in dir (empty = process CWD), bounded
// by timeout. A non-zero exit is not an error; err is only returned when the
// command could not be started.
func Run(ctx context.Context, dir, command string, timeout time.Duration) (RunResult, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)
	cmd.Dir = dir
	cmd.WaitDelay = runWaitDelay
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := RunResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return res, nil
	}

	switch {
	case ctx.Err() != nil:
		res.Canceled = true
		res.ExitCode = -1
		return res, nil
	case errors.Is(timeoutCtx.Err(), context.DeadlineExceeded):
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	res.ExitCode = -1
	return res, fmt.Errorf("run command: %w", err)
}
