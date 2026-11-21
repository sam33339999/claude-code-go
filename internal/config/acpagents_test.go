package config_test

import (
	"path/filepath"
	"testing"

	"github.com/tunsuy/glaude/internal/config"
)

// TestLoad_ACPAgentsMergeFieldLevel verifies acpAgents entries merge field by
// field across layers: a higher layer overrides only the fields it sets, env
// keys are merged, and new names are added.
func TestLoad_ACPAgentsMergeFieldLevel(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	writeJSON(t, filepath.Join(home, ".glaude", "settings.json"), map[string]any{
		"acpAgents": map[string]any{
			"grok": map[string]any{
				"command": "/opt/grok",
				"env":     map[string]string{"A": "1", "B": "user"},
			},
		},
	})
	writeJSON(t, filepath.Join(project, ".glaude.local", "settings.json"), map[string]any{
		"acpAgents": map[string]any{
			"grok":   map[string]any{"model": "grok-x", "env": map[string]string{"B": "local"}},
			"custom": map[string]any{"command": "my-agent", "args": []string{"acp"}},
		},
	})

	ls, err := config.NewLoader(home, project).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	g := ls.Merged.ACPAgents["grok"]
	if g.Command != "/opt/grok" || g.Model != "grok-x" {
		t.Errorf("grok = %+v, want command from user and model from local", g)
	}
	if g.Env["A"] != "1" || g.Env["B"] != "local" {
		t.Errorf("grok env = %v, want A=1 B=local", g.Env)
	}
	c := ls.Merged.ACPAgents["custom"]
	if c.Command != "my-agent" || len(c.Args) != 1 || c.Args[0] != "acp" {
		t.Errorf("custom = %+v", c)
	}
}

// TestLoad_ACPAgentsPolicyDisables verifies a managed policy can disable an
// agent configured by the user.
func TestLoad_ACPAgentsPolicyDisables(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	writeJSON(t, filepath.Join(home, ".glaude", "settings.json"), map[string]any{
		"acpAgents": map[string]any{"grok": map[string]any{"command": "grok"}},
	})
	writeJSON(t, filepath.Join(home, ".glaude", "managed-settings.json"), map[string]any{
		"acpAgents": map[string]any{"grok": map[string]any{"disabled": true}},
	})
	ls, err := config.NewLoader(home, project).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	g := ls.Merged.ACPAgents["grok"]
	if g.Disabled == nil || !*g.Disabled || g.Command != "grok" {
		t.Errorf("grok = %+v, want disabled with command kept", g)
	}
}

func TestMergeACPAgentConfig_DoesNotAliasInputs(t *testing.T) {
	base := config.ACPAgentConfig{Env: map[string]string{"A": "1"}}
	over := config.ACPAgentConfig{Env: map[string]string{"B": "2"}, Args: []string{"x"}}
	got := config.MergeACPAgentConfig(base, over)
	got.Env["C"] = "3"
	got.Args[0] = "y"
	if _, ok := base.Env["C"]; ok {
		t.Error("merged env aliases base env")
	}
	if over.Args[0] != "x" {
		t.Error("merged args alias override args")
	}
}
