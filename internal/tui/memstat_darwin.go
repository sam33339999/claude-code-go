//go:build darwin

package tui

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// psFailures counts consecutive ps failures; after psMaxFailures (e.g. a
// sandbox that forbids exec) stop spawning and use the Go runtime figure.
var psFailures atomic.Int32

const psMaxFailures = 3

// processRSS returns this process's resident set size. macOS exposes it only
// through task_info (cgo) or ps(1); glaude is cgo-free, so ask ps. Callers run
// this inside a tea.Cmd, never on the Update loop.
func processRSS() (uint64, bool) {
	if psFailures.Load() >= psMaxFailures {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	// Absolute path: don't depend on (or be hijacked through) $PATH.
	out, err := exec.CommandContext(ctx, "/bin/ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if err != nil {
		psFailures.Add(1)
		return 0, false
	}
	kb, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || kb == 0 {
		psFailures.Add(1)
		return 0, false
	}
	psFailures.Store(0)
	return kb * 1024, true
}
