package commands_test

import (
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/commands"
)

// envFrom builds a pure lookup function over a canned environment map.
func envFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDetectTerminal(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		terminal string
		shell    string
	}{
		{"vscode", map[string]string{"TERM_PROGRAM": "vscode", "SHELL": "/bin/zsh"}, "VS Code integrated terminal", "zsh"},
		{"iterm_program", map[string]string{"TERM_PROGRAM": "iTerm.app", "SHELL": "/bin/bash"}, "iTerm2", "bash"},
		{"iterm_session", map[string]string{"ITERM_SESSION_ID": "w0t0p0", "SHELL": "/usr/local/bin/fish"}, "iTerm2", "fish"},
		{"apple", map[string]string{"TERM_PROGRAM": "Apple_Terminal", "SHELL": "/bin/zsh"}, "Apple Terminal", "zsh"},
		{"wezterm", map[string]string{"TERM_PROGRAM": "WezTerm"}, "WezTerm", "unknown"},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, "Ghostty", "unknown"},
		{"windows_terminal", map[string]string{"WT_SESSION": "abc"}, "Windows Terminal", "unknown"},
		{"konsole", map[string]string{"KONSOLE_VERSION": "2204"}, "Konsole", "unknown"},
		{"gnome", map[string]string{"GNOME_TERMINAL_SCREEN": "/org/gnome/Terminal/screen/x"}, "GNOME Terminal", "unknown"},
		{"tmux", map[string]string{"TMUX": "/tmp/tmux-0/default,1,0", "TERM": "screen-256color"}, "tmux", "unknown"},
		{"screen", map[string]string{"TERM": "screen.xterm"}, "GNU screen", "unknown"},
		{"generic", map[string]string{"TERM": "xterm-256color"}, "generic (xterm-256color)", "unknown"},
		{"unknown", map[string]string{}, "unknown", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := commands.DetectTerminal(envFrom(tc.env))
			if info.Terminal != tc.terminal {
				t.Errorf("Terminal: want %q, got %q", tc.terminal, info.Terminal)
			}
			if info.Shell != tc.shell {
				t.Errorf("Shell: want %q, got %q", tc.shell, info.Shell)
			}
		})
	}
}

func TestFormatTerminalSetup_CompletionHint(t *testing.T) {
	out := commands.FormatTerminalSetup(commands.TerminalInfo{Terminal: "iTerm2", Shell: "zsh", Term: "xterm-256color"})
	for _, want := range []string{"Terminal setup", "iTerm2", "zsh", "xterm-256color", "glaude completion zsh", "Ctrl+J"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in report, got:\n%s", want, out)
		}
	}
	// pwsh maps to the cobra powershell completion target.
	out = commands.FormatTerminalSetup(commands.TerminalInfo{Terminal: "Windows Terminal", Shell: "pwsh"})
	if !strings.Contains(out, "glaude completion powershell") {
		t.Errorf("pwsh should map to powershell completion, got:\n%s", out)
	}
	// Unknown shell gets the generic hint, never a broken specific one.
	out = commands.FormatTerminalSetup(commands.TerminalInfo{Terminal: "unknown", Shell: "unknown"})
	if !strings.Contains(out, "bash|zsh|fish") {
		t.Errorf("unknown shell should get generic completion hint, got:\n%s", out)
	}
}

func TestConfigCommand_OpensDialog(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Lookup("config")
	if cmd == nil {
		t.Fatal("expected /config to be registered")
	}
	result := cmd.Execute(commands.CommandContext{}, "")
	if result.OpenDialog != "config" {
		t.Errorf("expected OpenDialog=config, got %q", result.OpenDialog)
	}
	if strings.Contains(result.Text, "not yet implemented") {
		t.Error("stub text must be gone")
	}
}

func TestTerminalSetupCommand_ReportsDetection(t *testing.T) {
	r := commands.NewRegistry()
	commands.RegisterBuiltins(r)
	cmd := r.Lookup("terminal-setup")
	if cmd == nil {
		t.Fatal("expected /terminal-setup to be registered")
	}
	result := cmd.Execute(commands.CommandContext{}, "")
	if result.Display != commands.DisplayMessage {
		t.Errorf("expected DisplayMessage, got %v", result.Display)
	}
	if !strings.Contains(result.Text, "Terminal setup") || !strings.Contains(result.Text, "Shell") {
		t.Errorf("expected detection report, got:\n%s", result.Text)
	}
	if strings.Contains(result.Text, "not yet implemented") {
		t.Error("stub text must be gone")
	}
}
