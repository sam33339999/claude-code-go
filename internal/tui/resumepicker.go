package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tunsuy/glaude/internal/commands"
)

// resumePicker is the interactive /resume session list: ↑/↓ to choose,
// Enter to resume, Esc to cancel. It replaces copying a UUID out of a text
// listing into "/resume <id>".
type resumePicker struct {
	sessions []commands.SessionSummary
	cursor   int
	// now is injectable for tests (relative times).
	now func() time.Time
}

func newResumePicker(sessions []commands.SessionSummary) *resumePicker {
	p := &resumePicker{sessions: sessions, now: time.Now}
	// Start on the newest session that isn't the one already running.
	for i, s := range sessions {
		if !s.Current {
			p.cursor = i
			break
		}
	}
	return p
}

func (p *resumePicker) move(delta int) {
	if len(p.sessions) == 0 {
		return
	}
	p.cursor = (p.cursor + delta + len(p.sessions)) % len(p.sessions)
}

func (p *resumePicker) selected() (commands.SessionSummary, bool) {
	if p == nil || p.cursor < 0 || p.cursor >= len(p.sessions) {
		return commands.SessionSummary{}, false
	}
	return p.sessions[p.cursor], true
}

// relativeTime formats t relative to now: "just now", "5m ago", "3h ago",
// "yesterday", "4d ago", then a date.
func relativeTime(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2 2006")
	}
}

// shortID returns the first 8 characters of a session ID.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// view renders the picker as a bordered dialog no taller than maxHeight lines
// and exactly width cells wide. Rows scroll to keep the cursor visible; on
// short terminals the id and key-hint lines are dropped first.
func (p *resumePicker) view(width, maxHeight int, theme Theme) string {
	inner := width - 4 // border + padding
	if inner < 10 {
		inner = 10
	}
	fit := func(s string) string { return truncateWidth(s, inner) }

	// Chrome: border(2) + title(1) + blank(1) + blank(1) + id(1) + hint(1),
	// plus up to two "N more" lines when scrolled.
	full := len(p.sessions) > 0 && maxHeight-9 >= 3
	chrome := 9
	if !full {
		chrome = 5 // border(2) + title(1) + "more" lines(2)
	}
	rows := maxHeight - chrome
	if rows < 1 {
		rows = 1
	}

	var sb strings.Builder
	title := warningStyle(theme).Bold(true).Render("Resume a session")
	count := mutedStyle(theme).Render(plural(len(p.sessions), "session"))
	gap := inner - displayWidth(title) - displayWidth(count)
	if gap < 1 {
		sb.WriteString(fit(title))
	} else {
		sb.WriteString(title + strings.Repeat(" ", gap) + count)
	}
	sb.WriteString("\n")
	if full {
		sb.WriteString("\n")
	}

	start := 0
	if p.cursor >= rows {
		start = p.cursor - rows + 1
	}
	end := start + rows
	if end > len(p.sessions) {
		end = len(p.sessions)
	}
	if start > 0 {
		sb.WriteString(mutedStyle(theme).Render(fit(fmt.Sprintf("  ↑ %d more", start))) + "\n")
	}

	now := p.now()
	for i := start; i < end; i++ {
		s := p.sessions[i]
		meta := fmt.Sprintf("%s · %s", relativeTime(s.ModTime, now), plural(s.MessageCount, "msg"))
		if s.Current {
			meta = "current · " + meta
		}
		// Keep at least 8 cells of title; the meta yields first when narrow.
		meta = truncateWidth(meta, inner-2-8-1)
		titleWidth := inner - 2 - displayWidth(meta) - 1
		label := truncateWidth(s.Title, titleWidth)
		pad := inner - 2 - displayWidth(label) - displayWidth(meta)
		if pad < 1 {
			pad = 1
		}
		if i == p.cursor {
			sb.WriteString(primaryStyle(theme).Bold(true).Render("❯ "+label) +
				strings.Repeat(" ", pad) + primaryStyle(theme).Render(meta))
		} else {
			style := mutedStyle(theme)
			if s.Current {
				style = style.Italic(true)
			}
			sb.WriteString("  " + label + strings.Repeat(" ", pad) + style.Render(meta))
		}
		sb.WriteString("\n")
	}
	if end < len(p.sessions) {
		sb.WriteString(mutedStyle(theme).Render(fit(fmt.Sprintf("  ↓ %d more", len(p.sessions)-end))) + "\n")
	}

	if full {
		sb.WriteString("\n")
		if s, ok := p.selected(); ok {
			sb.WriteString(mutedStyle(theme).Render(fit("id " + s.ID)))
			sb.WriteString("\n")
		}
		sb.WriteString(mutedStyle(theme).Render(fit("↑/↓ select · Enter resume · Esc cancel")))
	}
	return dialogBox(strings.TrimRight(sb.String(), "\n"), width, theme)
}

// resumePickerMaxHeight is the dialog's line budget: the terminal minus the
// status bar and one viewport line.
func (m AppModel) resumePickerMaxHeight() int {
	h := m.termHeight - 2
	if h > 22 {
		h = 22 // ~10 rows; a taller list stops being scannable
	}
	return h
}

// handleResumeKey handles keys while the /resume picker is open.
func (m AppModel) handleResumeKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	p := m.resumePicker
	if p == nil {
		m.activeDialog = dialogNone
		return m, nil
	}
	switch key.String() {
	case "up", "k", "shift+tab":
		p.move(-1)
		return m, nil
	case "down", "j", "tab":
		p.move(1)
		return m, nil
	case "esc", "q", "ctrl+g":
		m.activeDialog = dialogNone
		m.resumePicker = nil
		return m, m.drainDeferredQueue()
	case "enter", "ctrl+j":
		s, ok := p.selected()
		m.activeDialog = dialogNone
		m.resumePicker = nil
		if !ok {
			return m, m.drainDeferredQueue()
		}
		if s.Current {
			m.messages = append(m.messages, newSystemMessage("Already in this session."))
			m.syncViewportContent()
			return m, m.drainDeferredQueue()
		}
		// Resume first so queued work (e.g. agent notifications) runs in the
		// resumed conversation.
		m, cmd := m.handleSlashCommand("/resume " + s.ID)
		return m, tea.Batch(cmd, m.drainDeferredQueue())
	}
	return m, nil
}

// drainDeferredQueue processes queued commands that were held back while the
// picker was open (between-turn drain and agent wake-ups skip it).
func (m *AppModel) drainDeferredQueue() tea.Cmd {
	if m.isLoading || m.msgQueue == nil || m.msgQueue.Len() == 0 {
		return nil
	}
	return processQueueCmd(m)
}
