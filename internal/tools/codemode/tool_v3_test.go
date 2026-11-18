package codemode_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/codemode"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/tools"
	cmtool "github.com/tunsuy/claude-code-go/internal/tools/codemode"
)

// writeTool is a non-read-only tool for audit / write-gate tests (CM-03).
type writeTool struct {
	tools.BaseTool
	name string
}

func (w *writeTool) Name() string { return w.name }
func (w *writeTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "A write tool for tests."
}
func (w *writeTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(map[string]json.RawMessage{
		"msg": tools.PropSchema(map[string]any{"type": "string"}),
	}, nil)
}
func (w *writeTool) UserFacingName(_ tools.Input) string  { return w.name }
func (w *writeTool) IsReadOnly(_ tools.Input) bool        { return false }
func (w *writeTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (w *writeTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	return &tools.Result{Content: "wrote:" + string(input)}, nil
}

// cmAskChecker always Answers Ask; RequestPermission returns the configured
// decision and can mark the approval as CodeMode-persistent (CM-03).
type cmAskChecker struct {
	mu               sync.Mutex
	prompts          int
	decision         tools.PermissionBehavior
	alwaysInCodeMode bool
}

func (a *cmAskChecker) CanUseTool(_ context.Context, _ string, _ tools.Input, _ *tools.UseContext) (tools.PermissionResult, error) {
	return tools.PermissionResult{Behavior: tools.PermissionAsk}, nil
}
func (a *cmAskChecker) RequestPermission(_ context.Context, _ permissions.PermissionRequest, _ *tools.UseContext) (tools.PermissionResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prompts++
	return tools.PermissionResult{
		Behavior:         a.decision,
		Reason:           "user decision",
		AlwaysInCodeMode: a.decision == tools.PermissionAllow && a.alwaysInCodeMode,
	}, nil
}
func (a *cmAskChecker) GetDenialCount() int { return 0 }
func (a *cmAskChecker) promptCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prompts
}

func callInput(t *testing.T, tool tools.Tool, in map[string]any) *tools.Result {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Call(raw, &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	return res
}

// lastHistoryEntry reads the newest history.jsonl entry.
func lastHistoryEntry(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "history.jsonl"))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var e map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &e); err != nil {
		t.Fatalf("parse history entry: %v", err)
	}
	return e
}

func newV3Tool(t *testing.T, chk permissions.Checker, extra ...tools.Tool) (tools.Tool, string) {
	t.Helper()
	dir := t.TempDir()
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})
	for _, tl := range extra {
		reg.Register(tl)
	}
	return cmtool.NewCodeModeTool(reg, chk, codemode.NewMoejsRuntime(), dir), dir
}

func TestCodeModeV3_ReplayInput(t *testing.T) {
	tool, dir := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	res := callCode(t, tool, `return 40 + 2;`)
	if res.IsError {
		t.Fatal(res.Content)
	}
	id := lastHistoryEntry(t, dir)["id"].(string)

	res = callInput(t, tool, map[string]any{"replay": id})
	if res.IsError {
		t.Fatalf("replay failed: %v", res.Content)
	}
	if res.Content != "42" {
		t.Errorf("replay result: %v", res.Content)
	}
	e := lastHistoryEntry(t, dir)
	if e["replay_of"] != id {
		t.Errorf("replay provenance must be recorded, got %v", e["replay_of"])
	}
}

func TestCodeModeV3_ReplayMutuallyExclusive(t *testing.T) {
	tool, _ := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	res := callInput(t, tool, map[string]any{"code": "return 1;", "replay": "some-id"})
	if !res.IsError || !strings.Contains(res.Content.(string), "mutually exclusive") {
		t.Errorf("code+replay must be rejected, got %v", res.Content)
	}
}

func TestCodeModeV3_ReplayUnknownID(t *testing.T) {
	tool, _ := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	res := callInput(t, tool, map[string]any{"replay": "does-not-exist"})
	if !res.IsError {
		t.Error("unknown replay id must fail")
	}
}

func TestCodeModeV3_ReplayHook(t *testing.T) {
	tool, dir := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	callCode(t, tool, `return 42;`)
	id := lastHistoryEntry(t, dir)["id"].(string)
	res := callCode(t, tool, fmt.Sprintf(`return await codemode.replay(%q);`, id))
	if res.IsError {
		t.Fatalf("replay hook failed: %v", res.Content)
	}
	if res.Content != "42" {
		t.Errorf("replay hook result: %v", res.Content)
	}
}

func TestCodeModeV3_ReplayTypeScriptPreserved(t *testing.T) {
	tool, dir := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	res := callInput(t, tool, map[string]any{
		"code":       `const x: number = 42; return x;`,
		"typescript": true,
	})
	if res.IsError {
		t.Fatal(res.Content)
	}
	id := lastHistoryEntry(t, dir)["id"].(string)
	res = callInput(t, tool, map[string]any{"replay": id})
	if res.IsError || res.Content != "42" {
		t.Errorf("TS replay must preserve the typescript flag: %v (err=%v)", res.Content, res.IsError)
	}
}

func TestCodeModeV3_ReplayStillAsksPermission(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionAllow}
	tool, dir := newV3Tool(t, chk)
	callCode(t, tool, `return await tools.Echo({ msg: "x" });`)
	id := lastHistoryEntry(t, dir)["id"].(string)
	callInput(t, tool, map[string]any{"replay": id})
	if got := chk.promptCount(); got != 2 {
		t.Errorf("replay must re-run the permission pipeline (2 prompts), got %d", got)
	}
}

func TestCodeModeV3_SnippetSaveAndRun(t *testing.T) {
	tool, dir := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	res := callInput(t, tool, map[string]any{"code": `return 21 * 2;`, "save_as": "double"})
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content.(string), `saved as snippet "double"`) {
		t.Errorf("save confirmation missing: %v", res.Content)
	}
	if _, err := os.Stat(filepath.Join(dir, "snippets", "double.js")); err != nil {
		t.Fatalf("snippet file must exist: %v", err)
	}
	res = callCode(t, tool, `
		const list = await codemode.snippets();
		const v = await codemode.run("double");
		return { list, v };
	`)
	if res.IsError {
		t.Fatalf("snippet run failed: %v", res.Content)
	}
	var m map[string]any
	json.Unmarshal([]byte(res.Content.(string)), &m)
	if fmt.Sprintf("%v", m["v"]) != "42" {
		t.Errorf("snippet result: %v", m["v"])
	}
	if !strings.Contains(fmt.Sprintf("%v", m["list"]), "double") {
		t.Errorf("snippets() must list the saved name: %v", m["list"])
	}
}

func TestCodeModeV3_SnippetTraversalRejected(t *testing.T) {
	tool, _ := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	res := callInput(t, tool, map[string]any{"code": `return 1;`, "save_as": "../evil"})
	if !strings.Contains(res.Content.(string), "snippet save failed") {
		t.Errorf("traversal save_as must fail: %v", res.Content)
	}
	res = callCode(t, tool, `return await codemode.run("../evil");`)
	if !res.IsError {
		t.Error("traversal snippet run must fail")
	}
}

func TestCodeModeV3_SnippetSharesExecAllow(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionAllow}
	tool, _ := newV3Tool(t, chk, &writeTool{name: "Write"})
	// Execution 1: two writes, one prompt; save a snippet doing two writes.
	res := callInput(t, tool, map[string]any{
		"code":    `await tools.Write({msg:"a"}); await tools.Write({msg:"b"}); return "ok";`,
		"save_as": "twice",
	})
	if res.IsError {
		t.Fatal(res.Content)
	}
	// Execution 2: fresh scope → one prompt for the snippet's two writes.
	res = callCode(t, tool, `return await codemode.run("twice");`)
	if res.IsError {
		t.Fatalf("snippet run: %v", res.Content)
	}
	if got := chk.promptCount(); got != 2 {
		t.Errorf("expected 2 prompts total (1 per execution), got %d", got)
	}
}

func TestCodeModeV3_AuditRecordsWritesOnly(t *testing.T) {
	tool, dir := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow}, &writeTool{name: "Write"})
	callCode(t, tool, `
		await tools.Echo({ msg: "read" });
		await tools.Write({ msg: "change" });
		return "done";
	`)
	e := lastHistoryEntry(t, dir)
	effects, ok := e["side_effects"].([]any)
	if !ok || len(effects) != 1 {
		t.Fatalf("audit must record exactly the write call, got %v", e["side_effects"])
	}
	first := effects[0].(map[string]any)
	if first["tool"] != "Write" || !strings.Contains(first["input"].(string), "change") {
		t.Errorf("audit entry shape: %v", first)
	}
	id := e["id"].(string)
	res := callCode(t, tool, fmt.Sprintf(`return await codemode.audit(%q);`, id))
	if res.IsError || !strings.Contains(res.Content.(string), "Write") {
		t.Errorf("codemode.audit must return the trail: %v", res.Content)
	}
}

func TestCodeModeV3_PersistedApprovalAndRevoke(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionAllow, alwaysInCodeMode: true}
	tool, dir := newV3Tool(t, chk, &writeTool{name: "Write"})
	callCode(t, tool, `await tools.Write({msg:"a"}); return 1;`)
	raw, err := os.ReadFile(filepath.Join(dir, "approvals.json"))
	if err != nil || !strings.Contains(string(raw), "Write") {
		t.Fatalf("approval must be persisted: %v (%s)", err, raw)
	}
	// New execution: persisted approval → no prompt.
	callCode(t, tool, `await tools.Write({msg:"b"}); return 2;`)
	if got := chk.promptCount(); got != 1 {
		t.Errorf("persisted approval must skip the prompt, got %d prompts", got)
	}
	// List + revoke inside the sandbox, then a prompt must return.
	res := callCode(t, tool, `
		const before = await codemode.approvals();
		await codemode.revoke("Write");
		const after = await codemode.approvals();
		return { before, after };
	`)
	if res.IsError {
		t.Fatal(res.Content)
	}
	if !strings.Contains(res.Content.(string), "Write") {
		t.Errorf("approvals() must list Write: %v", res.Content)
	}
	callCode(t, tool, `await tools.Write({msg:"c"}); return 3;`)
	if got := chk.promptCount(); got != 2 {
		t.Errorf("revoked approval must prompt again, got %d prompts", got)
	}
}

func TestCodeModeV3_DenyOverridesPersistedApproval(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionAllow, alwaysInCodeMode: true}
	dir := t.TempDir()
	reg := tools.NewRegistry()
	reg.Register(&writeTool{name: "Write"})
	tool := cmtool.NewCodeModeTool(reg, chk, codemode.NewMoejsRuntime(), dir)
	callCode(t, tool, `await tools.Write({msg:"a"}); return 1;`)
	// Policy flips to Deny (e.g. plan mode): the persisted approval must NOT
	// override it — CanUseTool is consulted first on every call.
	denyTool := cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: tools.PermissionDeny}, codemode.NewMoejsRuntime(), dir)
	res := callCode(t, denyTool, `await tools.Write({msg:"b"}); return 2;`)
	if !res.IsError || !strings.Contains(res.Content.(string), "denied") {
		t.Errorf("Deny must override the persisted approval, got %v", res.Content)
	}
}

func TestCodeModeV3_WriteGateSingleApproval(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionAllow}
	tool, _ := newV3Tool(t, chk, &writeTool{name: "Write"}, &writeTool{name: "Delete"})
	res := callInput(t, tool, map[string]any{
		"code": `
			await tools.Write({msg:"a"});
			await tools.Delete({msg:"b"});
			await tools.Write({msg:"c"});
			await tools.Echo({msg:"read"});
			return "done";
		`,
		"batch_writes": true,
	})
	if res.IsError {
		t.Fatalf("write gate run: %v", res.Content)
	}
	// 1 gate prompt for ALL writes + 1 normal Ask for the read.
	if got := chk.promptCount(); got != 2 {
		t.Errorf("expected 2 prompts (write gate + read), got %d", got)
	}
}

func TestCodeModeV3_WriteGateParallelWrites(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionAllow}
	tool, _ := newV3Tool(t, chk, &writeTool{name: "Write"})
	res := callInput(t, tool, map[string]any{
		"code": `
			const rs = await Promise.all([1,2,3,4,5].map(i => tools.Write({msg:"w"+i})));
			return rs.length;
		`,
		"batch_writes": true,
	})
	if res.IsError {
		t.Fatalf("parallel writes: %v", res.Content)
	}
	if got := chk.promptCount(); got != 1 {
		t.Errorf("parallel writes must share ONE gate prompt, got %d", got)
	}
}

func TestCodeModeV3_WriteGateDenyBlocksAllWrites(t *testing.T) {
	chk := &cmAskChecker{decision: tools.PermissionDeny}
	tool, _ := newV3Tool(t, chk, &writeTool{name: "Write"})
	res := callInput(t, tool, map[string]any{
		"code": `
			let blocked = 0;
			for (let i = 0; i < 3; i++) {
				try { await tools.Write({msg:"w"+i}); } catch (e) { blocked++; }
			}
			return blocked;
		`,
		"batch_writes": true,
	})
	if res.IsError {
		t.Fatalf("gate deny run: %v", res.Content)
	}
	if res.Content != "3" {
		t.Errorf("all writes must be blocked after one gate denial, got %v", res.Content)
	}
	if got := chk.promptCount(); got != 1 {
		t.Errorf("gate denial must ask exactly once, got %d", got)
	}
}

func TestCodeModeV3_NestedDepthGuard(t *testing.T) {
	tool, dir := newV3Tool(t, &fakeChecker{behavior: tools.PermissionAllow})
	snDir := filepath.Join(dir, "snippets")
	os.MkdirAll(snDir, 0o755)
	os.WriteFile(filepath.Join(snDir, "loop.js"), []byte(`return await codemode.run("loop");`), 0o644)
	res := callCode(t, tool, `return await codemode.run("loop");`)
	if !res.IsError || !strings.Contains(res.Content.(string), "depth") {
		t.Errorf("recursive snippets must hit the depth guard, got %v (err=%v)", res.Content, res.IsError)
	}
}

func TestCodeModeV3_DisabledWithoutDataDir(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	for _, code := range []string{
		`return await codemode.replay("x");`,
		`return await codemode.run("x");`,
		`return await codemode.approvals();`,
	} {
		res := callCode(t, tool, code)
		if !res.IsError || !strings.Contains(res.Content.(string), "not available") {
			t.Errorf("persistence hooks must be disabled without dataDir: %q → %v", code, res.Content)
		}
	}
}
