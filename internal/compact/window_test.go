package compact

import "testing"

func TestResolveContextWindow_Precedence(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "50000")
	if got := ResolveContextWindow("x", 131072); got != 50000 {
		t.Errorf("env override = %d, want 50000", got)
	}

	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	cases := []struct {
		model      string
		configured int
		want       int
	}{
		{"qwen", 131072, 131072},
		{"claude-sonnet-5-5", 0, 200_000},
		{"claude-opus-5-5", 0, 200_000},
		{"claude-haiku-4-5", 0, 200_000},
		{"claude-2.1", 0, 100_000},
		{"glm-5.3-flash", 0, 200_000},
		{"claude-2.1", 50_000, 50_000},
	}
	for _, c := range cases {
		if got := ResolveContextWindow(c.model, c.configured); got != c.want {
			t.Errorf("ResolveContextWindow(%q, %d) = %d, want %d", c.model, c.configured, got, c.want)
		}
	}
}

func TestTarget_Window(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	if got := (Target{Model: "qwen", ContextWindow: 32000}).Window(); got != 32000 {
		t.Errorf("Window = %d, want 32000", got)
	}
}

func TestKnownContextWindow(t *testing.T) {
	if n, ok := KnownContextWindow("claude-sonnet-5-5"); !ok || n != 200_000 {
		t.Errorf("claude = %d %v", n, ok)
	}
	if n, ok := KnownContextWindow("qwen"); ok || n != 0 {
		t.Errorf("qwen = %d %v", n, ok)
	}
}
