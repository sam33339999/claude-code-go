package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/glaude/internal/acpclient/acptest"
	"github.com/tunsuy/glaude/internal/config"

	"github.com/tunsuy/glaude/internal/agenttype"
)

// TestMain sets up the test environment.
// We redirect HOME and XDG_CONFIG_HOME to a temp dir so no real
// keychain / token files are touched during testing.
func TestMain(m *testing.M) {
	// The test binary doubles as a scripted ACP agent for the external-agent
	// integration test (settings acpAgents → os.Args[0]).
	if os.Getenv("GLAUDE_ACP_FAKE_AGENT") == "1" {
		acptest.New().Serve(os.Stdin, os.Stdout, func() { os.Exit(3) })
		os.Exit(0)
	}
	// Use a fresh temp dir as the home directory for all tests so that
	// OAuth token stores and config files are isolated.
	tmpHome, err := os.MkdirTemp("", "bootstrap-test-home-*")
	if err != nil {
		panic("bootstrap_test: create tmp home: " + err.Error())
	}
	defer os.RemoveAll(tmpHome)

	os.Setenv("HOME", tmpHome)
	os.Setenv("XDG_CONFIG_HOME", tmpHome)
	// Ensure no real API key bleeds into tests.
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("ANTHROPIC_MODEL")
	os.Unsetenv("CLAUDE_MODEL")

	os.Exit(m.Run())
}

// TestRunAuthStatus_NotAuthenticated_ReturnsError verifies that
// runAuthStatus returns a non-nil error when there is no OAuth token
// and no ANTHROPIC_API_KEY in the environment.
func TestRunAuthStatus_NotAuthenticated_ReturnsError(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	err := runAuthStatus()
	if err == nil {
		t.Fatal("expected non-nil error when not authenticated, got nil")
	}
}

// TestBuildContainer_MissingConfig_Succeeds verifies that BuildContainer
// succeeds even when there are no config files or API key present.
// An empty API key is acceptable at construction time; auth errors surface
// only on the first real API call.
func TestBuildContainer_MissingConfig_Succeeds(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")

	opts := ContainerOptions{
		HomeDir:    t.TempDir(),
		WorkingDir: t.TempDir(),
	}

	container, err := BuildContainer(opts)
	if err != nil {
		t.Fatalf("BuildContainer returned unexpected error: %v", err)
	}
	if container == nil {
		t.Fatal("BuildContainer returned nil container, expected non-nil")
	}
}

// TestResolveModel_Override verifies that passing a non-empty override
// returns that override, ignoring any settings (including nil settings).
func TestResolveModel_Override(t *testing.T) {
	const want = "claude-3-haiku"
	got := resolveModel(nil, want)
	if got != want {
		t.Errorf("resolveModel(nil, %q) = %q, want %q", want, got, want)
	}
}

// TestResolveModel_OpenAIProviderUsesOpenAIDefault verifies that an
// OpenAI-compatible provider does not fall back to an Anthropic model ID.
func TestResolveModel_OpenAIProviderUsesOpenAIDefault(t *testing.T) {
	settings := &config.LayeredSettings{Merged: &config.SettingsJson{Provider: "openai"}}
	if got := resolveModel(settings, ""); got != defaultOpenAIModel {
		t.Errorf("resolveModel(openai) = %q, want %q", got, defaultOpenAIModel)
	}
}

// TestResolveModel_AnthropicProviderUsesAnthropicDefault verifies the
// anthropic default is kept for direct providers and empty settings.
func TestResolveModel_AnthropicProviderUsesAnthropicDefault(t *testing.T) {
	for name, settings := range map[string]*config.LayeredSettings{
		"nil settings": nil,
		"direct":       {Merged: &config.SettingsJson{Provider: "direct"}},
		"empty":        {Merged: &config.SettingsJson{}},
	} {
		if got := resolveModel(settings, ""); got != defaultModel {
			t.Errorf("%s: resolveModel() = %q, want %q", name, got, defaultModel)
		}
	}
}

// TestResolveModel_SettingsModelWinsOverProviderDefault verifies an explicit
// model in settings beats either provider default.
func TestResolveModel_SettingsModelWinsOverProviderDefault(t *testing.T) {
	settings := &config.LayeredSettings{Merged: &config.SettingsJson{
		Provider: "openai",
		Model:    "qwen3.7-max",
	}}
	if got := resolveModel(settings, ""); got != "qwen3.7-max" {
		t.Errorf("resolveModel() = %q, want qwen3.7-max", got)
	}
}

// TestResolveProviderName defaults to anthropic when unset.
func TestResolveProviderName(t *testing.T) {
	if got := resolveProviderName(nil); got != "anthropic" {
		t.Errorf("resolveProviderName(nil) = %q, want anthropic", got)
	}
	settings := &config.LayeredSettings{Merged: &config.SettingsJson{Provider: "openai"}}
	if got := resolveProviderName(settings); got != "openai" {
		t.Errorf("resolveProviderName() = %q, want openai", got)
	}
}

func TestLoadCustomAgentProfiles_UserAndProjectOverride(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()

	userAgents := filepath.Join(home, ".glaude", "agents")
	projectAgents := filepath.Join(work, ".glaude", "agents")
	if err := os.MkdirAll(userAgents, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectAgents, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(userAgents, "reviewer.json"), []byte(`{
		"name": "reviewer",
		"display_name": "Reviewer",
		"description": "user reviewer",
		"system_prompt": "user prompt"
	}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectAgents, "reviewer.json"), []byte(`{
		"name": "reviewer",
		"display_name": "Reviewer",
		"description": "project reviewer",
		"system_prompt": "project prompt"
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	reg := agenttype.NewRegistry()
	loadCustomAgentProfiles(reg, home, work)

	profile, ok := reg.Get(agenttype.AgentType("reviewer"))
	if !ok {
		t.Fatal("expected reviewer profile")
	}
	if profile.Description != "project reviewer" {
		t.Errorf("project profile should override user profile, got %q", profile.Description)
	}
	if profile.SystemPrompt != "project prompt" {
		t.Errorf("SystemPrompt = %q", profile.SystemPrompt)
	}
}

// TestCollectHeadlessPrompt_FromArgs verifies that positional CLI arguments
// are joined with a single space and returned without error.
func TestCollectHeadlessPrompt_FromArgs(t *testing.T) {
	args := []string{"hello", "world"}
	got, err := collectHeadlessPrompt(args)
	if err != nil {
		t.Fatalf("collectHeadlessPrompt returned unexpected error: %v", err)
	}
	const want = "hello world"
	if got != want {
		t.Errorf("collectHeadlessPrompt(%v) = %q, want %q", args, got, want)
	}
}
