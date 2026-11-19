package tui

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/tunsuy/glaude/internal/commands"
)

// --- Completion popup (slash commands and @file mentions) ---

type completionKind int

const (
	completionNone completionKind = iota
	completionSlash
	completionFile
)

// maxCompletionItems caps the popup height.
const maxCompletionItems = 6

type completionItem struct {
	value string // text inserted on accept (without the leading / or @)
	desc  string // optional description (slash commands)
}

// completionState is the popup shown under the input box while typing a
// /command or an @path. It is recomputed from the input after every edit.
type completionState struct {
	kind     completionKind
	token    string // the partial being completed, including its / or @
	items    []completionItem
	selected int
	// navigated is true once the user moved the selection with ↑/↓.
	navigated bool
	// dismissedFor suppresses the popup (after Esc) until the input changes.
	dismissedFor string
}

// visible reports whether the popup has anything to show.
func (c completionState) visible() bool {
	return c.kind != completionNone && len(c.items) > 0
}

// move shifts the selection by delta, wrapping around.
func (c completionState) move(delta int) completionState {
	if len(c.items) == 0 {
		return c
	}
	c.selected = (c.selected + delta + len(c.items)) % len(c.items)
	c.navigated = true
	return c
}

// trailingToken returns the whitespace-delimited token at the end of value
// ("" when value ends with whitespace).
func trailingToken(value string) string {
	if value == "" || strings.ContainsAny(value[len(value)-1:], " \t\n") {
		return ""
	}
	if i := strings.LastIndexAny(value, " \t\n"); i >= 0 {
		return value[i+1:]
	}
	return value
}

// computeCompletion derives the popup state for the current input value.
// cursorAtEnd must be true for @-completion (we only rewrite the tail).
func computeCompletion(prev completionState, value string, cursorAtEnd bool, reg *commands.Registry, files *fileIndex) completionState {
	next := completionState{dismissedFor: prev.dismissedFor}
	if prev.dismissedFor != "" && prev.dismissedFor != value {
		next.dismissedFor = ""
	}
	if next.dismissedFor != "" {
		return next
	}

	// /command: the whole input is a single slash token.
	if strings.HasPrefix(value, "/") && !strings.ContainsAny(value, " \t\n") && reg != nil {
		for _, c := range reg.CompletePrefix(value) {
			next.items = append(next.items, completionItem{value: c.Name, desc: c.Description})
		}
		// Hide when the input is already exactly the only match.
		if len(next.items) == 1 && "/"+next.items[0].value == value {
			next.items = nil
		}
		if len(next.items) > 0 {
			next.kind = completionSlash
			next.token = value
		}
	} else if tok := trailingToken(value); cursorAtEnd && strings.HasPrefix(tok, "@") && files != nil {
		for _, p := range files.match(tok[1:], maxCompletionItems) {
			next.items = append(next.items, completionItem{value: p})
		}
		// Hide when the mention already names the only match exactly.
		if len(next.items) == 1 && "@"+next.items[0].value == tok {
			next.items = nil
		}
		if len(next.items) > 0 {
			next.kind = completionFile
			next.token = tok
		}
	}
	// Keep the selection on the same item when the list is unchanged.
	if next.kind == prev.kind && next.token != "" && len(next.items) == len(prev.items) {
		next.selected = prev.selected
		next.navigated = prev.navigated
	}
	if next.selected >= len(next.items) {
		next.selected = 0
	}
	return next
}

// applyCompletion returns value with the selected completion inserted.
func applyCompletion(c completionState, value string) string {
	if !c.visible() {
		return value
	}
	item := c.items[c.selected]
	switch c.kind {
	case completionSlash:
		return "/" + item.value + " "
	case completionFile:
		return strings.TrimSuffix(value, c.token) + "@" + item.value + " "
	}
	return value
}

// renderCompletion renders the popup lines (selected item highlighted).
func renderCompletion(c completionState, width int, theme Theme) string {
	if !c.visible() {
		return ""
	}
	items := c.items
	start := 0
	if len(items) > maxCompletionItems {
		// Scroll the window so the selection stays visible.
		if c.selected >= maxCompletionItems {
			start = c.selected - maxCompletionItems + 1
		}
		items = items[start : start+maxCompletionItems]
	}
	nameWidth := 0
	for _, it := range items {
		if w := displayWidth(it.value) + 1; w > nameWidth {
			nameWidth = w
		}
	}
	var sb strings.Builder
	for i, it := range items {
		prefix := "/"
		if c.kind == completionFile {
			prefix = "@"
		}
		name := prefix + it.value
		line := "  " + name
		if it.desc != "" {
			line += strings.Repeat(" ", nameWidth-displayWidth(it.value)+2) + it.desc
		}
		line = truncateWidth(line, width)
		if start+i == c.selected {
			sb.WriteString(primaryStyle(theme).Bold(true).Render(line))
		} else {
			sb.WriteString(mutedStyle(theme).Render(line))
		}
		sb.WriteString("\n")
	}
	if len(c.items) > maxCompletionItems {
		sb.WriteString(mutedStyle(theme).Render("  … " + itoa(len(c.items)) + " matches"))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// --- Project file index for @-mentions ---

// fileIndexMaxEntries caps the walk (files and directories) so huge trees
// don't take long even in the background.
const fileIndexMaxEntries = 50_000

// fileIndexTTL is how long a built index is reused before re-walking.
const fileIndexTTL = 30 * time.Second

// skippedDirs are never descended into.
var skippedDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".glaude": true,
	".idea": true, ".vscode": true, "dist": true, "build": true, "bin": true,
	"__pycache__": true, ".venv": true, "target": true,
}

// fileIndex lists files under root (relative paths, forward slashes) for
// @-mention completion. The walk runs in a tea.Cmd (never on the Update
// loop); results arrive as fileIndexReadyMsg. Until then match returns
// nothing. All fields are touched only from Update.
type fileIndex struct {
	root     string
	files    []string
	builtAt  time.Time
	building bool
	// now is injectable for tests.
	now func() time.Time
}

// fileIndexReadyMsg carries a finished background walk.
type fileIndexReadyMsg struct {
	root  string
	files []string
	at    time.Time
}

func newFileIndex(root string) *fileIndex {
	return &fileIndex{root: root, now: time.Now}
}

// refreshCmd returns a Cmd that (re)builds the index for root when it is
// missing, stale, or for a different root; nil when fresh or already building.
func (fi *fileIndex) refreshCmd(root string) tea.Cmd {
	if root == "" {
		root = fi.root
	}
	if root == "" || fi.building {
		return nil
	}
	if root == fi.root && fi.files != nil && fi.now().Sub(fi.builtAt) < fileIndexTTL {
		return nil
	}
	fi.building = true
	now := fi.now
	return func() tea.Msg {
		return fileIndexReadyMsg{root: root, files: walkProjectFiles(root), at: now()}
	}
}

// adopt installs a finished walk.
func (fi *fileIndex) adopt(msg fileIndexReadyMsg) {
	fi.building = false
	fi.root = msg.root
	fi.files = msg.files
	fi.builtAt = msg.at
}

// walkProjectFiles lists files under root, skipping hidden and vendor dirs.
func walkProjectFiles(root string) []string {
	files := []string{}
	entries := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		entries++
		if entries > fileIndexMaxEntries {
			return fs.SkipAll
		}
		if d.IsDir() {
			if path != root && (skippedDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if rel, rerr := filepath.Rel(root, path); rerr == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// match returns up to limit files matching query: path-prefix matches first,
// then base-name prefix, then substring; each group shortest-first.
func (fi *fileIndex) match(query string, limit int) []string {
	if fi == nil {
		return nil
	}
	q := strings.ToLower(query)
	var prefix, base, sub []string
	for _, f := range fi.files {
		lf := strings.ToLower(f)
		switch {
		case strings.HasPrefix(lf, q):
			prefix = append(prefix, f)
		case strings.HasPrefix(strings.ToLower(filepath.Base(f)), q):
			base = append(base, f)
		case q != "" && strings.Contains(lf, q):
			sub = append(sub, f)
		}
	}
	byLen := func(s []string) {
		sort.SliceStable(s, func(i, j int) bool { return len(s[i]) < len(s[j]) })
	}
	byLen(prefix)
	byLen(base)
	byLen(sub)
	out := append(append(prefix, base...), sub...)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// --- Prompt history (↑/↓ on the first/last input line) ---

// maxHistoryEntries bounds the in-memory history.
const maxHistoryEntries = 200

// inputHistory holds submitted prompts for recall with ↑/↓.
type inputHistory struct {
	entries []string
	// idx is the entry being shown; len(entries) means "not browsing".
	idx   int
	draft string // the unsent text saved when browsing starts
}

// add records a submitted prompt (deduplicating consecutive repeats) and
// resets browsing.
func (h inputHistory) add(s string) inputHistory {
	if strings.TrimSpace(s) != "" && (len(h.entries) == 0 || h.entries[len(h.entries)-1] != s) {
		h.entries = append(h.entries, s)
		if len(h.entries) > maxHistoryEntries {
			h.entries = h.entries[len(h.entries)-maxHistoryEntries:]
		}
	}
	h.idx = len(h.entries)
	h.draft = ""
	return h
}

// browsing reports whether a history entry is currently shown.
func (h inputHistory) browsing() bool {
	return h.idx < len(h.entries)
}

// prev moves to the older entry. ok is false when there is nothing older.
func (h inputHistory) prev(current string) (inputHistory, string, bool) {
	if h.idx == 0 || len(h.entries) == 0 {
		return h, current, false
	}
	if !h.browsing() {
		h.draft = current
	}
	h.idx--
	return h, h.entries[h.idx], true
}

// next moves to the newer entry, restoring the draft past the newest.
func (h inputHistory) next() (inputHistory, string, bool) {
	if !h.browsing() {
		return h, "", false
	}
	h.idx++
	if h.idx == len(h.entries) {
		return h, h.draft, true
	}
	return h, h.entries[h.idx], true
}
