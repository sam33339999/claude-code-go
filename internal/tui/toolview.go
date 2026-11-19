package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tunsuy/glaude/pkg/types"
)

// Per-tool presentation for the message list. Each tool gets a compact header
// argument (`Bash(git status)`, `Read(internal/tui/view.go)`) and a summarised
// result (`⎿ Read 120 lines`, a coloured diff for Edit, …) instead of the
// generic `key=value` dump plus first-N-lines of raw output.
//
// This lives in the TUI layer on purpose: tools sit below the TUI in the
// dependency order, so they must not import lipgloss styles or Theme.

// resultPrefix starts the first line of a tool result; resultIndent aligns the
// following lines under it.
const (
	resultPrefix = "  ⎿  "
	resultIndent = "     "
)

// collapsedResultLines is how many raw output lines a collapsed generic result
// shows before the "+N lines (ctrl+o to expand)" hint.
const collapsedResultLines = 4

// collapsedDiffLines is how many diff lines a collapsed Edit result shows.
const collapsedDiffLines = 12

// maxDiffLCSCells bounds the O(n·m) line-diff table; larger edits fall back
// to "all removed, then all added" which is still correct, just less minimal.
const maxDiffLCSCells = 250_000

// inputString returns input[key] as a string ("" when absent or not a string).
func inputString(input map[string]any, key string) string {
	if v, ok := input[key].(string); ok {
		return v
	}
	return ""
}

// displayPath shows p relative to cwd when it lives under it, else as given.
func displayPath(p, cwd string) string {
	if p == "" || cwd == "" || !filepath.IsAbs(p) {
		return p
	}
	rel, err := filepath.Rel(cwd, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

// toolArgSummary returns the text shown inside `ToolName(…)` for a tool call.
// The result is a single line no wider than maxWidth cells; "" means the
// parentheses are omitted.
func toolArgSummary(name string, input map[string]any, cwd string, maxWidth int) string {
	if len(input) == 0 {
		return ""
	}
	var s string
	switch name {
	case "Bash":
		s = firstLine(inputString(input, "command"))
	case "Read":
		s = displayPath(inputString(input, "file_path"), cwd)
		if off, lim := intInput(input, "offset"), intInput(input, "limit"); lim > 0 {
			start := off
			if start < 1 {
				start = 1
			}
			s += fmt.Sprintf(" · lines %d-%d", start, start+lim-1)
		}
	case "Write", "Edit":
		s = displayPath(inputString(input, "file_path"), cwd)
	case "NotebookEdit":
		s = displayPath(inputString(input, "notebook_path"), cwd)
	case "Grep", "Glob":
		s = fmt.Sprintf("pattern: %q", inputString(input, "pattern"))
		if p := inputString(input, "path"); p != "" {
			s += ", path: " + displayPath(p, cwd)
		}
	case "WebFetch":
		s = inputString(input, "url")
	case "WebSearch":
		s = inputString(input, "query")
	case "Skill":
		s = inputString(input, "skill")
	case "Agent":
		s = inputString(input, "description")
		if s == "" {
			s = inputString(input, "agent_type")
		}
		if s == "" {
			s = firstLine(inputString(input, "prompt"))
		}
	case "TodoWrite":
		return "" // the todo panel shows the list itself
	default:
		s = summariseToolInput(input, maxWidth)
	}
	return truncateWidth(s, maxWidth)
}

// intInput returns input[key] as an int (JSON numbers decode as float64).
func intInput(input map[string]any, key string) int {
	switch v := input[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// toolResultText concatenates the text parts of a tool_result block and
// reports whether it carried non-text (e.g. image) content.
func toolResultText(blk types.ContentBlock) (text string, hasNonText bool) {
	var sb strings.Builder
	for _, inner := range blk.Content {
		if inner.Type == types.ContentTypeText && inner.Text != nil {
			sb.WriteString(*inner.Text)
		} else {
			hasNonText = true
		}
	}
	return sb.String(), hasNonText
}

// nonEmptyLines splits s into lines, dropping trailing newlines and blank lines.
func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// plural returns "1 line" / "3 lines" / "2 matches".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	if strings.HasSuffix(word, "ch") || strings.HasSuffix(word, "s") || strings.HasSuffix(word, "x") {
		return fmt.Sprintf("%d %ses", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// renderToolResult renders the result of a tool call, choosing a per-tool
// summary when one exists and falling back to the generic output preview.
func renderToolResult(name string, input map[string]any, result types.ContentBlock, expanded bool, opts renderOptions) string {
	text, hasNonText := toolResultText(result)
	isError := result.IsError != nil && *result.IsError
	if isError {
		return renderOutputLines(text, true, expanded, opts)
	}

	switch name {
	case "Read":
		if hasNonText && text == "" {
			return summaryLine("Read image", opts)
		}
		n := len(strings.Split(strings.TrimRight(text, "\n"), "\n"))
		if text == "" {
			n = 0
		}
		return summaryLine("Read "+plural(n, "line"), opts)
	case "Grep":
		return summaryLine(grepSummary(text, inputString(input, "output_mode")), opts)
	case "Glob":
		var files []string
		for _, l := range nonEmptyLines(text) {
			if !strings.HasPrefix(l, "[Results truncated") {
				files = append(files, l)
			}
		}
		if len(files) == 0 || strings.HasPrefix(files[0], "No files") {
			return summaryLine("No files found", opts)
		}
		return summaryLine("Found "+plural(len(files), "file"), opts)
	case "Edit":
		return renderEditDiff(input, expanded, opts)
	case "Write":
		return renderWriteResult(input, expanded, opts)
	case "TodoWrite":
		return summaryLine("Updated todos", opts)
	}
	return renderOutputLines(text, false, expanded, opts)
}

// grepSummary describes a Grep result without dumping every match.
func grepSummary(text, mode string) string {
	lines := nonEmptyLines(text)
	// Drop the trailer lines the tool appends.
	kept := lines[:0:0]
	for _, l := range lines {
		if strings.HasPrefix(l, "[Results truncated") || l == "No matches found." {
			continue
		}
		kept = append(kept, l)
	}
	if len(kept) == 0 {
		return "No matches found"
	}
	switch mode {
	case "files_with_matches":
		return "Found " + plural(len(kept), "file")
	case "count":
		return "Counted matches in " + plural(len(kept), "file")
	}
	return "Found " + plural(len(kept), "match")
}

// summaryLine renders a one-line muted result summary under a tool call.
func summaryLine(s string, opts renderOptions) string {
	return mutedStyle(opts.theme).Render(resultPrefix+truncateWidth(s, opts.width-displayWidth(resultPrefix))) + "\n"
}

// renderOutputLines renders raw tool output: the first collapsedResultLines
// lines (all when expanded), each cut to the terminal width, plus a hint.
func renderOutputLines(text string, isError, expanded bool, opts renderOptions) string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		if isError {
			return errorStyle(opts.theme).Render(resultPrefix+"Error") + "\n"
		}
		return mutedStyle(opts.theme).Render(resultPrefix+"(no output)") + "\n"
	}
	style := mutedStyle(opts.theme)
	if isError {
		style = errorStyle(opts.theme)
	}
	lines := strings.Split(text, "\n")
	limit := collapsedResultLines
	if expanded {
		limit = len(lines)
	}
	lineWidth := opts.width - displayWidth(resultIndent)
	var sb strings.Builder
	for i, line := range lines {
		if i >= limit {
			break
		}
		prefix := resultIndent
		if i == 0 {
			prefix = resultPrefix
		}
		// Tabs render with unpredictable width; expand so truncation is exact.
		line = strings.ReplaceAll(line, "\t", "    ")
		sb.WriteString(style.Render(prefix + truncateWidth(line, lineWidth)))
		sb.WriteString("\n")
	}
	if len(lines) > limit {
		sb.WriteString(mutedStyle(opts.theme).Render(
			fmt.Sprintf("%s… +%s (ctrl+o to expand)", resultIndent, plural(len(lines)-limit, "line"))))
		sb.WriteString("\n")
	}
	return sb.String()
}

// diffOp is one line of a line diff.
type diffOp struct {
	kind byte // ' ', '-', '+'
	text string
}

// lineDiff returns a minimal line diff of a → b using an LCS table. Inputs
// whose table would exceed maxDiffLCSCells degrade to "all removed, all added".
func lineDiff(a, b []string) []diffOp {
	n, m := len(a), len(b)
	if n*m > maxDiffLCSCells {
		ops := make([]diffOp, 0, n+m)
		for _, l := range a {
			ops = append(ops, diffOp{'-', l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{'+', l})
		}
		return ops
	}
	// lcs[i][j] = LCS length of a[i:] and b[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}

// splitLines splits s into lines ("" → no lines).
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// renderEditDiff renders an Edit result as a summary plus a coloured diff of
// old_string → new_string.
func renderEditDiff(input map[string]any, expanded bool, opts renderOptions) string {
	ops := lineDiff(splitLines(inputString(input, "old_string")), splitLines(inputString(input, "new_string")))
	added, removed := 0, 0
	for _, op := range ops {
		switch op.kind {
		case '+':
			added++
		case '-':
			removed++
		}
	}
	path := displayPath(inputString(input, "file_path"), opts.cwd)
	var sb strings.Builder
	sb.WriteString(summaryLine(fmt.Sprintf("Updated %s with %s and %s",
		path, plural(added, "addition"), plural(removed, "removal")), opts))

	limit := collapsedDiffLines
	if expanded {
		limit = len(ops)
	}
	lineWidth := opts.width - displayWidth(resultIndent) - 2
	for i, op := range ops {
		if i >= limit {
			break
		}
		line := truncateWidth(strings.ReplaceAll(op.text, "\t", "    "), lineWidth)
		switch op.kind {
		case '+':
			sb.WriteString(resultIndent + successStyle(opts.theme).Render("+ "+line))
		case '-':
			sb.WriteString(resultIndent + errorStyle(opts.theme).Render("- "+line))
		default:
			sb.WriteString(resultIndent + mutedStyle(opts.theme).Render("  "+line))
		}
		sb.WriteString("\n")
	}
	if len(ops) > limit {
		sb.WriteString(mutedStyle(opts.theme).Render(
			fmt.Sprintf("%s… +%s (ctrl+o to expand)", resultIndent, plural(len(ops)-limit, "line"))))
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderWriteResult renders a Write result as a summary plus a short preview
// of the written content.
func renderWriteResult(input map[string]any, expanded bool, opts renderOptions) string {
	lines := splitLines(inputString(input, "content"))
	path := displayPath(inputString(input, "file_path"), opts.cwd)
	var sb strings.Builder
	sb.WriteString(summaryLine(fmt.Sprintf("Wrote %s to %s", plural(len(lines), "line"), path), opts))
	limit := collapsedResultLines
	if expanded {
		limit = len(lines)
	}
	lineWidth := opts.width - displayWidth(resultIndent)
	for i, l := range lines {
		if i >= limit {
			break
		}
		sb.WriteString(mutedStyle(opts.theme).Render(resultIndent + truncateWidth(strings.ReplaceAll(l, "\t", "    "), lineWidth)))
		sb.WriteString("\n")
	}
	if len(lines) > limit {
		sb.WriteString(mutedStyle(opts.theme).Render(
			fmt.Sprintf("%s… +%s (ctrl+o to expand)", resultIndent, plural(len(lines)-limit, "line"))))
		sb.WriteString("\n")
	}
	return sb.String()
}

// summariseToolInput formats an arbitrary tool input as a one-line summary.
// Single command-like values are shown bare; otherwise `key=value` pairs in
// sorted key order (map iteration order is random, which made the header
// reshuffle on every re-render). The result fits in maxLen cells.
func summariseToolInput(input map[string]any, maxLen int) string {
	if len(input) == 0 {
		return ""
	}
	if len(input) == 1 {
		for k, v := range input {
			if k == "command" || k == "query" || k == "path" || k == "content" {
				return truncateWidth(firstLine(toString(v)), maxLen)
			}
		}
	}
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, truncateWidth(k+"="+firstLine(toString(input[k])), 30))
	}
	return truncateWidth(strings.Join(parts, ", "), maxLen)
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return itoa(int(t))
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return "<complex>"
	}
}
