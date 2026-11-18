package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tunsuy/claude-code-go/internal/codemode"
	"github.com/tunsuy/claude-code-go/internal/tools"
)

// errNoDataDir marks features disabled because the host configured no dataDir.
var errNoDataDir = errors.New("codemode: feature requires a data directory (host configuration)")

// maxNestedDepth bounds replay/run recursion (a replayed script may itself
// call codemode.replay/run; CM-03 v3).
const maxNestedDepth = 3

// findHistory scans the FULL history file for one execution ID (readHistory
// only returns the newest N entries; replay/audit must reach older ones).
func (t *codeModeTool) findHistory(id string) *historyEntry {
	if t.dataDir == "" || id == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(t.dataDir, "history.jsonl"))
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var e historyEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.ID == id {
			return &e
		}
	}
	return nil
}

// runNested executes replay/snippet code through the SAME engine + bridge,
// sharing the outer execution's approval scope and audit trail (CM-03 v3).
// The nested run gets its own history entry (replay_of / snippet provenance).
// It blocks the outer event loop while running; the outer ctx cancellation
// and timeout still apply.
func (t *codeModeTool) runNested(
	ctx context.Context, outer *tools.UseContext, st *execState,
	code string, ts bool, replayOf, snippet string,
) (any, error) {
	if st.depth.Add(1) > maxNestedDepth {
		st.depth.Add(-1)
		return nil, fmt.Errorf("codemode: nested execution depth exceeds %d (replay/run recursion)", maxNestedDepth)
	}
	defer st.depth.Add(-1)

	label := "snippet:" + snippet
	if replayOf != "" {
		label = "replay:" + replayOf
	}
	execID := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), t.callSeq.Add(1))
	start := time.Now()
	opts := t.sandboxOptions(ctx, outer, st)
	opts.Typescript = ts
	res, err := t.engine.Execute(ctx, code, t.bridge(ctx, outer, st), opts)
	duration := time.Since(start).Milliseconds()
	if err == nil && res != nil {
		st.appendNestedLogs(label, res.Logs)
	}
	t.recordHistory(historyEntry{
		ID:         execID,
		Timestamp:  start.UTC().Format(time.RFC3339),
		OK:         err == nil,
		DurationMS: duration,
		Typescript: ts,
		Code:       capString(code, historyCodeCap),
		Tools:      st.snapshotTools(),
		Preview:    capString(fmt.Sprintf("%v", nestedPreview(res)), 200),
		Error:      errorString(err),
		ReplayOf:   replayOf,
		Snippet:    snippet,
	})
	if err != nil {
		return nil, err
	}
	return res.Value, nil
}

func nestedPreview(res *codemode.Result) any {
	if res == nil {
		return nil
	}
	return res.Value
}

// sandboxOptions assembles the full hook set for one execution (outer or
// nested — nested runs share st, so approvals/audit stay execution-wide).
// Persistence-backed hooks are only installed when dataDir is configured.
func (t *codeModeTool) sandboxOptions(ctx context.Context, outer *tools.UseContext, st *execState) codemode.Options {
	opts := codemode.Options{
		Search: func(query string) (any, error) {
			return codemode.Search(t.catalog, query), nil
		},
		Describe: func(name string) (any, error) {
			e, ok := codemode.Describe(t.catalog, name)
			if !ok {
				return nil, fmt.Errorf("unknown tool %q", name)
			}
			return e, nil
		},
		History: func(limit int) (any, error) {
			return t.readHistory(limit), nil
		},
	}
	if t.dataDir == "" {
		return opts
	}
	opts.ImportModule = t.resolveModule
	opts.Replay = func(id string) (any, error) {
		e := t.findHistory(id)
		if e == nil {
			return nil, fmt.Errorf("codemode: no execution %q in history", id)
		}
		return t.runNested(ctx, outer, st, e.Code, e.Typescript, id, "")
	}
	opts.Run = func(name string) (any, error) {
		src, err := t.readSnippet(name)
		if err != nil {
			return nil, err
		}
		return t.runNested(ctx, outer, st, src, false, "", name)
	}
	opts.Snippets = func() (any, error) { return t.listSnippets() }
	opts.Audit = func(id string) (any, error) {
		e := t.findHistory(id)
		if e == nil {
			return nil, fmt.Errorf("codemode: no execution %q in history", id)
		}
		return map[string]any{"id": e.ID, "side_effects": e.SideEffects}, nil
	}
	opts.Approvals = func() (any, error) { return t.loadApprovals(), nil }
	opts.Revoke = func(name string) (any, error) { return t.revokeApproval(name) }
	return opts
}
