package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tunsuy/agtkeel/session"
	"github.com/tunsuy/agtkeel/types"
	utilfs "github.com/tunsuy/agtkeel/utils/fs"
)

const (
	// resumeListLimit caps the number of sessions shown by a bare /resume.
	resumeListLimit = 10
	// resumeSnippetLen is the maximum rune length of the first-message snippet.
	resumeSnippetLen = 60
)

// sessionInfo is one row of the /resume session listing.
type sessionInfo struct {
	id           string
	title        string // first user message snippet
	modTime      time.Time
	messageCount int
}

// listSessions discovers the most recently modified JSONL sessions for the
// project rooted at cwd, newest first, up to limit entries. A missing session
// directory yields an empty list (not an error). Unreadable individual session
// files are skipped.
func listSessions(cwd string, limit int) ([]sessionInfo, error) {
	dir, err := sessionDir(cwd)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session directory: %w", err)
	}

	type candidate struct {
		id      string
		path    string
		modTime time.Time
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), ".jsonl")
		candidates = append(candidates, candidate{
			id:      stem,
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime(),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	sessions := make([]sessionInfo, 0, len(candidates))
	for _, c := range candidates {
		info := sessionInfo{id: c.id, modTime: c.modTime, title: "(unreadable)"}
		store, err := session.OpenSessionStore(c.path)
		if err != nil {
			sessions = append(sessions, info)
			continue
		}
		envelopes, err := store.ReadAll()
		closeErr := store.Close()
		if err != nil || closeErr != nil {
			sessions = append(sessions, info)
			continue
		}
		msgs := extractSessionMessages(envelopes)
		info.messageCount = len(msgs)
		info.title = sessionTitle(msgs)
		sessions = append(sessions, info)
	}
	return sessions, nil
}

// loadSessionMessages reopens the session identified by sessionID under cwd
// and returns its ordered conversation messages (user + assistant roles).
// Corrupt or unknown entry types are skipped, mirroring the CLI --resume path.
// Unlike the CLI path, the underlying resume error is preserved via %w for
// diagnostics, and a Close failure is surfaced (same pattern as listSessions).
func loadSessionMessages(cwd, sessionID string) ([]types.Message, error) {
	if !validSessionID(sessionID) {
		return nil, fmt.Errorf("invalid session ID %q", sessionID)
	}
	_, mgr, err := session.Resume(sessionID, cwd)
	if err != nil {
		return nil, fmt.Errorf("session %q not found for this project: %w\n"+
			"Tip: use /resume to list available sessions", sessionID, err)
	}

	entries, err := mgr.ReadAll()
	closeErr := mgr.Close()
	if err != nil {
		return nil, fmt.Errorf("resume session %q: %w", sessionID, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close session %q: %w", sessionID, closeErr)
	}
	return extractSessionMessages(entries), nil
}

// validSessionID rejects IDs that could escape the session directory when
// joined into a file path (types.AsSessionId performs no validation).
func validSessionID(id string) bool {
	return id != "" && !strings.ContainsAny(id, "/\\") && !strings.Contains(id, "..")
}

// sessionDir returns the JSONL session directory for the project rooted at cwd:
// <cwd>/.claude/projects/<projectHash> (same layout as the CLI --resume path).
func sessionDir(cwd string) (string, error) {
	hash, err := utilfs.ProjectHash(cwd)
	if err != nil {
		return "", fmt.Errorf("compute project hash: %w", err)
	}
	return filepath.Join(cwd, ".claude", "projects", hash), nil
}

// extractSessionMessages decodes user/assistant entries into messages,
// silently skipping corrupt or unrelated entry types.
func extractSessionMessages(entries []types.EntryEnvelope) []types.Message {
	var msgs []types.Message
	for _, env := range entries {
		if env.Type != types.EntryTypeUser && env.Type != types.EntryTypeAssistant {
			continue
		}
		var sm types.SerializedMessage
		if err := json.Unmarshal(env.Raw, &sm); err != nil {
			continue
		}
		msgs = append(msgs, sm.Message)
	}
	return msgs
}

// sessionTitle derives a display title from the first user message text.
func sessionTitle(msgs []types.Message) string {
	for _, msg := range msgs {
		if msg.Role != types.RoleUser {
			continue
		}
		for _, block := range msg.Content {
			if block.Type == types.ContentTypeText && block.Text != nil && *block.Text != "" {
				return snippet(*block.Text, resumeSnippetLen)
			}
		}
	}
	if len(msgs) == 0 {
		return "(no messages)"
	}
	return "(untitled)"
}

// snippet collapses whitespace and truncates s to maxRunes runes.
func snippet(s string, maxRunes int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// formatSessionList renders the bare-/resume listing with usage guidance.
func formatSessionList(sessions []sessionInfo) string {
	var sb strings.Builder
	sb.WriteString("Recent sessions:\n\n")
	for _, s := range sessions {
		sb.WriteString(fmt.Sprintf("  %s  %s  %3d msgs  %s\n",
			s.id, s.modTime.Format("2006-01-02 15:04"), s.messageCount, s.title))
	}
	sb.WriteString("\nResume a session with /resume <id>")
	return sb.String()
}
