package config_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"github.com/tunsuy/glaude/internal/config"
)

func writeUserSettings(t *testing.T, home, content string, perm os.FileMode) string {
	t.Helper()
	path := config.UserSettingsPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	return path
}

func mainModelInput() config.MainModelSettings {
	return config.MainModelSettings{Provider: "openai", BaseURL: "https://x/v1", Model: "q/a", APIKey: "sk-new"}
}

func TestUserSettingsPath(t *testing.T) {
	if got := config.UserSettingsPath("/h"); got != filepath.Join("/h", ".glaude", "settings.json") {
		t.Errorf("UserSettingsPath = %q", got)
	}
}

func TestWriteUserMainModel_CreatesFile(t *testing.T) {
	home := t.TempDir()
	if err := config.WriteUserMainModel(home, mainModelInput()); err != nil {
		t.Fatal(err)
	}
	doc, err := config.LoadOrderedJSON(config.UserSettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"provider": "openai", "baseUrl": "https://x/v1", "model": "q/a", "apiKey": "sk-new"}
	for k, v := range want {
		if got := doc.GetString(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestWriteUserMainModel_PreservesOtherKeys(t *testing.T) {
	home := t.TempDir()
	writeUserSettings(t, home, `{"theme":"dark","permissions":{"allow":["x"]},"model":"old"}`, 0o644)
	if err := config.WriteUserMainModel(home, mainModelInput()); err != nil {
		t.Fatal(err)
	}
	doc, err := config.LoadOrderedJSON(config.UserSettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	keys := doc.Keys()
	if len(keys) < 3 || keys[0] != "theme" || keys[1] != "permissions" || keys[2] != "model" {
		t.Errorf("key order = %v, want theme, permissions, model first", keys)
	}
	if doc.GetString("theme") != "dark" || doc.GetString("model") != "q/a" {
		t.Errorf("doc = theme %q model %q", doc.GetString("theme"), doc.GetString("model"))
	}
	if perms, ok := doc.GetMap("permissions"); !ok || perms.Len() != 1 {
		t.Error("permissions object not preserved")
	}
}

func TestWriteUserMainModel_EmptyKeyKeepsExisting(t *testing.T) {
	home := t.TempDir()
	writeUserSettings(t, home, `{"apiKey":"old"}`, 0o600)
	in := mainModelInput()
	in.APIKey = ""
	if err := config.WriteUserMainModel(home, in); err != nil {
		t.Fatal(err)
	}
	doc, _ := config.LoadOrderedJSON(config.UserSettingsPath(home))
	if got := doc.GetString("apiKey"); got != "old" {
		t.Errorf("apiKey = %q, want old", got)
	}
}

func TestWriteUserMainModel_Permissions0600(t *testing.T) {
	home := t.TempDir()
	path := writeUserSettings(t, home, `{}`, 0o644)
	if err := config.WriteUserMainModel(home, mainModelInput()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}

func TestWriteUserMainModel_RefusesInvalidJSON(t *testing.T) {
	home := t.TempDir()
	path := writeUserSettings(t, home, `{bad`, 0o644)
	if err := config.WriteUserMainModel(home, mainModelInput()); err == nil {
		t.Fatal("want error for unparseable settings.json")
	}
	if b, _ := os.ReadFile(path); string(b) != "{bad" {
		t.Errorf("file changed: %q", b)
	}
}

// TestMainModelEnvVars_MatchesLoader keeps MainModelEnvVars in sync with the
// main-model env vars applyEnvOverrides actually reads.
func TestMainModelEnvVars_MatchesLoader(t *testing.T) {
	src, err := os.ReadFile("loader.go")
	if err != nil {
		t.Fatal(err)
	}
	notMainModel := map[string]bool{"OPENAI_ORGANIZATION": true, "OPENAI_ORG_ID": true, "OPENAI_PROJECT": true}
	var read []string
	for _, m := range regexp.MustCompile(`Getenv\("([A-Z_]+)"\)`).FindAllStringSubmatch(string(src), -1) {
		if !notMainModel[m[1]] && !slices.Contains(read, m[1]) {
			read = append(read, m[1])
		}
	}
	got := slices.Clone(config.MainModelEnvVars)
	slices.Sort(got)
	slices.Sort(read)
	if !slices.Equal(got, read) {
		t.Errorf("MainModelEnvVars = %v, loader reads %v", got, read)
	}

	for _, v := range config.MainModelEnvVars {
		for _, other := range config.MainModelEnvVars {
			t.Setenv(other, "")
		}
		t.Setenv(v, "from-env")
		ls, err := config.NewLoader(t.TempDir(), t.TempDir()).Load()
		if err != nil {
			t.Fatal(err)
		}
		m := ls.Merged
		if m.Provider != "from-env" && m.APIKey != "from-env" && m.BaseURL != "from-env" && m.Model != "from-env" {
			t.Errorf("%s does not override a main-model field", v)
		}
	}
}
