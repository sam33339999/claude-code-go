package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/tunsuy/glaude/internal/agenttype"
	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/commands"
	"github.com/tunsuy/glaude/internal/modeltier"
)

// providerClientFactory builds the api.Client for one registered provider.
// apiKey is the value of the provider's api_key_env.
type providerClientFactory func(id string, p modeltier.Provider, apiKey string) (api.Client, error)

// tierRouter routes sub-agents to the provider client of their tier binding.
// Clients are built once at startup (they do not connect until used).
type tierRouter struct {
	store   *modeltier.Store
	clients map[string]api.Client
	// keyErr records providers whose client could not be built (missing key
	// or construction error); dispatching to them fails loudly.
	keyErr map[string]error
}

// subAgentTarget is where and how a sub-agent query runs. A nil Client means
// the engine default (main loop) client.
type subAgentTarget struct {
	Model         string
	Client        api.Client
	ContextWindow int
	MaxTokens     int
}

// loadTierRouter loads ~/.glaude/config.toml and builds one client per
// provider. The main loop's client is registered under modeltier.MainProviderID.
// A nil factory uses api.NewClient. The returned warnings are for display.
func loadTierRouter(homeDir string, mainClient api.Client, factory providerClientFactory) (*tierRouter, []string) {
	store, warnings := modeltier.Load(modeltier.DefaultPath(homeDir))
	if factory == nil {
		factory = defaultProviderClient
	}
	r := &tierRouter{
		store:   store,
		clients: map[string]api.Client{modeltier.MainProviderID: mainClient},
		keyErr:  map[string]error{},
	}
	for _, ps := range store.Providers() {
		if ps.ID == modeltier.MainProviderID {
			continue
		}
		p, _ := store.Provider(ps.ID)
		key := ""
		if p.APIKeyEnv != "" {
			key = os.Getenv(p.APIKeyEnv)
			if key == "" {
				r.keyErr[ps.ID] = fmt.Errorf("environment variable %s is not set", p.APIKeyEnv)
				continue
			}
		}
		client, err := factory(ps.ID, p, key)
		if err != nil {
			r.keyErr[ps.ID] = fmt.Errorf("build client: %w", err)
			warnings = append(warnings, fmt.Sprintf("config.toml: providers.%s: %v", ps.ID, err))
			continue
		}
		r.clients[ps.ID] = client
	}
	return r, warnings
}

func defaultProviderClient(_ string, p modeltier.Provider, apiKey string) (api.Client, error) {
	provider := api.ProviderOpenAI
	if p.Format == modeltier.FormatAnthropic {
		provider = api.ProviderDirect
	}
	return api.NewClient(api.ClientConfig{Provider: provider, BaseURL: p.BaseURL, APIKey: apiKey}, nil)
}

// resolveSubAgentTarget applies the sub-agent model policy with tier routing.
// Candidate order is unchanged (CLAUDE_CODE_SUBAGENT_MODEL > tool model >
// profile model > inherit); a candidate that names a bound tier, legacy alias
// or providerID:modelID is routed to that provider. Unbound lite/pro/max fall
// back to their legacy alias rules; unbound custom tiers inherit.
func resolveSubAgentTarget(r *tierRouter, provider, parentModel, toolModel, profileModel string) (subAgentTarget, error) {
	if r == nil {
		return subAgentTarget{Model: agenttype.ResolveModelForProvider(provider, parentModel, legacyOf(toolModel), legacyOf(profileModel))}, nil
	}
	cand := firstNonEmpty(os.Getenv("CLAUDE_CODE_SUBAGENT_MODEL"), toolModel, profileModel)
	if cand == "" || strings.EqualFold(cand, "inherit") {
		return subAgentTarget{}, nil
	}
	b, kind := r.store.Resolve(cand)
	switch kind {
	case modeltier.Bound:
		t := subAgentTarget{Model: b.Model, ContextWindow: b.ContextWindow, MaxTokens: b.MaxTokens}
		if b.Provider == modeltier.MainProviderID {
			return t, nil
		}
		if err := r.keyErr[b.Provider]; err != nil {
			return subAgentTarget{}, fmt.Errorf("tier %q → provider %q: %w", tierLabel(b, cand), b.Provider, err)
		}
		t.Client = r.clients[b.Provider]
		return t, nil
	case modeltier.Unbound:
		if alias, ok := modeltier.LegacyAlias(b.Tier); ok {
			return subAgentTarget{Model: agenttype.ResolveAliasForProvider(provider, parentModel, alias)}, nil
		}
		return subAgentTarget{}, nil
	default:
		return subAgentTarget{Model: agenttype.ResolveModelForProvider(provider, parentModel, toolModel, profileModel)}, nil
	}
}

// legacyOf maps lite/pro/max to haiku/sonnet/opus (for the router-less path).
func legacyOf(model string) string {
	if alias, ok := modeltier.LegacyAlias(model); ok {
		return alias
	}
	return model
}

func tierLabel(b modeltier.Binding, cand string) string {
	if b.Tier != "" {
		return b.Tier
	}
	return cand
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// wireTiers loads the tier router for a container build and reports config
// warnings on stderr (a broken config.toml never stops startup).
func wireTiers(opts ContainerOptions, mainClient api.Client) *tierRouter {
	router, warnings := loadTierRouter(opts.HomeDir, mainClient, opts.ProviderClientFactory)
	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	return router
}

// engineDebugLogf returns the engine's debug logger (nil unless --debug).
func engineDebugLogf(opts ContainerOptions) func(string, ...any) {
	if !opts.Debug {
		return nil
	}
	return func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
	}
}

// tiersOrNil converts a possibly-nil *modeltier.Store into the TUI's
// TierManager without producing a typed-nil interface.
func tiersOrNil(s *modeltier.Store) commands.TierManager {
	if s == nil {
		return nil
	}
	return s
}
