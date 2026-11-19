package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// MainModelSettings are the main-loop fields /setup writes to the user
// settings.json. An empty APIKey leaves any existing apiKey untouched.
type MainModelSettings struct {
	Provider string
	BaseURL  string
	Model    string
	APIKey   string
}

// MainModelEnvVars lists every environment variable applyEnvOverrides reads
// for the main model (provider, API key, base URL, model). Any of them being
// set overrides settings.json, so /setup warns about them.
var MainModelEnvVars = []string{
	"CLAUDE_PROVIDER", "API_PROVIDER",
	"ANTHROPIC_API_KEY", "OPENAI_API_KEY",
	"ANTHROPIC_BASE_URL", "OPENAI_BASE_URL", "OPENAI_API_BASE",
	"ANTHROPIC_MODEL", "CLAUDE_MODEL", "OPENAI_MODEL",
}

// UserSettingsPath returns the global ~/.glaude/settings.json path.
func UserSettingsPath(homeDir string) string {
	return filepath.Join(homeDir, ClaudeDir, SettingsFile)
}

// WriteUserMainModel sets provider, baseUrl, model (and apiKey when non-empty)
// in the user settings.json with a read-modify-write that keeps every other
// key and the key order. The file holds an API key, so it ends up 0600. An
// existing file that does not parse is left untouched and reported.
func WriteUserMainModel(homeDir string, s MainModelSettings) error {
	path := UserSettingsPath(homeDir)
	doc, err := LoadOrderedJSON(path)
	if err != nil {
		return fmt.Errorf("config: %s is not valid JSON; not rewriting it: %w", path, err)
	}
	doc.Set("provider", s.Provider)
	doc.Set("baseUrl", s.BaseURL)
	doc.Set("model", s.Model)
	if s.APIKey != "" {
		doc.Set("apiKey", s.APIKey)
	}
	if err := WriteOrderedJSONAtomic(path, doc); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("config: chmod %s: %w", path, err)
	}
	return nil
}
