package engine

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

// TestQuery_MainLoopRequestUnchangedWithoutTiers pins the main loop's request
// bytes (anti-goal: tiers must not move the main loop's prompt prefix). The
// golden file was generated before the tier/compaction changes.
func TestQuery_MainLoopRequestUnchangedWithoutTiers(t *testing.T) {
	reg := newTestRegistry(&stubTool{name: "Read"}, &stubTool{name: "Bash"})
	eng := New(Config{Client: &mockClient{}, Registry: reg, Model: "claude-sonnet-5-5"}).(*engineImpl)
	messages := []types.Message{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("first")}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("reply")}}},
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("second")}}},
	}
	params := QueryParams{
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{
			{Text: "stable prefix", CacheControl: "ephemeral"},
			{Text: "dynamic tail"},
		}},
		QuerySource:    "foreground",
		ToolUseContext: &tools.UseContext{Ctx: context.Background()},
	}
	req, err := eng.buildRequestWithModel(params, messages, eng.MainTarget().Model)
	if err != nil {
		t.Fatalf("buildRequestWithModel: %v", err)
	}
	got, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join("testdata", "main_request.golden.json")
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("main-loop request bytes changed:\n got: %s\nwant: %s", got, want)
	}
}
