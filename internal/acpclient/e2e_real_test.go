//go:build acpe2e

// Real-agent end-to-end test (opt-in, spends real tokens):
//
//	go test -tags acpe2e -run TestRealAgents -v ./internal/acpclient/
//
// Requires the agent binaries in PATH and signed in. GLAUDE_ACP_E2E_AGENTS
// selects agents (comma-separated, default "grok,opencode").
package acpclient

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRealAgents(t *testing.T) {
	names := []string{"grok", "opencode"}
	if v := os.Getenv("GLAUDE_ACP_E2E_AGENTS"); v != "" {
		names = strings.Split(v, ",")
	}
	for _, name := range names {
		name := strings.TrimSpace(name)
		t.Run(name, func(t *testing.T) {
			spec, ok := Builtins()[name]
			if !ok {
				t.Skipf("no builtin %q", name)
			}
			if _, err := exec.LookPath(spec.Command); err != nil {
				t.Skipf("%s not installed", spec.Command)
			}
			runRealAgent(t, name)
		})
	}
}

func runRealAgent(t *testing.T, name string) {
	dir := t.TempDir()
	var mu sync.Mutex
	var queries []PermissionQuery
	perm := func(_ context.Context, q PermissionQuery) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		queries = append(queries, q)
		// Allow only the one harmless command; deny anything else.
		return q.ToolName == "Bash" && strings.Contains(string(q.Input), "e2e-marker"), nil
	}
	m := NewManager(Builtins(), Options{Permission: perm, Logf: t.Logf})
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var events int
	onEvent := func(e Event) {
		events++
		if e.Kind == EventToolCall || e.Kind == EventNotice {
			t.Logf("event %d: %s", e.Kind, e.Title+e.Text)
		}
	}
	start := time.Now()
	r1, err := m.Prompt(ctx, name, dir, "Run exactly this shell command with your terminal tool: echo e2e-marker > marker.txt\nDo not use any other tool. Then reply with the single word DONE.", onEvent)
	if err != nil {
		t.Fatalf("prompt 1: %v", err)
	}
	t.Logf("prompt 1 (%s, %d events): %q", time.Since(start).Round(time.Millisecond), events, r1.Text)
	if !r1.NewSession {
		t.Error("first prompt should start a session")
	}
	b, err := os.ReadFile(filepath.Join(dir, "marker.txt"))
	if err != nil || !strings.Contains(string(b), "e2e-marker") {
		t.Fatalf("marker.txt = %q, %v (command did not run)", b, err)
	}
	mu.Lock()
	gotBash := false
	for _, q := range queries {
		t.Logf("permission query: %s %s", q.ToolName, q.Input)
		if q.ToolName == "Bash" {
			gotBash = true
		}
	}
	mu.Unlock()
	if !gotBash {
		t.Fatal("no Bash permission request reached glaude (agent bypassed the client)")
	}

	start = time.Now()
	r2, err := m.Prompt(ctx, name, dir, "Without using any tool: what is the name of the file you just created? Reply with only the file name.", onEvent)
	if err != nil {
		t.Fatalf("prompt 2: %v", err)
	}
	t.Logf("prompt 2 (%s): %q", time.Since(start).Round(time.Millisecond), r2.Text)
	if r2.NewSession {
		t.Error("second prompt started a new session; want the same long-lived one")
	}
	if !strings.Contains(r2.Text, "marker.txt") {
		t.Errorf("agent did not remember the previous turn: %q", r2.Text)
	}
}
