package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/tunsuy/glaude/internal/tools/shell"
)

// bashContextLimit caps each stream (stdout/stderr) of a ! command, in runes,
// before it is attached to the next user message.
const bashContextLimit = 30_000

// bashResultMsg carries a finished ! command back to Update. gen ties it to
// the run that started it so results outliving /clear are dropped.
type bashResultMsg struct {
	gen     uint64
	command string
	res     shell.RunResult
	err     error
}

// parseBang reports whether text is a ! command and returns the trimmed
// command after the bang.
func parseBang(text string) (string, bool) {
	if !strings.HasPrefix(text, "!") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(text, "!")), true
}

// truncateRunes keeps at most limit runes of s and reports whether it cut.
func truncateRunes(s string, limit int) (string, bool) {
	n := 0
	for i := range s {
		if n == limit {
			return s[:i], true
		}
		n++
	}
	return s, false
}

// formatBashContext renders a ! command and its output for the model.
func formatBashContext(command string, res shell.RunResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "<bash-input>%s</bash-input>\n", command)
	writeStream := func(tag, s string) {
		kept, cut := truncateRunes(s, bashContextLimit)
		fmt.Fprintf(&sb, "<%s>%s", tag, kept)
		if cut {
			fmt.Fprintf(&sb, "\n[output truncated to %d characters]", bashContextLimit)
		}
		fmt.Fprintf(&sb, "</%s>\n", tag)
	}
	writeStream("bash-stdout", res.Stdout)
	if res.Stderr != "" {
		writeStream("bash-stderr", res.Stderr)
	}
	if res.TimedOut {
		sb.WriteString("[command timed out]\n")
	} else if res.ExitCode != 0 {
		fmt.Fprintf(&sb, "[exit code %d]\n", res.ExitCode)
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

// codeFence wraps s in a fence longer than any backtick run inside it, so
// command output is shown verbatim instead of rendered as markdown.
func codeFence(s string) string {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + "\n" + strings.TrimRight(s, "\n") + "\n" + fence
}

// formatBashDisplay renders a finished ! command for the message list.
func formatBashDisplay(res shell.RunResult, err error) string {
	switch {
	case res.Canceled:
		return "command canceled"
	case err != nil:
		return "Error: " + err.Error()
	}
	var parts []string
	out := strings.TrimRight(res.Stdout, "\n")
	if res.Stderr != "" {
		out = strings.TrimRight(out+"\n"+res.Stderr, "\n")
	}
	if out != "" {
		parts = append(parts, codeFence(out))
	}
	if res.TimedOut {
		parts = append(parts, "command timed out")
	} else if res.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit code %d", res.ExitCode))
	}
	if len(parts) == 0 {
		return "(no output)"
	}
	return strings.Join(parts, "\n\n")
}

// startBash runs command in the session working directory without going
// through the model or the permission chain; it runs even while the main
// loop is busy.
func (m AppModel) startBash(command string) (AppModel, tea.Cmd) {
	m = m.cancelBash()
	m.bashGen++
	gen := m.bashGen
	ctx, cancel := context.WithCancel(context.Background())
	m.bashCancel = cancel

	m.messages = append(m.messages, newUserMessage("! "+command))
	m.syncViewportContent()

	dir := ""
	if m.appState != nil {
		dir = m.appState.GetState().WorkingDir
	}
	timeout := time.Duration(shell.DefaultBashTimeoutMs) * time.Millisecond
	return m, func() tea.Msg {
		res, err := shell.Run(ctx, dir, command, timeout)
		return bashResultMsg{gen: gen, command: command, res: res, err: err}
	}
}

// onBashResult shows a finished command and queues it for the next message.
func (m AppModel) onBashResult(msg bashResultMsg) (AppModel, tea.Cmd) {
	if msg.gen != m.bashGen {
		return m, nil
	}
	if m.bashCancel != nil {
		m.bashCancel()
		m.bashCancel = nil
	}
	m.messages = append(m.messages, newSystemMessage(formatBashDisplay(msg.res, msg.err)))
	if !msg.res.Canceled && msg.err == nil {
		m.bashPending = append(m.bashPending, formatBashContext(msg.command, msg.res))
	}
	m.syncViewportContent()
	return m, nil
}

// cancelBash stops a running ! command; its canceled result is still shown.
func (m AppModel) cancelBash() AppModel {
	if m.bashCancel != nil {
		m.bashCancel()
		m.bashCancel = nil
	}
	return m
}

// takeBashPending prefixes userText with the pending shell outputs and
// clears them.
func (m *AppModel) takeBashPending(userText string) string {
	if len(m.bashPending) == 0 {
		return userText
	}
	out := strings.Join(m.bashPending, "\n\n") + "\n\n" + userText
	m.bashPending = nil
	return out
}

// bashPendingHint is shown above the input while shell outputs wait to be
// attached.
func (m AppModel) bashPendingHint() string {
	n := len(m.bashPending)
	if n == 0 {
		return ""
	}
	noun := "output"
	if n > 1 {
		noun = "outputs"
	}
	return mutedStyle(m.theme).Render(fmt.Sprintf(
		"%d shell %s will be attached to your next message · Esc to discard", n, noun))
}
