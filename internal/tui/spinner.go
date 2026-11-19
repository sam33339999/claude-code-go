package tui

import (
	"strings"
	"time"
)

// SpinnerMode controls how the spinner verb is displayed.
type SpinnerMode int

const (
	SpinnerModeNormal   SpinnerMode = iota // single agent
	SpinnerModeBrief                       // brief / compact
	SpinnerModeTeammate                    // multi-agent
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerModel represents the loading spinner.
type SpinnerModel struct {
	frames  []string
	current int
	verb    string
	elapsed time.Duration
}

// newSpinner creates a default spinner.
func newSpinner() SpinnerModel {
	return SpinnerModel{
		frames: spinnerFrames,
		verb:   "Thinking",
	}
}

// Tick advances the spinner frame and accumulates elapsed time.
func (s SpinnerModel) Tick(d time.Duration) SpinnerModel {
	s.current = (s.current + 1) % len(s.frames)
	s.elapsed += d
	return s
}

// Reset resets the spinner to the initial state.
func (s SpinnerModel) Reset() SpinnerModel {
	s.current = 0
	s.elapsed = 0
	return s
}

// spinnerStatus is the live context the spinner line describes.
type spinnerStatus struct {
	tool         string // running tool name ("" = model is generating)
	outputTokens int    // output tokens so far this query (real or estimated)
}

// verbFor returns the spinner verb for the current activity.
func verbFor(tool string) string {
	switch tool {
	case "":
		return "Thinking"
	case "Bash":
		return "Running command"
	case "Read", "NotebookRead":
		return "Reading"
	case "Edit", "NotebookEdit":
		return "Editing"
	case "Write":
		return "Writing"
	case "Grep", "Glob", "ToolSearch":
		return "Searching"
	case "WebFetch":
		return "Fetching"
	case "WebSearch":
		return "Searching the web"
	case "Agent":
		return "Delegating"
	case "TodoWrite":
		return "Planning"
	default:
		return "Running " + tool
	}
}

// View renders the spinner line, e.g.
// "⠋ Reading… (12s · ↓ 1.2k tokens · esc to interrupt)".
func (s SpinnerModel) View(theme Theme) string {
	return s.ViewStatus(theme, spinnerStatus{})
}

// ViewStatus renders the spinner line for the given live status.
func (s SpinnerModel) ViewStatus(theme Theme, st spinnerStatus) string {
	frame := s.frames[s.current%len(s.frames)]
	verb := s.verb
	if st.tool != "" || verb == "" {
		verb = verbFor(st.tool)
	}
	details := []string{formatDuration(s.elapsed)}
	if st.outputTokens > 0 {
		details = append(details, "↓ "+compactCount(st.outputTokens)+" tokens")
	}
	details = append(details, "esc to interrupt")
	return accentStyle(theme).Render(frame) + " " + accentStyle(theme).Render(verb+"…") + " " +
		mutedStyle(theme).Render("("+strings.Join(details, " · ")+")")
}

// formatDuration returns a human-readable elapsed time string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return formatInt(secs) + "s"
	}
	mins := secs / 60
	secs = secs % 60
	return formatInt(mins) + "m" + formatInt(secs) + "s"
}

func formatInt(n int) string {
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
