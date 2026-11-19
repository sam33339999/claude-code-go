package commands

import (
	"context"
	"errors"
)

// Setup wire formats (the values written to settings.json "provider").
const (
	SetupFormatAnthropic = "direct"
	SetupFormatOpenAI    = "openai"
)

// SetupDefaults are the current global settings the /setup wizard starts from.
type SetupDefaults struct {
	// Format, BaseURL and Model come from ~/.glaude/settings.json only.
	// Format is always SetupFormatAnthropic or SetupFormatOpenAI (an unset
	// provider is reported as SetupFormatAnthropic).
	Format  string
	BaseURL string
	Model   string
	// KeySources says, per format, where a reusable key would come from:
	// "settings" (global settings.json, only for the current Format),
	// "env:<VAR>", or absent/"" when the user must type a key.
	KeySources map[string]string
	// Tiers maps lite/pro/max to their current binding: "main:<model>",
	// "<provider>:<model>", or "" when unbound.
	Tiers map[string]string
}

// SetupInput is the endpoint the user entered. An empty APIKey means "reuse
// the existing key" (the backend resolves it; the TUI never holds it).
type SetupInput struct {
	Format  string
	BaseURL string
	APIKey  string
}

// SetupModel is one model offered by the endpoint.
type SetupModel struct {
	ID string
	// ContextWindow is the reported context length (0 = unknown).
	ContextWindow int
}

// SetupPlan is what Apply writes.
type SetupPlan struct {
	Input     SetupInput
	MainModel string
	// Tiers covers only the lite/pro/max entries the wizard controls: a model
	// ID binds main:<model>, "" unbinds. Tiers absent from the map are left alone.
	Tiers map[string]string
	// ContextWindows holds reported context lengths by model ID.
	ContextWindows map[string]int
}

// SetupReport describes what Apply wrote.
type SetupReport struct {
	SettingsPath    string
	ConfigPath      string
	SettingsWritten bool
	ConfigWritten   bool
	Warnings        []string
}

// SetupBackend is what the /setup wizard needs from the outside world.
// bootstrap implements it; the TUI calls ListModels and Apply from tea.Cmds.
type SetupBackend interface {
	// Current returns the starting values from the global settings.
	Current() SetupDefaults
	// ListModels validates the endpoint and returns its models without
	// calling an LLM. Wraps ErrModelListUnsupported when the endpoint has no
	// model list.
	ListModels(ctx context.Context, in SetupInput) ([]SetupModel, error)
	// Apply writes settings.json and config.toml.
	Apply(ctx context.Context, plan SetupPlan) (SetupReport, error)
	// Overrides lists settings that would override what /setup writes.
	Overrides() []string
	// NeedsSetup reports that no usable credentials were found at startup.
	NeedsSetup() bool
}

// ErrModelListUnsupported means the endpoint cannot list models; the wizard
// then asks for model IDs by hand.
var ErrModelListUnsupported = errors.New("endpoint does not support listing models")

func cmdSetup() *Command {
	return &Command{
		Name:        "setup",
		Description: "Set up the main model: endpoint, API key, model and tiers",
		Execute: func(_ CommandContext, _ string) Result {
			return Result{OpenDialog: "setup", Display: DisplayNone}
		},
	}
}
