package codemode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// approvalsDoc is the on-disk shape of dataDir/approvals.json (CM-03 v3):
// CodeMode-scoped persistent tool approvals, granted explicitly by the user
// via the permission dialog ("always allow <tool> inside CodeMode") and
// revocable with codemode.revoke(tool). These are NOT project-wide rules.
type approvalsDoc struct {
	Tools []string `json:"tools"`
}

func (t *codeModeTool) approvalsPath() string {
	return filepath.Join(t.dataDir, "approvals.json")
}

// loadApprovals reads the persisted approval list (nil when absent/unreadable).
// Caller holds no lock; t.approvalMu serializes read-modify-write cycles.
func (t *codeModeTool) loadApprovals() []string {
	if t.dataDir == "" {
		return nil
	}
	raw, err := os.ReadFile(t.approvalsPath())
	if err != nil {
		return nil
	}
	var doc approvalsDoc
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	sort.Strings(doc.Tools)
	return doc.Tools
}

// hasApproval reports whether the tool has a persisted CodeMode approval.
func (t *codeModeTool) hasApproval(name string) bool {
	t.approvalMu.Lock()
	defer t.approvalMu.Unlock()
	for _, n := range t.loadApprovals() {
		if n == name {
			return true
		}
	}
	return false
}

// saveApproval persists one approval (best-effort, idempotent — a persistence
// failure must not fail the already-approved call; the execution-scope cache
// still applies).
func (t *codeModeTool) saveApproval(name string) {
	if t.dataDir == "" {
		return
	}
	t.approvalMu.Lock()
	defer t.approvalMu.Unlock()
	tools := t.loadApprovals()
	for _, n := range tools {
		if n == name {
			return
		}
	}
	tools = append(tools, name)
	sort.Strings(tools)
	raw, err := json.MarshalIndent(approvalsDoc{Tools: tools}, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(t.dataDir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(t.approvalsPath(), raw, 0o644)
}

// revokeApproval removes one persisted approval and returns the remaining
// list (idempotent). Revocation tightens permissions, so the sandbox may do
// it without an extra prompt.
func (t *codeModeTool) revokeApproval(name string) ([]string, error) {
	if t.dataDir == "" {
		return nil, errNoDataDir
	}
	t.approvalMu.Lock()
	defer t.approvalMu.Unlock()
	tools := t.loadApprovals()
	kept := tools[:0]
	for _, n := range tools {
		if n != name {
			kept = append(kept, n)
		}
	}
	raw, err := json.MarshalIndent(approvalsDoc{Tools: kept}, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(t.approvalsPath(), raw, 0o644); err != nil {
		return nil, err
	}
	return append([]string(nil), kept...), nil
}
