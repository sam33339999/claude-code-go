package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAgentDef writes a minimal JSON agent definition into dir.
func writeAgentDef(t *testing.T, dir, name, model string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"` + name + `","model":"` + model + `","description":"d","system_prompt":"p"}`
	if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFormatAgentsList_ShadowingAndGroups(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeAgentDef(t, filepath.Join(home, ".claude", "agents"), "alpha", "sonnet")
	writeAgentDef(t, filepath.Join(home, ".claude", "agents"), "beta", "")
	writeAgentDef(t, filepath.Join(cwd, ".claude", "agents"), "alpha", "opus")
	writeAgentDef(t, filepath.Join(cwd, ".claude", "agents"), "gamma", "haiku")

	out, err := formatAgentsList(home, cwd, "")
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	// active = beta(1) + alpha,gamma(2) + 5 builtins = 8
	if !strings.Contains(out, "8 active agents") {
		t.Errorf("expected 8 active agents, got:\n%s", out)
	}
	if !strings.Contains(out, "(shadowed by project) alpha · sonnet") {
		t.Errorf("expected user alpha shadowed by project, got:\n%s", out)
	}
	if !strings.Contains(out, "User agents:") || !strings.Contains(out, "Project agents:") || !strings.Contains(out, "Built-in agents:") {
		t.Errorf("expected all three groups, got:\n%s", out)
	}
	if !strings.Contains(out, "alpha · opus") || !strings.Contains(out, "gamma · haiku") {
		t.Errorf("expected project models, got:\n%s", out)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("expected non-shadowed user beta, got:\n%s", out)
	}
	if !strings.Contains(out, "worker") {
		t.Errorf("expected builtin worker, got:\n%s", out)
	}
}

func TestFormatAgentsList_SourcesFilter(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	writeAgentDef(t, filepath.Join(home, ".claude", "agents"), "useronly", "")
	writeAgentDef(t, filepath.Join(cwd, ".claude", "agents"), "projonly", "")

	out, err := formatAgentsList(home, cwd, "project")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "User agents:") || strings.Contains(out, "useronly") {
		t.Errorf("project-only must exclude user group, got:\n%s", out)
	}
	if !strings.Contains(out, "projonly") {
		t.Errorf("expected project agent, got:\n%s", out)
	}

	out, err = formatAgentsList(home, cwd, "user")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Project agents:") || strings.Contains(out, "projonly") {
		t.Errorf("user-only must exclude project group, got:\n%s", out)
	}

	// local is accepted (oracle parity) but contributes no custom agents.
	out, err = formatAgentsList(home, cwd, "local")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "useronly") || strings.Contains(out, "projonly") {
		t.Errorf("local-only must exclude custom groups, got:\n%s", out)
	}
	if !strings.Contains(out, "Built-in agents:") {
		t.Errorf("builtins always list, got:\n%s", out)
	}

	if _, err := formatAgentsList(home, cwd, "bogus"); err == nil {
		t.Error("expected error for unknown setting source")
	}
}

func TestFormatAgentsList_BuiltinsOnly(t *testing.T) {
	out, err := formatAgentsList(t.TempDir(), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "5 active agents") || !strings.Contains(out, "Built-in agents:") {
		t.Errorf("expected 5 builtin agents, got:\n%s", out)
	}
	if strings.Contains(out, "User agents:") || strings.Contains(out, "Project agents:") {
		t.Errorf("empty dirs must not render custom groups, got:\n%s", out)
	}
}

func TestAgentsCmd_OracleShape(t *testing.T) {
	cmd := newAgentsCmd()
	if cmd.Use != "agents" {
		t.Errorf("Use: want agents, got %q", cmd.Use)
	}
	if cmd.HasSubCommands() {
		t.Error("oracle shape: agents must have NO subcommands (list/add/remove stubs removed)")
	}
	if cmd.Flags().Lookup("setting-sources") == nil {
		t.Error("expected --setting-sources flag (oracle parity)")
	}
}
