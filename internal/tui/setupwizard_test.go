package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/commands"
)

func wKey(w *setupWizard, name string) setupAction {
	switch name {
	case "enter":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "esc":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	case "up":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	case "down":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	case "left":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	case "right":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	case "tab":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	case "space":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	case "backspace":
		return w.handleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	panic("unknown key " + name)
}

func wType(w *setupWizard, s string) {
	for _, r := range s {
		w.handleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func wView(w *setupWizard) string {
	return w.view(100, 30, DefaultDarkTheme)
}

// toEndpoint picks format index idx (0 Anthropic, 1 OpenAI) and enters step 2.
func toEndpoint(w *setupWizard, idx int) {
	for w.formatIdx != idx {
		wKey(w, "down")
	}
	wKey(w, "enter")
}

// toModels drives a fresh wizard to the model list with the given models.
func toModels(t *testing.T, d commands.SetupDefaults, models ...string) *setupWizard {
	t.Helper()
	w := newSetupWizard(d, nil)
	toEndpoint(w, 1)
	wKey(w, "tab")
	wType(w, "sk-test")
	if a := wKey(w, "enter"); a != actValidate {
		t.Fatalf("enter on endpoint = %v, want actValidate", a)
	}
	var ms []commands.SetupModel
	for _, id := range models {
		ms = append(ms, commands.SetupModel{ID: id})
	}
	w.onModels(ms, nil)
	if w.step != stepModels {
		t.Fatalf("step = %v, want stepModels", w.step)
	}
	return w
}

func TestSetupWizard_FormatDefaultsFromCurrent(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{Format: commands.SetupFormatOpenAI, BaseURL: "https://or/v1"}, nil)
	if w.formatIdx != 1 {
		t.Errorf("formatIdx = %d, want OpenAI (1)", w.formatIdx)
	}
	wKey(w, "enter")
	if w.step != stepEndpoint || w.input().BaseURL != "https://or/v1" {
		t.Errorf("step %v baseURL %q", w.step, w.input().BaseURL)
	}
}

func TestSetupWizard_BaseURLDefaultPerFormat(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, nil)
	toEndpoint(w, 0)
	if got := w.input().BaseURL; got != "https://api.anthropic.com" {
		t.Errorf("anthropic base = %q", got)
	}
	wKey(w, "esc")
	toEndpoint(w, 1)
	if got := w.input().BaseURL; got != "https://api.openai.com" {
		t.Errorf("openai base = %q", got)
	}
	if w.input().Format != commands.SetupFormatOpenAI {
		t.Errorf("format = %q", w.input().Format)
	}
}

func TestSetupWizard_KeyMaskedAndReuseHint(t *testing.T) {
	d := commands.SetupDefaults{Format: commands.SetupFormatOpenAI, KeySources: map[string]string{commands.SetupFormatOpenAI: "settings"}}
	w := newSetupWizard(d, nil)
	toEndpoint(w, 1)
	if !strings.Contains(wView(w), "leave empty") {
		t.Error("reuse hint missing for unchanged format")
	}
	wKey(w, "tab")
	wType(w, "sk-abc")
	v := wView(w)
	if strings.Contains(v, "sk-abc") || !strings.Contains(v, "••••••") {
		t.Errorf("key not masked:\n%s", v)
	}
	wKey(w, "esc")
	toEndpoint(w, 0)
	if strings.Contains(wView(w), "leave empty") {
		t.Error("reuse hint shown after changing format")
	}
}

func TestSetupWizard_EnterValidates(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{Format: commands.SetupFormatOpenAI, KeySources: map[string]string{commands.SetupFormatOpenAI: "settings"}}, nil)
	toEndpoint(w, 1)
	gen := w.gen
	if a := wKey(w, "enter"); a != actValidate {
		t.Fatalf("action = %v", a)
	}
	if w.step != stepValidating || w.gen != gen+1 {
		t.Errorf("step %v gen %d", w.step, w.gen)
	}
	if in := w.input(); in.APIKey != "" || in.BaseURL != "https://api.openai.com" {
		t.Errorf("input = %+v (empty APIKey means reuse)", in)
	}
}

func TestSetupWizard_EmptyKeyWithoutSourceBlocked(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, nil)
	toEndpoint(w, 1)
	if a := wKey(w, "enter"); a != actNone {
		t.Errorf("action = %v, want actNone", a)
	}
	if !strings.Contains(wView(w), "API key is required") {
		t.Error("missing required-key message")
	}
}

func TestSetupWizard_ValidationErrorReturnsWithInputs(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, nil)
	toEndpoint(w, 1)
	wKey(w, "tab")
	wType(w, "sk")
	wKey(w, "enter")
	w.onModels(nil, errors.New("unauthorized: invalid key"))
	if w.step != stepEndpoint || w.input().BaseURL != "https://api.openai.com" || w.input().APIKey != "sk" {
		t.Errorf("step %v input %+v", w.step, w.input())
	}
	if !strings.Contains(wView(w), "unauthorized") {
		t.Error("error not shown")
	}
}

func TestSetupWizard_UnsupportedGoesManual(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, nil)
	toEndpoint(w, 1)
	wKey(w, "tab")
	wType(w, "sk")
	wKey(w, "enter")
	w.onModels(nil, fmt.Errorf("404: %w", commands.ErrModelListUnsupported))
	if w.step != stepManualModels {
		t.Fatalf("step = %v, want manual", w.step)
	}
	wType(w, "a, b ,c")
	wKey(w, "enter")
	if w.step != stepRoles || strings.Join(w.checkOrder, ",") != "a,b,c" {
		t.Errorf("step %v checked %v", w.step, w.checkOrder)
	}
}

func TestSetupWizard_FilterAndCheckPersist(t *testing.T) {
	w := toModels(t, commands.SetupDefaults{}, "q/a", "q/b", "x/c")
	wType(w, "q")
	if n := len(w.filtered()); n != 2 {
		t.Fatalf("filtered = %d, want 2", n)
	}
	wKey(w, "space")
	wKey(w, "backspace")
	if n := len(w.filtered()); n != 3 || !w.checked["q/a"] {
		t.Errorf("filtered %d, checked %v", n, w.checked)
	}
}

func TestSetupWizard_SpaceIsToggleNotFilter(t *testing.T) {
	w := toModels(t, commands.SetupDefaults{}, "q/a", "q/b")
	wType(w, "q")
	wKey(w, "space")
	if string(w.filter) != "q" {
		t.Errorf("filter = %q, want q", string(w.filter))
	}
}

func TestSetupWizard_EnterWithNoneCheckedUsesCursor(t *testing.T) {
	w := toModels(t, commands.SetupDefaults{}, "q/a", "q/b", "x/c")
	wKey(w, "down")
	wKey(w, "down")
	wKey(w, "enter")
	if w.step != stepConfirm || w.plan().MainModel != "x/c" {
		t.Errorf("step %v main %q", w.step, w.plan().MainModel)
	}
}

func TestSetupWizard_SingleCheckedSkipsRoles(t *testing.T) {
	w := toModels(t, commands.SetupDefaults{}, "q/a", "q/b")
	wKey(w, "space")
	wKey(w, "enter")
	if w.step != stepConfirm {
		t.Errorf("step = %v, want confirm", w.step)
	}
}

func rolesWizard(t *testing.T) *setupWizard {
	t.Helper()
	d := commands.SetupDefaults{Tiers: map[string]string{"lite": "main:q/b", "pro": "zai:glm", "max": "main:old"}}
	w := toModels(t, d, "q/a", "q/b", "x/c")
	wKey(w, "space") // q/a
	wKey(w, "down")
	wKey(w, "space") // q/b
	wKey(w, "enter")
	if w.step != stepRoles {
		t.Fatalf("step = %v, want roles", w.step)
	}
	return w
}

func TestSetupWizard_RolesPrefillCurrent(t *testing.T) {
	w := rolesWizard(t)
	v := wView(w)
	for _, want := range []string{"q/a", "q/b", "zai:glm", "main:old", "will be unbound"} {
		if !strings.Contains(v, want) {
			t.Errorf("roles view missing %q:\n%s", want, v)
		}
	}
	// pro (fixed, other provider) cannot be changed.
	wKey(w, "down")
	wKey(w, "down")
	wKey(w, "right")
	if _, ok := w.plan().Tiers["pro"]; ok {
		t.Error("pro bound to another provider must not be in the plan")
	}
}

func TestSetupWizard_PlanTiers(t *testing.T) {
	w := rolesWizard(t)
	w.models[0].ContextWindow = 0
	w.models[1].ContextWindow = 32768
	p := w.plan()
	if p.MainModel != "q/a" {
		t.Errorf("main = %q", p.MainModel)
	}
	if len(p.Tiers) != 2 || p.Tiers["lite"] != "q/b" || p.Tiers["max"] != "" {
		t.Errorf("tiers = %v, want lite=q/b, max unbind", p.Tiers)
	}
	if len(p.ContextWindows) != 1 || p.ContextWindows["q/b"] != 32768 {
		t.Errorf("context windows = %v", p.ContextWindows)
	}
	if p.Input.APIKey != "sk-test" {
		t.Errorf("plan input = %+v", p.Input)
	}
}

func TestSetupWizard_ConfirmShowsKeySuffixOnly(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, []string{"environment variable OPENAI_MODEL overrides settings.json"})
	toEndpoint(w, 1)
	wKey(w, "tab")
	wType(w, "sk-abcdefgh")
	wKey(w, "enter")
	w.onModels([]commands.SetupModel{{ID: "m"}}, nil)
	wKey(w, "enter")
	v := wView(w)
	if w.step != stepConfirm || !strings.Contains(v, "efgh") || strings.Contains(v, "sk-abcd") {
		t.Errorf("confirm view:\n%s", v)
	}
	if !strings.Contains(v, "OPENAI_MODEL") {
		t.Error("override warning missing")
	}
}

func TestSetupWizard_EscWalksBackAndClosesAtStart(t *testing.T) {
	w := rolesWizard(t)
	steps := []setupStep{stepModels, stepEndpoint, stepFormat}
	for _, want := range steps {
		wKey(w, "esc")
		if w.step != want {
			t.Fatalf("after esc step = %v, want %v", w.step, want)
		}
	}
	if a := wKey(w, "esc"); a != actClose {
		t.Errorf("esc at format = %v, want actClose", a)
	}
}

func TestSetupWizard_EscWhileValidatingInvalidates(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, nil)
	toEndpoint(w, 1)
	wKey(w, "tab")
	wType(w, "sk")
	wKey(w, "enter")
	gen := w.gen
	wKey(w, "esc")
	if w.step != stepEndpoint || w.gen == gen {
		t.Errorf("step %v gen %d (was %d)", w.step, w.gen, gen)
	}
}

func toConfirm(t *testing.T) *setupWizard {
	t.Helper()
	w := toModels(t, commands.SetupDefaults{}, "m")
	wKey(w, "enter")
	if a := wKey(w, "enter"); a != actApply || w.step != stepWriting {
		t.Fatalf("confirm enter = %v step %v", a, w.step)
	}
	return w
}

func TestSetupWizard_ApplyErrorStaysOnConfirm(t *testing.T) {
	w := toConfirm(t)
	w.onApplied(commands.SetupReport{SettingsWritten: true, SettingsPath: "/h/.glaude/settings.json"}, errors.New("config.toml no longer parses"))
	v := wView(w)
	if w.step != stepConfirm || !strings.Contains(v, "settings.json") || !strings.Contains(v, "no longer parses") {
		t.Errorf("step %v view:\n%s", w.step, v)
	}
}

func TestSetupWizard_DoneAsksRestart(t *testing.T) {
	w := toConfirm(t)
	w.onApplied(commands.SetupReport{SettingsWritten: true}, nil)
	if w.step != stepDone || !strings.Contains(strings.ToLower(wView(w)), "restart") {
		t.Errorf("step %v view:\n%s", w.step, wView(w))
	}
	if a := wType1(w, "x"); a != actClose {
		t.Errorf("key on done = %v, want actClose", a)
	}
}

func wType1(w *setupWizard, s string) setupAction {
	return w.handleKey(tea.KeyPressMsg{Code: rune(s[0]), Text: s})
}

func TestSetupWizard_ScrollKeepsCursorVisible(t *testing.T) {
	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprintf("model-%04d", i)
	}
	w := toModels(t, commands.SetupDefaults{}, ids...)
	for range 500 {
		wKey(w, "down")
	}
	if v := w.view(100, 20, DefaultDarkTheme); !strings.Contains(v, "model-0500") {
		t.Errorf("cursor model not visible:\n%s", v)
	}
}

func TestSetupWizard_PasteGoesToFocusedField(t *testing.T) {
	w := newSetupWizard(commands.SetupDefaults{}, nil)
	toEndpoint(w, 1)
	wKey(w, "tab")
	w.handlePaste("sk-pasted\n")
	if got := w.input().APIKey; got != "sk-pasted" {
		t.Errorf("APIKey = %q, want sk-pasted (newlines stripped)", got)
	}
}

// TestSetupWizard_KeySourcePerFormat: an env key for the other format must be
// offered after switching formats, and a settings key must not carry over.
func TestSetupWizard_KeySourcePerFormat(t *testing.T) {
	d := commands.SetupDefaults{
		Format: commands.SetupFormatAnthropic,
		KeySources: map[string]string{
			commands.SetupFormatAnthropic: "settings",
			commands.SetupFormatOpenAI:    "env:OPENAI_API_KEY",
		},
	}
	w := newSetupWizard(d, nil)
	toEndpoint(w, 1)
	if v := wView(w); !strings.Contains(v, "OPENAI_API_KEY") || !strings.Contains(v, "leave empty") {
		t.Errorf("env key for openai not offered:\n%s", v)
	}
	if a := wKey(w, "enter"); a != actValidate {
		t.Errorf("empty key with env source = %v, want actValidate", a)
	}
	w2 := newSetupWizard(commands.SetupDefaults{Format: commands.SetupFormatAnthropic, KeySources: map[string]string{commands.SetupFormatAnthropic: "settings"}}, nil)
	toEndpoint(w2, 1)
	if a := wKey(w2, "enter"); a != actNone {
		t.Errorf("settings key must not carry over to another format: %v", a)
	}
}

func TestSetupWizard_DoneNotesKeyCaveat(t *testing.T) {
	w := toConfirm(t)
	w.onApplied(commands.SetupReport{SettingsWritten: true}, nil)
	if !strings.Contains(wView(w), "401") {
		t.Error("done page must say a wrong key may only show up as 401 later")
	}
}
