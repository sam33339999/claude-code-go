package commands

import (
	"fmt"
	"path/filepath"
	"strings"
)

// TerminalInfo describes the detected terminal environment for /terminal-setup.
type TerminalInfo struct {
	// Terminal is a human-readable terminal emulator name.
	Terminal string
	// Shell is the base name of the user's shell (bash, zsh, fish, ...).
	Shell string
	// Term is the raw TERM environment value (may be empty).
	Term string
}

// DetectTerminal identifies the terminal emulator and shell from environment
// variables. lookup is injected (os.Getenv in production) so tests stay pure
// and never mutate process environment. Detection is best-effort and honest:
// unknown environments are reported as such, never guessed.
func DetectTerminal(lookup func(string) string) TerminalInfo {
	info := TerminalInfo{Term: lookup("TERM")}
	switch {
	case lookup("TERM_PROGRAM") == "vscode":
		info.Terminal = "VS Code integrated terminal"
	case lookup("TERM_PROGRAM") == "iTerm.app" || lookup("ITERM_SESSION_ID") != "":
		info.Terminal = "iTerm2"
	case lookup("TERM_PROGRAM") == "Apple_Terminal":
		info.Terminal = "Apple Terminal"
	case lookup("TERM_PROGRAM") == "WezTerm":
		info.Terminal = "WezTerm"
	case lookup("TERM_PROGRAM") == "ghostty":
		info.Terminal = "Ghostty"
	case lookup("WT_SESSION") != "":
		info.Terminal = "Windows Terminal"
	case lookup("KONSOLE_VERSION") != "":
		info.Terminal = "Konsole"
	case lookup("GNOME_TERMINAL_SCREEN") != "":
		info.Terminal = "GNOME Terminal"
	case lookup("TMUX") != "":
		info.Terminal = "tmux"
	case strings.Contains(info.Term, "screen"):
		info.Terminal = "GNU screen"
	case info.Term != "":
		info.Terminal = "generic (" + info.Term + ")"
	default:
		info.Terminal = "unknown"
	}
	if sh := lookup("SHELL"); sh != "" {
		info.Shell = filepath.Base(sh)
	} else {
		info.Shell = "unknown"
	}
	return info
}

// FormatTerminalSetup renders the detection report plus non-invasive setup
// guidance. The command writes no files and changes no terminal settings;
// it reports what it detected and points at the sanctioned setup paths
// (design/tui.md §4.1: 终端环境配置检测).
func FormatTerminalSetup(info TerminalInfo) string {
	var sb strings.Builder
	sb.WriteString("Terminal setup\n\n")
	sb.WriteString(fmt.Sprintf("  Terminal : %s\n", info.Terminal))
	sb.WriteString(fmt.Sprintf("  Shell    : %s\n", info.Shell))
	if info.Term != "" {
		sb.WriteString(fmt.Sprintf("  TERM     : %s\n", info.Term))
	}
	sb.WriteString("\nMulti-line input: if Shift+Enter is not mapped by your terminal, use Ctrl+J (LF).\n")
	shell := info.Shell
	if shell == "pwsh" {
		shell = "powershell"
	}
	switch shell {
	case "bash", "zsh", "fish", "powershell":
		sb.WriteString(fmt.Sprintf("Shell completions: run `glaude completion %s` and follow the printed instructions.\n", shell))
	default:
		sb.WriteString("Shell completions: run `glaude completion bash|zsh|fish` and follow the printed instructions.\n")
	}
	return sb.String()
}
