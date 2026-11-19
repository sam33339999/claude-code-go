package commands

import (
	"fmt"
	"strings"

	"github.com/tunsuy/glaude/internal/modeltier"
)

// TierManager is the tier table view /tier needs. *modeltier.Store
// implements it; bootstrap injects it through the TUI (commands must not
// import bootstrap).
type TierManager interface {
	// List returns every known tier (unbound ones have an empty Provider).
	List() []modeltier.Binding
	// Providers returns the main pseudo-provider and registered providers.
	Providers() []modeltier.ProviderStatus
	// Bind points tier at providerID:modelID and rewrites config.toml.
	Bind(tier, providerID, modelID string) error
	// Unbind clears tier's binding and rewrites config.toml.
	Unbind(tier string) error
	// MainContextWindow returns [main] context_window (0 = unset).
	MainContextWindow() int
}

var _ TierManager = (*modeltier.Store)(nil)

const tierUsage = "Usage: /tier | /tier <tier> <providerID>:<modelID> | /tier <tier> none"

func cmdTier() *Command {
	return &Command{
		Name:        "tier",
		Description: "Show or change model tier bindings (lite/pro/max) from ~/.glaude/config.toml",
		Execute: func(ctx CommandContext, args string) Result {
			if ctx.Tiers == nil {
				return Result{Text: "Model tiers are not available in this session.", Display: DisplayError}
			}
			fields := strings.Fields(args)
			switch {
			case len(fields) == 0:
				return Result{Text: formatTiers(ctx.Tiers), Display: DisplayMessage}
			case len(fields) == 2 && strings.EqualFold(fields[1], "none"):
				if err := ctx.Tiers.Unbind(fields[0]); err != nil {
					return Result{Text: err.Error(), Display: DisplayError}
				}
				return Result{Text: fmt.Sprintf("Tier %s unbound; the next sub-agent dispatched with it inherits the main model.", fields[0]), Display: DisplayMessage}
			case len(fields) == 2:
				return bindTier(ctx.Tiers, fields[0], fields[1])
			default:
				return Result{Text: tierUsage, Display: DisplayError}
			}
		},
	}
}

func bindTier(m TierManager, tier, ref string) Result {
	i := strings.Index(ref, ":")
	if i <= 0 || i == len(ref)-1 {
		return Result{Text: tierUsage, Display: DisplayError}
	}
	provider, model := ref[:i], ref[i+1:]
	if err := m.Bind(tier, provider, model); err != nil {
		return Result{Text: err.Error(), Display: DisplayError}
	}
	return Result{
		Text: fmt.Sprintf("Tier %s → %s:%s. Takes effect for the next sub-agent dispatched "+
			"(running agents keep their binding). Note: config.toml was rewritten and its comments were not preserved.",
			tier, provider, model),
		Display: DisplayMessage,
	}
}

func formatTiers(m TierManager) string {
	var b strings.Builder
	b.WriteString("Tiers:\n")
	for _, t := range m.List() {
		if t.Provider == "" {
			fmt.Fprintf(&b, "  %-6s (unbound, inherits main model)\n", t.Tier)
			continue
		}
		fmt.Fprintf(&b, "  %-6s %s:%s", t.Tier, t.Provider, t.Model)
		if t.ContextWindow > 0 {
			fmt.Fprintf(&b, "  context_window=%d", t.ContextWindow)
		}
		if t.MaxTokens > 0 {
			fmt.Fprintf(&b, "  max_tokens=%d", t.MaxTokens)
		}
		b.WriteString("\n")
	}
	b.WriteString("Providers:\n")
	for _, p := range m.Providers() {
		key := ""
		switch {
		case p.APIKeyEnv == "":
		case p.KeyPresent:
			key = "  key " + p.APIKeyEnv + " set"
		default:
			key = "  key " + p.APIKeyEnv + " missing"
		}
		fmt.Fprintf(&b, "  %-12s %s%s\n", p.ID, p.Format, key)
	}
	b.WriteString(tierUsage)
	return b.String()
}
