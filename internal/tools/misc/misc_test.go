package misc_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/fileops"
	"github.com/tunsuy/claude-code-go/internal/tools/misc"
	"github.com/tunsuy/claude-code-go/internal/tools/shell"
)

// ── SkillTool ─────────────────────────────────────────────────────────────────

func TestSkillTool_Name(t *testing.T) {
	if misc.SkillTool.Name() != "Skill" {
		t.Errorf("expected Skill, got %q", misc.SkillTool.Name())
	}
}

func TestSkillTool_IsConcurrencySafe_False(t *testing.T) {
	if misc.SkillTool.IsConcurrencySafe(nil) {
		t.Error("SkillTool must not be concurrency-safe")
	}
}

func TestSkillTool_IsReadOnly_False(t *testing.T) {
	if misc.SkillTool.IsReadOnly(nil) {
		t.Error("SkillTool must not be read-only")
	}
}

func TestSkillTool_InputSchema(t *testing.T) {
	schema := misc.SkillTool.InputSchema()
	if _, ok := schema.Properties["skill"]; !ok {
		t.Error("schema missing 'skill'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "skill" {
		t.Errorf("expected Required=[skill], got %v", schema.Required)
	}
}

func TestSkillTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(misc.SkillInput{Skill: "commit"})
	name := misc.SkillTool.UserFacingName(in)
	if name != "Skill(commit)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestSkillTool_UserFacingName_NoInput(t *testing.T) {
	if misc.SkillTool.UserFacingName(nil) != "Skill" {
		t.Error("expected fallback Skill")
	}
}

func TestSkillTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.SkillInput{Skill: "commit"})
	result, err := misc.SkillTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── BriefTool ─────────────────────────────────────────────────────────────────

func TestBriefTool_Name(t *testing.T) {
	if misc.BriefTool.Name() != "Brief" {
		t.Errorf("expected Brief, got %q", misc.BriefTool.Name())
	}
}

func TestBriefTool_IsConcurrencySafe_True(t *testing.T) {
	if !misc.BriefTool.IsConcurrencySafe(nil) {
		t.Error("BriefTool should be concurrency-safe")
	}
}

func TestBriefTool_IsReadOnly_True(t *testing.T) {
	if !misc.BriefTool.IsReadOnly(nil) {
		t.Error("BriefTool should be read-only")
	}
}

func TestBriefTool_InputSchema(t *testing.T) {
	schema := misc.BriefTool.InputSchema()
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("schema missing 'content'")
	}
}

func TestBriefTool_ShortContentKeptInFull(t *testing.T) {
	in, _ := json.Marshal(misc.BriefInput{Content: "One sentence. Two sentences."})
	result, err := misc.BriefTool.Call(in, nil, nil)
	if err != nil || result.IsError {
		t.Fatalf("unexpected failure: err=%v result=%+v", err, result)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "kept in full") || !strings.Contains(content, "Two sentences.") {
		t.Errorf("short content must be kept in full, got: %s", content)
	}
}

func TestBriefTool_LongContentCompressedInOrder(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&sb, "Sentence %d discusses the deployment pipeline and its verification steps in detail. ", i)
	}
	in, _ := json.Marshal(misc.BriefInput{Content: sb.String()})
	result, err := misc.BriefTool.Call(in, nil, nil)
	if err != nil || result.IsError {
		t.Fatalf("unexpected failure: err=%v result=%+v", err, result)
	}
	content := result.Content.(string)
	if !strings.Contains(content, "sentences)") {
		t.Errorf("expected compression header, got: %s", content)
	}
	// Kept sentences must appear in original order (Sentence 0 before Sentence 19 if both kept).
	i0 := strings.Index(content, "Sentence 0 ")
	i19 := strings.Index(content, "Sentence 19 ")
	if i0 >= 0 && i19 >= 0 && i0 > i19 {
		t.Error("kept sentences must preserve original order")
	}
	if len(content) >= len(sb.String()) {
		t.Error("brief must be shorter than the input")
	}
	// Deterministic: same input → same output.
	result2, _ := misc.BriefTool.Call(in, nil, nil)
	if result2.Content != result.Content {
		t.Error("extractive brief must be deterministic")
	}
}

func TestBriefTool_EmptyContentRejected(t *testing.T) {
	in, _ := json.Marshal(misc.BriefInput{Content: "   "})
	result, _ := misc.BriefTool.Call(in, nil, nil)
	if !result.IsError {
		t.Error("empty content must be rejected")
	}
}

// ── ToolSearchTool（BF-12）────────────────────────────────────────────────────

func newSearchTool() tools.Tool {
	reg := tools.NewRegistry()
	reg.Register(fileops.FileReadTool)
	reg.Register(fileops.GrepTool)
	reg.Register(fileops.NotebookEditTool)
	reg.Register(shell.BashTool)
	return misc.NewToolSearchTool(reg)
}

func TestToolSearchTool_Metadata(t *testing.T) {
	tool := newSearchTool()
	if tool.Name() != "ToolSearch" {
		t.Errorf("expected ToolSearch, got %q", tool.Name())
	}
	if !tool.IsConcurrencySafe(nil) {
		t.Error("ToolSearch should be concurrency-safe")
	}
	if !tool.IsReadOnly(nil) {
		t.Error("ToolSearch should be read-only")
	}
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "file read"})
	if name := tool.UserFacingName(in); name != "ToolSearch(file read)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestToolSearchTool_FindsByHintAndName(t *testing.T) {
	tool := newSearchTool()
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "search file content regex"})
	result, err := tool.Call(in, nil, nil)
	if err != nil || result.IsError {
		t.Fatalf("unexpected failure: err=%v result=%+v", err, result)
	}
	var entries []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(result.Content.(string)), &entries); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if len(entries) == 0 || entries[0].Name != "Grep" {
		t.Errorf("expected Grep ranked first for content-regex query, got %+v", entries)
	}
	for _, e := range entries {
		if e.Description == "" {
			t.Errorf("entry %s missing description", e.Name)
		}
	}
}

func TestToolSearchTool_CamelCaseNameMatch(t *testing.T) {
	tool := newSearchTool()
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "notebook"})
	result, _ := tool.Call(in, nil, nil)
	if result.IsError || !strings.Contains(result.Content.(string), "NotebookEdit") {
		t.Errorf("expected NotebookEdit for 'notebook' query (camelCase split), got %+v", result)
	}
}

func TestToolSearchTool_NoMatchAndEmptyQuery(t *testing.T) {
	tool := newSearchTool()
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "zzzznonexistent"})
	result, _ := tool.Call(in, nil, nil)
	if result.IsError || !strings.Contains(result.Content.(string), "No tools match") {
		t.Errorf("expected honest no-match message, got %+v", result)
	}
	in, _ = json.Marshal(misc.ToolSearchInput{Query: "  "})
	result, _ = tool.Call(in, nil, nil)
	if !result.IsError {
		t.Error("empty query must be rejected")
	}
}

func TestToolSearchTool_NilRegistry(t *testing.T) {
	tool := misc.NewToolSearchTool(nil)
	in, _ := json.Marshal(misc.ToolSearchInput{Query: "x"})
	result, _ := tool.Call(in, nil, nil)
	if !result.IsError {
		t.Error("nil registry must report honest error")
	}
}

// ── SleepTool ─────────────────────────────────────────────────────────────────

func TestSleepTool_Name(t *testing.T) {
	if misc.SleepTool.Name() != "Sleep" {
		t.Errorf("expected Sleep, got %q", misc.SleepTool.Name())
	}
}

func TestSleepTool_InputSchema(t *testing.T) {
	schema := misc.SleepTool.InputSchema()
	if _, ok := schema.Properties["milliseconds"]; !ok {
		t.Error("schema missing 'milliseconds'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "milliseconds" {
		t.Errorf("expected Required=[milliseconds], got %v", schema.Required)
	}
}

func TestSleepTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 500})
	name := misc.SleepTool.UserFacingName(in)
	if name != "Sleep(500ms)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestSleepTool_Call_Completes(t *testing.T) {
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 20})
	start := time.Now()
	result, err := misc.SleepTool.Call(in, &tools.UseContext{Ctx: context.Background()}, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got: %v", result.Content)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("sleep returned too early: %v", elapsed)
	}
	if !strings.Contains(result.Content.(string), "20") {
		t.Errorf("expected duration in result, got: %v", result.Content)
	}
}

func TestSleepTool_Call_ZeroMilliseconds(t *testing.T) {
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 0})
	result, err := misc.SleepTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Errorf("0ms sleep should succeed immediately, got: %v", result.Content)
	}
}

func TestSleepTool_Call_NilUseContext(t *testing.T) {
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 1})
	result, err := misc.SleepTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Errorf("nil UseContext should fall back to background context, got: %v", result.Content)
	}
}

func TestSleepTool_Call_InvalidInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"negative", `{"milliseconds":-5}`, ">="},
		{"missing field", `{}`, "missing"},
		{"wrong type", `{"milliseconds":"soon"}`, "invalid input"},
		{"not json", `not json`, "invalid input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := misc.SleepTool.Call(json.RawMessage(tc.input), nil, nil)
			if err != nil {
				t.Fatalf("unexpected hard error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected IsError=true for %s", tc.name)
			}
			if !strings.Contains(result.Content.(string), tc.want) {
				t.Errorf("expected message containing %q, got: %v", tc.want, result.Content)
			}
		})
	}
}

func TestSleepTool_Call_ContextCancelledBeforeCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 5000})
	start := time.Now()
	result, err := misc.SleepTool.Call(in, &tools.UseContext{Ctx: ctx}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for pre-cancelled context")
	}
	if time.Since(start) > time.Second {
		t.Error("cancelled sleep should return immediately")
	}
}

func TestSleepTool_Call_ContextCancelledDuringSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(20*time.Millisecond, cancel)
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 5000})
	start := time.Now()
	result, err := misc.SleepTool.Call(in, &tools.UseContext{Ctx: ctx}, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when cancelled mid-sleep")
	}
	if elapsed > time.Second {
		t.Errorf("sleep should stop promptly on cancellation, took %v", elapsed)
	}
	if !strings.Contains(result.Content.(string), "cancel") {
		t.Errorf("expected cancellation message, got: %v", result.Content)
	}
}

func TestSleepTool_Call_DeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 5000})
	result, err := misc.SleepTool.Call(in, &tools.UseContext{Ctx: ctx}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when the deadline expires mid-sleep")
	}
	if !strings.Contains(result.Content.(string), "deadline") {
		t.Errorf("expected deadline message, got: %v", result.Content)
	}
}

func TestSleepTool_Call_AbortCh(t *testing.T) {
	abortCh := make(chan struct{})
	time.AfterFunc(20*time.Millisecond, func() { close(abortCh) })
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 5000})
	start := time.Now()
	result, err := misc.SleepTool.Call(in, &tools.UseContext{Ctx: context.Background(), AbortCh: abortCh}, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when AbortCh is closed mid-sleep")
	}
	if elapsed > time.Second {
		t.Errorf("sleep should stop promptly on abort, took %v", elapsed)
	}
	if !strings.Contains(result.Content.(string), "abort") {
		t.Errorf("expected abort message, got: %v", result.Content)
	}
}

// TestSleepTool_Call_OverflowRejected covers CR-BF01-5: durations above the 24h
// cap (including values that would overflow time.Duration to a negative and
// fire the timer immediately, falsely reporting success) are rejected honestly
// and return without sleeping.
func TestSleepTool_Call_OverflowRejected(t *testing.T) {
	cases := []struct {
		name string
		ms   int
	}{
		{"just above 24h cap", 24*60*60*1000 + 1},
		{"max int (overflow region)", int(^uint(0) >> 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, _ := json.Marshal(misc.SleepInput{Milliseconds: tc.ms})
			start := time.Now()
			result, err := misc.SleepTool.Call(in, nil, nil)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("unexpected hard error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected honest rejection, got false success: %v", result.Content)
			}
			if elapsed > 500*time.Millisecond {
				t.Errorf("overflow sleep must return immediately, took %v", elapsed)
			}
			if !strings.Contains(result.Content.(string), "<=") {
				t.Errorf("expected upper-bound message, got: %v", result.Content)
			}
		})
	}
}

// TestSleepTool_Call_AtCapAccepted proves the cap is inclusive without actually
// sleeping 24h: a pre-cancelled context turns the accepted cap value into a
// cancellation result (not a cap rejection).
func TestSleepTool_Call_AtCapAccepted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in, _ := json.Marshal(misc.SleepInput{Milliseconds: 24 * 60 * 60 * 1000})
	result, err := misc.SleepTool.Call(in, &tools.UseContext{Ctx: ctx}, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected cancellation result")
	}
	content := result.Content.(string)
	if strings.Contains(content, "<=") {
		t.Errorf("cap value should be accepted by validation, got cap rejection: %v", content)
	}
	if !strings.Contains(content, "cancel") {
		t.Errorf("expected cancellation message, got: %v", content)
	}
}

// ── SyntheticOutputTool ───────────────────────────────────────────────────────

func TestSyntheticOutputTool_Name(t *testing.T) {
	if misc.SyntheticOutputTool.Name() != "SyntheticOutput" {
		t.Errorf("expected SyntheticOutput, got %q", misc.SyntheticOutputTool.Name())
	}
}

func TestSyntheticOutputTool_IsConcurrencySafe_False(t *testing.T) {
	if misc.SyntheticOutputTool.IsConcurrencySafe(nil) {
		t.Error("SyntheticOutputTool must not be concurrency-safe")
	}
}

func TestSyntheticOutputTool_IsReadOnly_True(t *testing.T) {
	if !misc.SyntheticOutputTool.IsReadOnly(nil) {
		t.Error("SyntheticOutputTool should be read-only")
	}
}

func TestSyntheticOutputTool_InputSchema(t *testing.T) {
	schema := misc.SyntheticOutputTool.InputSchema()
	if _, ok := schema.Properties["content"]; !ok {
		t.Error("schema missing 'content'")
	}
	if _, ok := schema.Properties["is_error"]; !ok {
		t.Error("schema missing 'is_error'")
	}
	if len(schema.Required) != 1 || schema.Required[0] != "content" {
		t.Errorf("expected Required=[content], got %v", schema.Required)
	}
}

func TestSyntheticOutputTool_Call_ReturnsError(t *testing.T) {
	in, _ := json.Marshal(misc.SyntheticOutputInput{Content: "test"})
	result, err := misc.SyntheticOutputTool.Call(in, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestAllMiscTools_ImplementToolInterface(t *testing.T) {
	var _ tools.Tool = misc.SkillTool
	var _ tools.Tool = misc.BriefTool
	var _ tools.Tool = misc.NewToolSearchTool(nil)
	var _ tools.Tool = misc.SleepTool
	var _ tools.Tool = misc.SyntheticOutputTool
}
