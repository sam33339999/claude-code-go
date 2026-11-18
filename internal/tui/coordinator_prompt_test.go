package tui

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Coordinator system-prompt injection (cache-stable prefix)
// ─────────────────────────────────────────────────────────────────────────────

func TestStartQueryCmd_CoordinatorPromptInjected(t *testing.T) {
	m := newTestModel()
	m.coordinatorMode = true
	m.memdirPrompt = "CLAUDE.md content"
	fe := m.queryEngine.(*fakeEngine)

	cmd := startQueryCmd(&m, "hello")
	if cmd == nil {
		t.Fatal("startQueryCmd returned nil")
	}
	_ = cmd() // dispatches the query against the fake engine

	parts := fe.lastParams.SystemPrompt.Parts
	if len(parts) != 2 {
		t.Fatalf("system parts = %d, want 2 (CLAUDE.md + coordinator prompt)", len(parts))
	}
	if parts[0].Text != "CLAUDE.md content" {
		t.Errorf("part 0 = %q, want CLAUDE.md content", parts[0].Text)
	}
	if !strings.Contains(parts[1].Text, "coordinator") {
		t.Error("part 1 must be the coordinator role prompt")
	}
	// One breakpoint at the end of the stable prefix; CLAUDE.md stays unmarked
	// so the whole stable section is cached as a single prefix.
	if parts[0].CacheControl != "" {
		t.Error("part 0 must not carry a breakpoint (single marker at the stable tail)")
	}
	if parts[1].CacheControl != "ephemeral" {
		t.Errorf("last stable part marker = %q, want ephemeral", parts[1].CacheControl)
	}
}

func TestStartQueryCmd_NoCoordinatorMode(t *testing.T) {
	m := newTestModel()
	m.coordinatorMode = false
	m.memdirPrompt = "CLAUDE.md content"
	fe := m.queryEngine.(*fakeEngine)

	cmd := startQueryCmd(&m, "hello")
	if cmd == nil {
		t.Fatal("startQueryCmd returned nil")
	}
	_ = cmd()

	parts := fe.lastParams.SystemPrompt.Parts
	if len(parts) != 1 {
		t.Fatalf("system parts = %d, want 1 (CLAUDE.md only)", len(parts))
	}
	if parts[0].CacheControl != "ephemeral" {
		t.Errorf("single stable part marker = %q, want ephemeral", parts[0].CacheControl)
	}
}
