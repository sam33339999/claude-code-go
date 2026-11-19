package modeltier

import "testing"

func TestResolve_TierBound(t *testing.T) {
	s, _ := loadString(t, specExample)
	b, kind := s.Resolve("lite")
	want := Binding{Tier: "lite", Provider: "zai", Model: "glm-5.3-flash", MaxTokens: 8192}
	if kind != Bound || b != want {
		t.Errorf("Resolve(lite) = %+v %v, want %+v Bound", b, kind, want)
	}
}

func TestResolve_LegacyAlias(t *testing.T) {
	s, _ := loadString(t, specExample)
	lite, _ := s.Resolve("lite")
	for _, name := range []string{"haiku", "HAIKU", " Lite "} {
		b, kind := s.Resolve(name)
		if kind != Bound || b != lite {
			t.Errorf("Resolve(%q) = %+v %v, want %+v", name, b, kind, lite)
		}
	}
}

func TestResolve_ValuePrecedence(t *testing.T) {
	s, _ := loadString(t, specExample)
	b, _ := s.Resolve("pro")
	if b.ContextWindow != 262144 || b.MaxTokens != 16384 {
		t.Errorf("pro from provider model table = %d/%d, want 262144/16384", b.ContextWindow, b.MaxTokens)
	}

	s2, _ := loadString(t, specExample+`
[tiers.custom]
provider = "openrouter"
model = "qwen/qwen3.8-27b"
context_window = 100000
`)
	b2, _ := s2.Resolve("custom")
	if b2.ContextWindow != 100000 || b2.MaxTokens != 16384 {
		t.Errorf("tier value precedence = %d/%d, want 100000/16384", b2.ContextWindow, b2.MaxTokens)
	}
}

func TestResolve_MainTierUsesOwnValues(t *testing.T) {
	s, _ := loadString(t, specExample)
	b, kind := s.Resolve("max")
	if kind != Bound || b.Provider != MainProviderID || b.ContextWindow != 200000 {
		t.Errorf("Resolve(max) = %+v %v, want main with 200000 (not [main] 131072)", b, kind)
	}
}

func TestResolve_UnboundKnownTier(t *testing.T) {
	s, _ := loadString(t, `
[tiers.foo]
`)
	b, kind := s.Resolve("foo")
	if kind != Unbound || b.Tier != "foo" {
		t.Errorf("Resolve(foo) = %+v %v, want Unbound with Tier foo", b, kind)
	}
	b, kind = s.Resolve("opus")
	if kind != Unbound || b.Tier != "max" {
		t.Errorf("Resolve(opus) = %+v %v, want Unbound with Tier max", b, kind)
	}
}

func TestResolve_ColonInModelID(t *testing.T) {
	s, _ := loadString(t, specExample)
	b, kind := s.Resolve("openrouter:qwen/qwen3:free")
	if kind != Bound || b.Provider != "openrouter" || b.Model != "qwen/qwen3:free" || b.Tier != "" {
		t.Errorf("Resolve(openrouter:qwen/qwen3:free) = %+v %v", b, kind)
	}
	if _, kind := s.Resolve("qwen3:free"); kind != NotTier {
		t.Errorf("Resolve(qwen3:free) kind = %v, want NotTier", kind)
	}
	b, _ = s.Resolve("openrouter:qwen/qwen3.8-27b")
	if b.ContextWindow != 262144 {
		t.Errorf("explicit provider:model should use model table, got %d", b.ContextWindow)
	}
}

func TestResolve_EmptyAndInherit(t *testing.T) {
	s, _ := loadString(t, specExample)
	for _, name := range []string{"", "  ", "inherit"} {
		if _, kind := s.Resolve(name); kind != NotTier {
			t.Errorf("Resolve(%q) kind = %v, want NotTier", name, kind)
		}
	}
}

func TestLegacyAlias(t *testing.T) {
	cases := map[string]string{"lite": "haiku", "pro": "sonnet", "max": "opus"}
	for tier, want := range cases {
		if got, ok := LegacyAlias(tier); !ok || got != want {
			t.Errorf("LegacyAlias(%q) = %q %v, want %q", tier, got, ok, want)
		}
	}
	if got, ok := LegacyAlias("foo"); ok || got != "" {
		t.Errorf("LegacyAlias(foo) = %q %v, want \"\" false", got, ok)
	}
}
