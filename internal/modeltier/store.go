package modeltier

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// ProviderStatus is a display view of one provider for /tier.
type ProviderStatus struct {
	ID        string
	Format    string
	BaseURL   string
	APIKeyEnv string
	// KeyPresent reports whether APIKeyEnv is set, checked at call time.
	// Always true for MainProviderID and for providers without APIKeyEnv.
	KeyPresent bool
}

// List returns every known tier sorted by name. lite / pro / max are always
// present; unbound tiers have an empty Provider.
func (s *Store) List() []Binding {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := map[string]bool{}
	for _, t := range defaultTiers {
		names[t] = true
	}
	for n := range s.cfg.Tiers {
		names[n] = true
	}
	out := make([]Binding, 0, len(names))
	for n := range names {
		b, _ := resolve(&s.cfg, n)
		b.Tier = n
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tier < out[j].Tier })
	return out
}

// Providers returns the main pseudo-provider followed by the registered
// providers sorted by id.
func (s *Store) Providers() []ProviderStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []ProviderStatus{{ID: MainProviderID, Format: "(main loop)", KeyPresent: true}}
	ids := make([]string, 0, len(s.cfg.Providers))
	for id := range s.cfg.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := s.cfg.Providers[id]
		out = append(out, ProviderStatus{
			ID:         id,
			Format:     p.Format,
			BaseURL:    p.BaseURL,
			APIKeyEnv:  p.APIKeyEnv,
			KeyPresent: p.APIKeyEnv == "" || os.Getenv(p.APIKeyEnv) != "",
		})
	}
	return out
}

// Bind points tier at providerID:modelID and atomically rewrites
// config.toml. Tier-level context_window / max_tokens are cleared because
// they described the previous model. Comments in the file are not preserved.
func (s *Store) Bind(tier, providerID, modelID string) error {
	name, err := validTierName(tier)
	if err != nil {
		return err
	}
	if strings.TrimSpace(modelID) == "" {
		return fmt.Errorf("modeltier: bind %s: model is empty", name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cfg.Providers[providerID]; !ok && providerID != MainProviderID {
		return fmt.Errorf("modeltier: bind %s: provider %q is not registered in %s", name, providerID, s.path)
	}
	return s.commitLocked(name, TierEntry{Provider: providerID, Model: strings.TrimSpace(modelID)})
}

// Unbind clears tier's binding (the tier stays known, so it inherits the
// main loop's model) and atomically rewrites config.toml.
func (s *Store) Unbind(tier string) error {
	name, err := validTierName(tier)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitLocked(name, TierEntry{})
}

// commitLocked sets tiers[name] in config.toml with a read-modify-write of
// the file as it is on disk now, so sections the validator ignored, unknown
// keys and edits made after startup all survive (only comments are lost).
// The in-memory config is replaced only after the write succeeded.
// Caller holds mu.
func (s *Store) commitLocked(name string, entry TierEntry) error {
	if s.loadErr != nil {
		return fmt.Errorf("modeltier: %s could not be loaded at startup (%v); fix the file and restart before using /tier", s.path, s.loadErr)
	}
	raw := map[string]any{}
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("modeltier: read %s: %w", s.path, err)
	default:
		if _, err := toml.Decode(string(data), &raw); err != nil {
			return fmt.Errorf("modeltier: %s no longer parses (%v); not rewriting it", s.path, err)
		}
	}
	tiers, _ := raw["tiers"].(map[string]any)
	if tiers == nil {
		tiers = map[string]any{}
	}
	for k := range tiers {
		if strings.EqualFold(strings.TrimSpace(k), name) {
			delete(tiers, k)
		}
	}
	table := map[string]any{}
	if entry.Provider != "" {
		table["provider"] = entry.Provider
		table["model"] = entry.Model
	}
	tiers[name] = table
	raw["tiers"] = tiers

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return fmt.Errorf("modeltier: encode config: %w", err)
	}
	next, _, err := parse(buf.Bytes())
	if err != nil {
		return fmt.Errorf("modeltier: re-parse rewritten config: %w", err)
	}
	if err := writeAtomic(s.path, buf.Bytes()); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func validTierName(tier string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(tier))
	switch {
	case name == "":
		return "", fmt.Errorf("modeltier: tier name is empty")
	case strings.ContainsAny(name, ": \t."):
		return "", fmt.Errorf("modeltier: tier name %q must not contain ':', '.' or spaces", tier)
	case name == "inherit":
		return "", fmt.Errorf("modeltier: %q is reserved", name)
	}
	if t, ok := legacyToTier[name]; ok {
		return "", fmt.Errorf("modeltier: %q is a legacy alias of %q; bind %q instead", name, t, t)
	}
	return name, nil
}

func writeAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("modeltier: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".config.toml.*")
	if err != nil {
		return fmt.Errorf("modeltier: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return fmt.Errorf("modeltier: write temp file: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("modeltier: chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("modeltier: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("modeltier: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("modeltier: replace %s: %w", path, err)
	}
	return nil
}

// Path returns the config.toml path this Store reads and writes.
func (s *Store) Path() string {
	return s.path
}

// Warnings returns the startup warnings (config.toml problems and provider
// client failures) so they can be surfaced inside the TUI.
func (s *Store) Warnings() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.warnings...)
}

// AppendWarnings records additional startup warnings (e.g. a provider client
// that could not be built).
func (s *Store) AppendWarnings(ws ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, ws...)
}
