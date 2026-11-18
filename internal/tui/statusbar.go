package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// TokenUsage holds token counts for the current session.
type TokenUsage struct {
	Input        int
	Output       int
	CacheRead    int
	CacheCreated int
}

// StatusBar renders the top status line.
type StatusBar struct {
	model       string
	cwd         string
	effort      string // low, medium, high
	tokenUsage  TokenUsage
	cost        float64
	coordinator bool
	// mode is the live permission mode (BF-09/BF-21); rendered as a visible
	// indicator so the user always knows the current mode (YOLO warned).
	mode types.PermissionMode
}

// View renders the status bar to a string.
func (s StatusBar) View(width int, theme Theme) string {
	modelStr := s.model
	if modelStr == "" {
		modelStr = "claude"
	}

	effortStr := s.effort
	if effortStr == "" {
		effortStr = "medium"
	}

	left := lipgloss.JoinHorizontal(
		lipgloss.Left,
		primaryStyle(theme).Render(modelStr),
		mutedStyle(theme).Render(" | "),
		mutedStyle(theme).Render(truncateCWD(s.cwd, 40)),
	)
	if s.coordinator {
		left = left + mutedStyle(theme).Render(" | ") +
			accentStyle(theme).Render("Coordinator")
	}
	// Permission mode indicator (BF-09/BF-21): always visible when not normal.
	switch s.mode {
	case types.PermissionModeAcceptEdits:
		left += mutedStyle(theme).Render(" | ") + accentStyle(theme).Render("⏵⏵ accept edits")
	case types.PermissionModePlan:
		left += mutedStyle(theme).Render(" | ") + accentStyle(theme).Render("⏸ plan")
	case types.PermissionModeBypassPermissions:
		left += mutedStyle(theme).Render(" | ") + warningStyle(theme).Bold(true).Render("⚠ YOLO")
	}

	// Show effort level with icon
	effortIcon := map[string]string{
		"low":    "⚡",
		"medium": "⚖️",
		"high":   "🧠",
	}
	icon := effortIcon[effortStr]
	if icon == "" {
		icon = "⚖️"
	}

	right := lipgloss.JoinHorizontal(
		lipgloss.Left,
		mutedStyle(theme).Render(icon+" "+effortStr+" | "),
		mutedStyle(theme).Render(fmt.Sprintf(
			"%s tok · $%.4f",
			formatTokens(s.tokenUsage),
			s.cost,
		)),
	)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	return left + strings.Repeat(" ", gap) + right
}

// formatTokens returns a compact token count string.
func formatTokens(u TokenUsage) string {
	total := u.Input + u.Output
	if total >= 1000 {
		return fmt.Sprintf("%.1fk", float64(total)/1000)
	}
	return fmt.Sprintf("%d", total)
}

// truncateCWD shortens a directory path to at most maxLen characters.
func truncateCWD(cwd string, maxLen int) string {
	if len(cwd) <= maxLen {
		return cwd
	}
	// Keep the last maxLen characters preceded by "…"
	return "…" + cwd[len(cwd)-maxLen+1:]
}
