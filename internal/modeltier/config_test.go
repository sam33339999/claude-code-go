package modeltier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// specExample is the example config.toml from the design doc (§5.1).
const specExample = `
[providers.qwencloud]
format      = "openai"
base_url    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
api_key_env = "DASHSCOPE_API_KEY"

[providers.openrouter]
format      = "openai"
base_url    = "https://openrouter.ai/api/v1"
api_key_env = "OPENROUTER_API_KEY"

[providers.zai]
format      = "openai"
base_url    = "https://api.z.ai/api/paas/v4"
api_key_env = "ZAI_API_KEY"

[providers.openrouter.models."qwen/qwen3.8-27b"]
context_window = 262144
max_tokens     = 16384

[main]
context_window = 131072

[tiers.lite]
provider   = "zai"
model      = "glm-5.3-flash"
max_tokens = 8192

[tiers.pro]
provider = "openrouter"
model    = "qwen/qwen3.8-27b"

[tiers.max]
provider       = "main"
model          = "claude-opus-5-5"
context_window = 200000
`

// writeConfig writes content to a temp config.toml and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func loadString(t *testing.T, content string) (*Store, []string) {
	t.Helper()
	return Load(writeConfig(t, content))
}

func hasWarning(warnings []string, sub string) bool {
	for _, w := range warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestDefaultPath(t *testing.T) {
	got := DefaultPath("/home/u")
	if got != filepath.Join("/home/u", ".glaude", "config.toml") {
		t.Errorf("DefaultPath = %q", got)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	s, warnings := Load(filepath.Join(t.TempDir(), "absent.toml"))
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	if _, kind := s.Resolve("lite"); kind != Unbound {
		t.Errorf("Resolve(lite) kind = %v, want Unbound", kind)
	}
	if _, kind := s.Resolve("qwen-turbo"); kind != NotTier {
		t.Errorf("Resolve(qwen-turbo) kind = %v, want NotTier", kind)
	}
	if got := s.MainContextWindow(); got != 0 {
		t.Errorf("MainContextWindow = %d, want 0", got)
	}
}

func TestLoad_MalformedFile(t *testing.T) {
	path := writeConfig(t, "[[[")
	s, warnings := Load(path)
	if len(warnings) != 1 || !strings.Contains(warnings[0], path) {
		t.Fatalf("warnings = %v, want one naming %s", warnings, path)
	}
	if _, kind := s.Resolve("lite"); kind != Unbound {
		t.Errorf("Resolve(lite) kind = %v, want Unbound", kind)
	}
}

func TestLoad_RejectsBadFormat(t *testing.T) {
	s, warnings := loadString(t, `
[providers.x]
format = "bedrock"
base_url = "https://x"
api_key_env = "X_KEY"
`)
	if !hasWarning(warnings, "providers.x") {
		t.Errorf("warnings = %v, want one naming providers.x", warnings)
	}
	if _, ok := s.Provider("x"); ok {
		t.Error("provider x should have been dropped")
	}
}

func TestLoad_ReservedMainIgnored(t *testing.T) {
	s, warnings := loadString(t, `
[providers.main]
format = "openai"
base_url = "https://x"
api_key_env = "X_KEY"
`)
	if !hasWarning(warnings, "providers.main") {
		t.Errorf("warnings = %v, want one naming providers.main", warnings)
	}
	b, kind := s.Resolve("main:foo")
	if kind != Bound || b.Provider != MainProviderID || b.Model != "foo" {
		t.Errorf("Resolve(main:foo) = %+v %v, want Bound main/foo", b, kind)
	}
}

func TestLoad_TierWithUnknownProvider(t *testing.T) {
	s, warnings := loadString(t, `
[tiers.lite]
provider = "nope"
model = "m"
`)
	if !hasWarning(warnings, "tiers.lite") {
		t.Errorf("warnings = %v, want one naming tiers.lite", warnings)
	}
	if _, kind := s.Resolve("lite"); kind != Unbound {
		t.Errorf("Resolve(lite) kind = %v, want Unbound", kind)
	}
}

func TestLoad_PlaintextAPIKeyRejected(t *testing.T) {
	s, warnings := loadString(t, `
[providers.x]
format = "openai"
base_url = "https://x"
api_key = "sk-secret"
`)
	if !hasWarning(warnings, "api_key_env") {
		t.Errorf("warnings = %v, want one mentioning api_key_env", warnings)
	}
	if hasWarning(warnings, "sk-secret") {
		t.Error("warning must not echo the secret value")
	}
	if _, ok := s.Provider("x"); ok {
		t.Error("provider x with plaintext api_key should have been dropped")
	}
}

func TestLoad_MainSection(t *testing.T) {
	s, warnings := loadString(t, specExample)
	if len(warnings) != 0 {
		t.Errorf("spec example produced warnings: %v", warnings)
	}
	if got := s.MainContextWindow(); got != 131072 {
		t.Errorf("MainContextWindow = %d, want 131072", got)
	}
}

func TestLoad_NonPositiveValuesWarned(t *testing.T) {
	_, warnings := loadString(t, `
[main]
context_window = -1
`)
	if !hasWarning(warnings, "main.context_window") {
		t.Errorf("warnings = %v, want one naming main.context_window", warnings)
	}
}

// TestExampleConfigParses keeps docs/config.example.toml in sync with the
// loader: it must parse without a single warning.
func TestExampleConfigParses(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "config.example.toml")
	s, warnings := Load(path)
	if len(warnings) != 0 {
		t.Fatalf("example produced warnings: %v", warnings)
	}
	b, kind := s.Resolve("lite")
	if kind != Bound || b.Provider != "zai" || b.Model != "glm-5.3-flash" || b.ContextWindow != 131072 || b.MaxTokens != 8192 {
		t.Errorf("lite = %+v %v", b, kind)
	}
	if b, kind := s.Resolve("max"); kind != Bound || b.Provider != MainProviderID {
		t.Errorf("max = %+v %v", b, kind)
	}
	if _, ok := s.Provider("qwencloud"); !ok {
		t.Error("qwencloud provider missing")
	}
}
