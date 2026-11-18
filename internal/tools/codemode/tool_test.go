package codemode_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/codemode"
	"github.com/tunsuy/claude-code-go/internal/permissions"
	"github.com/tunsuy/claude-code-go/internal/tools"
	cmtool "github.com/tunsuy/claude-code-go/internal/tools/codemode"
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
	return cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: behavior}, codemode.NewMoejsRuntime()), reg
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
	tool := cmtool.NewCodeModeTool(reg, &fakeChecker{behavior: tools.PermissionAllow}, codemode.NewMoejsRuntime())
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
