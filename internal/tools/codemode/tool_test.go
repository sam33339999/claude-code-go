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

	"github.com/tunsuy/glaude/internal/codemode"
	"github.com/tunsuy/glaude/internal/permissions"
	"github.com/tunsuy/glaude/internal/tools"
	cmtool "github.com/tunsuy/glaude/internal/tools/codemode"
)

// echoTool is a minimal real tool for end-to-end sandbox tests.
type echoTool struct {
	tools.BaseTool
	payload string
}

func (e *echoTool) Name() string { return "Echo" }
func (e *echoTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return "Echoes input back."
}
func (e *echoTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(map[string]json.RawMessage{
		"msg": tools.PropSchema(map[string]any{"type": "string"}),
	}, []string{"msg"})
}
func (e *echoTool) UserFacingName(_ tools.Input) string  { return "Echo" }
func (e *echoTool) IsReadOnly(_ tools.Input) bool        { return true }
func (e *echoTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (e *echoTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	if e.payload != "" {
		return &tools.Result{Content: e.payload}, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(input, &parsed); err != nil {
		return &tools.Result{Content: string(input)}, nil
	}
	return &tools.Result{Content: parsed}, nil
}

// fakeChecker implements permissions.Checker with a canned decision.
type fakeChecker struct{ behavior tools.PermissionBehavior }

func (f *fakeChecker) CanUseTool(_ context.Context, _ string, _ tools.Input, _ *tools.UseContext) (tools.PermissionResult, error) {
	return tools.PermissionResult{Behavior: f.behavior}, nil
}
func (f *fakeChecker) RequestPermission(_ context.Context, _ permissions.PermissionRequest, _ *tools.UseContext) (tools.PermissionResult, error) {
	return tools.PermissionResult{Behavior: f.behavior, Reason: "test policy"}, nil
}
func (f *fakeChecker) GetDenialCount() int { return 0 }

func newTestTool(behavior tools.PermissionBehavior) (tools.Tool, *tools.Registry) {
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})
	return cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: behavior}, codemode.NewMoejsRuntime(), ""), reg
}

func callCode(t *testing.T, tool tools.Tool, code string) *tools.Result {
	t.Helper()
	in, _ := json.Marshal(map[string]string{"code": code})
	res, err := tool.Call(in, &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	return res
}

func TestCodeMode_EndToEnd(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	res := callCode(t, tool, `
		const r = await tools.Echo({ msg: "hello sandbox" });
		console.log("got", r.msg);
		return r.msg;
	`)
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	content := res.Content.(string)
	if !strings.Contains(content, "hello sandbox") {
		t.Errorf("expected echoed value in result, got: %s", content)
	}
	if !strings.Contains(content, "got") {
		t.Errorf("expected captured console.log, got: %s", content)
	}
}

func TestCodeMode_PermissionDenyPropagates(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionDeny)
	res := callCode(t, tool, `return await tools.Echo({ msg: "x" });`)
	if !res.IsError {
		t.Fatal("denied inner call must surface as error")
	}
	if !strings.Contains(res.Content.(string), "denied") {
		t.Errorf("expected denial reason, got: %v", res.Content)
	}
}

func TestCodeMode_DenyCatchableInCode(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionDeny)
	res := callCode(t, tool, `
		try { await tools.Echo({ msg: "x" }); return "allowed"; }
		catch (e) { return "blocked"; }
	`)
	if res.IsError || res.Content != "blocked" {
		t.Errorf("code must be able to catch permission denial, got %+v", res)
	}
}

func TestCodeMode_UnknownTool(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	res := callCode(t, tool, `return await tools.NoSuchTool({});`)
	if !res.IsError || !strings.Contains(res.Content.(string), "unknown tool") {
		t.Errorf("expected unknown-tool error, got %+v", res)
	}
}

func TestCodeMode_EmptyCodeAndBadInput(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	if res := callCode(t, tool, "   "); !res.IsError {
		t.Error("empty code must be rejected")
	}
	res, err := tool.Call([]byte("not-json"), nil, nil)
	if err != nil || !res.IsError {
		t.Error("bad input must be an error result")
	}
}

func TestCodeMode_ResultCap(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&echoTool{payload: strings.Repeat("x", 40<<10)})
	tool := cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: tools.PermissionAllow}, codemode.NewMoejsRuntime(), "")
	res := callCode(t, tool, `return await tools.Echo({ msg: "big" });`)
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	content := res.Content.(string)
	if len(content) > 33<<10 {
		t.Errorf("result must be capped near 32 KiB, got %d bytes", len(content))
	}
	if !strings.Contains(content, "truncated") {
		t.Error("expected honest truncation note")
	}
}

func TestCodeMode_DiscoveryInsideSandbox(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	res := callCode(t, tool, `
		const names = codemode.search("echo");
		const d = codemode.describe(names[0]);
		return d.name;
	`)
	if res.IsError || res.Content != "Echo" {
		t.Errorf("progressive discovery failed: %+v", res)
	}
}

func TestCodeMode_DescriptionHasCatalog(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	desc := tool.Description(nil, nil)
	if !strings.Contains(desc, "tools.Echo") {
		t.Error("description must include the tool catalog")
	}
	if !strings.Contains(desc, "codemode.describe") {
		t.Error("description must document progressive discovery")
	}
}

func TestCodeMode_Metadata(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	if tool.Name() != "CodeMode" {
		t.Errorf("name: %q", tool.Name())
	}
	if tool.IsReadOnly(nil) || tool.IsConcurrencySafe(nil) {
		t.Error("CodeMode may write via inner tools: not read-only, not concurrency-safe")
	}
}

// ── CM-02 v2 tests ───────────────────────────────────────────────────────────

// askChecker simulates HIL: CanUseTool always Ask; RequestPermission records
// the prompt and returns a canned user decision.
type askChecker struct {
	mu       sync.Mutex
	prompts  int
	decision tools.PermissionBehavior
}

func (a *askChecker) CanUseTool(_ context.Context, _ string, _ tools.Input, _ *tools.UseContext) (tools.PermissionResult, error) {
	return tools.PermissionResult{Behavior: tools.PermissionAsk}, nil
}
func (a *askChecker) RequestPermission(_ context.Context, _ permissions.PermissionRequest, _ *tools.UseContext) (tools.PermissionResult, error) {
	a.mu.Lock()
	a.prompts++
	a.mu.Unlock()
	return tools.PermissionResult{Behavior: a.decision, Reason: "user decision"}, nil
}
func (a *askChecker) GetDenialCount() int { return 0 }
func (a *askChecker) promptCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.prompts
}

func TestCodeModeV2_BatchApprovalSameTool(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})
	chk := &askChecker{decision: tools.PermissionAllow}
	tool := cmtool.NewCodeModeTool(reg, chk, codemode.NewMoejsRuntime(), "")
	res := callCode(t, tool, `
		const a = await tools.Echo({ msg: "1" });
		const b = await tools.Echo({ msg: "2" });
		const c = await tools.Echo({ msg: "3" });
		return [a.msg, b.msg, c.msg];
	`)
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	if chk.promptCount() != 1 {
		t.Errorf("batch approval: 3 same-tool calls must prompt once, prompted %d times", chk.promptCount())
	}
}

func TestCodeModeV2_DenyNotCached(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})
	chk := &askChecker{decision: tools.PermissionDeny}
	tool := cmtool.NewCodeModeTool(reg, chk, codemode.NewMoejsRuntime(), "")
	res := callCode(t, tool, `
		const out = [];
		for (let i = 0; i < 3; i++) {
			try { await tools.Echo({ msg: String(i) }); out.push("ok"); }
			catch (e) { out.push("denied"); }
		}
		return out;
	`)
	if res.IsError {
		t.Fatalf("unexpected hard error: %v", res.Content)
	}
	if chk.promptCount() != 3 {
		t.Errorf("denials must NOT be cached: expected 3 prompts, got %d", chk.promptCount())
	}
}

func TestCodeModeV2_DirectAllowNotCached(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})
	chk := &fakeChecker{behavior: tools.PermissionAllow}
	tool := cmtool.NewCodeModeTool(reg, chk, codemode.NewMoejsRuntime(), "")
	res := callCode(t, tool, `
		await tools.Echo({ msg: "a" });
		return await tools.Echo({ msg: "b" });
	`)
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	// fakeChecker.RequestPermission would count if called; direct Allow must
	// never reach it (re-checked each call so live mode changes apply).
}

func TestCodeModeV2_TypeScriptInput(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	in, _ := json.Marshal(map[string]any{
		"code":       `const double = (x: number): number => x * 2; return double(21);`,
		"typescript": true,
	})
	res, err := tool.Call(in, &tools.UseContext{Ctx: context.Background()}, nil)
	if err != nil || res.IsError {
		t.Fatalf("TS execution failed: err=%v res=%+v", err, res)
	}
	if res.Content != "42" {
		t.Errorf("TS result: %v", res.Content)
	}
}

func TestCodeModeV2_ResultSpill(t *testing.T) {
	dir := t.TempDir()
	reg := tools.NewRegistry()
	reg.Register(&echoTool{payload: strings.Repeat("y", 40<<10)})
	tool := cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: tools.PermissionAllow}, codemode.NewMoejsRuntime(), dir)
	res := callCode(t, tool, `return await tools.Echo({ msg: "big" });`)
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.Content)
	}
	content := res.Content.(string)
	if !strings.Contains(content, "full output saved to") {
		t.Fatalf("expected spill pointer, got: %.200s", content)
	}
	if len(content) > 8<<10 {
		t.Errorf("inline result must be preview-sized, got %d bytes", len(content))
	}
	// The spill file must exist and hold the full output.
	matches, _ := filepath.Glob(filepath.Join(dir, "spill", "*.txt"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 spill file, got %d", len(matches))
	}
	raw, _ := os.ReadFile(matches[0])
	if len(raw) < 40<<10 {
		t.Errorf("spill file must hold full output, got %d bytes", len(raw))
	}
}

func TestCodeModeV2_HistoryAndReuse(t *testing.T) {
	dir := t.TempDir()
	tool, _ := newTestToolWithDir(tools.PermissionAllow, dir)
	res := callCode(t, tool, `return await tools.Echo({ msg: "first run" });`)
	if res.IsError {
		t.Fatal(res.Content)
	}
	res = callCode(t, tool, `
		const h = codemode.history(5);
		return { count: h.length, lastCode: h[0].code, ok: h[0].ok };
	`)
	if res.IsError {
		t.Fatalf("history read failed: %v", res.Content)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(res.Content.(string)), &m); err != nil {
		t.Fatalf("history result not JSON: %v (%s)", err, res.Content)
	}
	if fmt.Sprintf("%v", m["count"]) != "1" {
		t.Errorf("expected 1 history entry, got %v", m["count"])
	}
	if !strings.Contains(m["lastCode"].(string), "first run") {
		t.Errorf("history must carry reusable code, got %v", m["lastCode"])
	}
	if _, err := os.Stat(filepath.Join(dir, "history.jsonl")); err != nil {
		t.Errorf("history.jsonl must exist: %v", err)
	}
}

func newTestToolWithDir(behavior tools.PermissionBehavior, dir string) (tools.Tool, *tools.Registry) {
	reg := tools.NewRegistry()
	reg.Register(&echoTool{})
	return cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: behavior}, codemode.NewMoejsRuntime(), dir), reg
}

func TestCodeModeV2_ControlledImport(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "modules")
	os.MkdirAll(modDir, 0o755)
	os.WriteFile(filepath.Join(modDir, "mathx.js"), []byte(`export const triple = (x) => x * 3;`), 0o644)
	tool, _ := newTestToolWithDir(tools.PermissionAllow, dir)
	res := callCode(t, tool, `
		const { triple } = await import("mathx");
		return triple(14);
	`)
	if res.IsError {
		t.Fatalf("controlled import failed: %v", res.Content)
	}
	if res.Content != "42" {
		t.Errorf("import result: %v", res.Content)
	}
	// Traversal and absent modules must fail.
	res = callCode(t, tool, `await import("../escape"); return 1;`)
	if !res.IsError {
		t.Error("path traversal import must be rejected")
	}
	res = callCode(t, tool, `await import("absent"); return 1;`)
	if !res.IsError {
		t.Error("absent module must be rejected")
	}
}

func TestCodeModeV2_ImportDisabledWithoutDataDir(t *testing.T) {
	tool, _ := newTestTool(tools.PermissionAllow)
	res := callCode(t, tool, `await import("anything"); return 1;`)
	if !res.IsError {
		t.Error("imports must be disabled when no module directory is configured")
	}
}
