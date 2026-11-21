package acpclient

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"
)

// closeGrace is how long Close waits for an agent to exit after its stdin is
// closed before killing it.
const closeGrace = 3 * time.Second

// Proc is a running agent process (or an in-memory stand-in in tests).
type Proc struct {
	// Stdin carries client→agent JSON-RPC; Stdout agent→client.
	Stdin  io.WriteCloser
	Stdout io.Reader

	done    chan struct{}
	kill    func()
	stderr  *tailBuffer
	closeMu sync.Once
}

// NewProc wraps transport streams. done must close when the agent exits;
// kill forcibly stops it. Either may be nil for in-memory transports.
func NewProc(stdin io.WriteCloser, stdout io.Reader, done chan struct{}, kill func()) *Proc {
	if done == nil {
		done = make(chan struct{})
	}
	if kill == nil {
		kill = func() {}
	}
	return &Proc{Stdin: stdin, Stdout: stdout, done: done, kill: kill, stderr: newTailBuffer(4096)}
}

// Done closes when the process has exited.
func (p *Proc) Done() <-chan struct{} { return p.done }

// StderrTail returns the last bytes the agent wrote to stderr.
func (p *Proc) StderrTail() string { return p.stderr.String() }

// Close closes stdin, waits up to closeGrace for exit, then kills.
func (p *Proc) Close() error {
	p.closeMu.Do(func() {
		_ = p.Stdin.Close()
		select {
		case <-p.done:
		case <-time.After(closeGrace):
			p.kill()
		}
	})
	return nil
}

// StartFunc launches an agent for cwd. The process must stop when ctx ends.
type StartFunc func(ctx context.Context, spec AgentSpec, cwd string) (*Proc, error)

// ExecStart starts spec.Command as a child process speaking ACP on stdio.
// stderr is captured (last 4 KiB) rather than inherited, so agent output
// never reaches glaude's terminal.
func ExecStart(ctx context.Context, spec AgentSpec, cwd string) (*Proc, error) {
	path, err := exec.LookPath(spec.Command)
	if err != nil {
		return nil, fmt.Errorf("agent command %q not found in PATH: %w", spec.Command, err)
	}
	cmd := exec.CommandContext(ctx, path, spec.Args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), envList(spec.Env)...)
	cmd.WaitDelay = closeGrace

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	tail := newTailBuffer(4096)
	cmd.Stderr = tail
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", spec.Command, err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	p := NewProc(stdin, stdout, done, func() { _ = cmd.Process.Kill() })
	p.stderr = tail
	return p, nil
}

func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max} }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = append([]byte(nil), t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
