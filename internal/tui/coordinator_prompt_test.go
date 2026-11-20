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
	m.memdirPrompt = "GLAUDE.md content"
	fe := m.queryEngine.(*fakeEngine)

	cmd := startQueryCmd(&m, "hello", true)
	if cmd == nil {
		t.Fatal("startQueryCmd returned nil")
	}
	_ = cmd() // dispatches the query against the fake engine

	parts := fe.lastParams.SystemPrompt.Parts
	if len(parts) != 2 {
		t.Fatalf("system parts = %d, want 2 (GLAUDE.md + coordinator prompt)", len(parts))
	}
	if parts[0].Text != "GLAUDE.md content" {
		t.Errorf("part 0 = %q, want GLAUDE.md content", parts[0].Text)
	}
	if !strings.Contains(parts[1].Text, "coordinator") {
		t.Error("part 1 must be the coordinator role prompt")
	}
	// One breakpoint at the end of the stable prefix; GLAUDE.md stays unmarked
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
	m.memdirPrompt = "GLAUDE.md content"
	fe := m.queryEngine.(*fakeEngine)

	cmd := startQueryCmd(&m, "hello", true)
	if cmd == nil {
		t.Fatal("startQueryCmd returned nil")
	}
	_ = cmd()

	parts := fe.lastParams.SystemPrompt.Parts
	if len(parts) != 1 {
		t.Fatalf("system parts = %d, want 1 (GLAUDE.md only)", len(parts))
	}
	if parts[0].CacheControl != "ephemeral" {
		t.Errorf("single stable part marker = %q, want ephemeral", parts[0].CacheControl)
	}
}

func TestStartQueryCmd_WorkingDirFromAppState(t *testing.T) {
	m := newTestModel() // appState WorkingDir = "/tmp"
	fe := m.queryEngine.(*fakeEngine)

	cmd := startQueryCmd(&m, "hello", true)
	if cmd == nil {
		t.Fatal("startQueryCmd returned nil")
	}
	_ = cmd()

	uctx := fe.lastParams.ToolUseContext
	if uctx == nil || uctx.WorkingDir != "/tmp" {
		t.Errorf("ToolUseContext.WorkingDir = %v, want /tmp (from AppState)", uctx)
	}
}
