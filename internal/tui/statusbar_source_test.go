package tui

import (
	"strings"
	"testing"
)

func barText(s StatusBar, width int) string {
	return stripANSI(s.View(width, DefaultDarkTheme))
}

func TestStatusBar_SameAsDefaultNoMarker(t *testing.T) {
	got := barText(StatusBar{model: "a", defaultModel: "a", cwd: "/p"}, 120)
	if strings.Contains(got, "session") || strings.Contains(got, "a*") {
		t.Errorf("unexpected marker: %q", got)
	}
}

func TestStatusBar_SessionSwitchShowsDefault(t *testing.T) {
	got := barText(StatusBar{model: "a", defaultModel: "b", cwd: "/p"}, 120)
	if !strings.Contains(got, "a (session · default b)") {
		t.Errorf("bar = %q", got)
	}
}

func TestStatusBar_RestartPending(t *testing.T) {
	got := barText(StatusBar{model: "a", defaultModel: "a", restartModel: "r", cwd: "/p"}, 120)
	if !strings.Contains(got, "a (restart → r)") {
		t.Errorf("bar = %q", got)
	}
}

func TestStatusBar_NoDefaultNoMarker(t *testing.T) {
	got := barText(StatusBar{model: "a", cwd: "/p"}, 120)
	if strings.Contains(got, "session") {
		t.Errorf("empty default must not mark session: %q", got)
	}
}

func TestStatusBar_NarrowCollapsesAnnotation(t *testing.T) {
	s := StatusBar{model: "a", defaultModel: "a-very-long-default-model-name", cwd: "/Users/someone/project",
		contextWindow: 200_000}.addUsage(TokenUsage{Input: 1000})
	got := barText(s, 60)
	if !strings.Contains(got, "a*") || strings.Contains(got, "session") {
		t.Errorf("bar = %q", got)
	}
	if w := displayWidth(got); w > 60 {
		t.Errorf("width %d > 60", w)
	}
}
