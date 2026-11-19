package modeltier

import (
	"slices"
	"strings"
)

// Binding is a resolved routing target. ContextWindow and MaxTokens are 0
// when unspecified (callers then fall back to the built-in model table and
// the engine default, respectively).
type Binding struct {
	// Tier is the normalised tier name ("" for explicit provider:model).
	Tier string
	// Provider is a registered provider id or MainProviderID.
	Provider string
	// Model is passed to the provider endpoint verbatim.
	Model string
	// ContextWindow resolves tier value > provider model table > 0.
	ContextWindow int
	// MaxTokens resolves tier value > provider model table > 0.
	MaxTokens int
}

// ResolveKind classifies a Resolve result.
type ResolveKind int

const (
	// NotTier means the name is not a tier, alias or provider:model reference;
	// callers should treat it as a plain model ID (or inherit for ""/"inherit").
	NotTier ResolveKind = iota
	// Unbound means the name is a known tier that currently has no binding.
	// Binding.Tier carries the normalised tier name.
	Unbound
	// Bound means the Binding is complete and should be used.
	Bound
)

// defaultTiers are always known, even when absent from config.toml.
var defaultTiers = []string{"lite", "pro", "max"}

var legacyToTier = map[string]string{"haiku": "lite", "sonnet": "pro", "opus": "max"}

var tierToLegacy = map[string]string{"lite": "haiku", "pro": "sonnet", "max": "opus"}

// LegacyAlias returns the legacy Anthropic family alias for a default tier
// (lite→haiku, pro→sonnet, max→opus).
func LegacyAlias(tier string) (string, bool) {
	alias, ok := tierToLegacy[strings.ToLower(strings.TrimSpace(tier))]
	return alias, ok
}

// Resolve maps a tier name, legacy alias or explicit "providerID:modelID"
// to a Binding. Tier names and aliases are case-insensitive; model IDs keep
// their case. "providerID:modelID" splits at the first ':' and only matches
// a registered provider (or MainProviderID).
func (s *Store) Resolve(name string) (Binding, ResolveKind) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolve(&s.cfg, name)
}

func resolve(cfg *Config, name string) (Binding, ResolveKind) {
	raw := strings.TrimSpace(name)
	n := strings.ToLower(raw)
	if n == "" || n == "inherit" {
		return Binding{}, NotTier
	}
	if tier, ok := legacyToTier[n]; ok {
		n = tier
	}
	if entry, ok := cfg.Tiers[n]; ok || isDefaultTier(n) {
		if entry.Provider == "" || entry.Model == "" {
			return Binding{Tier: n}, Unbound
		}
		info := modelInfo(cfg, entry.Provider, entry.Model)
		return Binding{
			Tier:          n,
			Provider:      entry.Provider,
			Model:         entry.Model,
			ContextWindow: firstPositive(entry.ContextWindow, info.ContextWindow),
			MaxTokens:     firstPositive(entry.MaxTokens, info.MaxTokens),
		}, Bound
	}
	if i := strings.Index(raw, ":"); i > 0 && i < len(raw)-1 {
		provider := raw[:i]
		if _, ok := cfg.Providers[provider]; ok || provider == MainProviderID {
			model := raw[i+1:]
			info := modelInfo(cfg, provider, model)
			return Binding{
				Provider:      provider,
				Model:         model,
				ContextWindow: info.ContextWindow,
				MaxTokens:     info.MaxTokens,
			}, Bound
		}
	}
	return Binding{}, NotTier
}

// MainContextWindow returns [main] context_window (0 when unset).
func (s *Store) MainContextWindow() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg.Main.ContextWindow
}

// Provider returns the registered provider with id.
func (s *Store) Provider(id string) (Provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.cfg.Providers[id]
	return p, ok
}

func isDefaultTier(n string) bool {
	return slices.Contains(defaultTiers, n)
}

func modelInfo(cfg *Config, provider, model string) ModelInfo {
	if p, ok := cfg.Providers[provider]; ok {
		return p.Models[model]
	}
	return ModelInfo{}
}

func firstPositive(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}
