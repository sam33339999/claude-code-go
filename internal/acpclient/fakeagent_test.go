package acpclient

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/tunsuy/glaude/internal/acpclient/acptest"
)

// pipeStarter is a StartFunc that runs a fresh acptest.Agent in-process per
// start, recording each agent for inspection.
type pipeStarter struct {
	mu     sync.Mutex
	agents []*acptest.Agent
	starts int
	fail   error
}

func (ps *pipeStarter) start(ctx context.Context, _ AgentSpec, _ string) (*Proc, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.starts++
	if ps.fail != nil {
		return nil, ps.fail
	}
	a, stdin, stdout, done, kill := acptest.Pipe(ctx)
	ps.agents = append(ps.agents, a)
	return NewProc(stdin, stdout, done, kill), nil
}

func (ps *pipeStarter) agent(i int) *acptest.Agent {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.agents[i]
}

// TestMain doubles as the fake agent binary for ExecStart tests.
func TestMain(m *testing.M) {
	if os.Getenv("GLAUDE_ACP_FAKE_AGENT") == "1" {
		acptest.New().Serve(os.Stdin, os.Stdout, func() { os.Exit(3) })
		os.Exit(0)
	}
	os.Exit(m.Run())
}
