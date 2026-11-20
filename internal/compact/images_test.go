package compact

import (
	"context"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/pkg/types"
)

// bigImageMsg is a tool_result carrying a ~1.3 MB base64 image.
func bigImageMsg() types.Message {
	id := "t1"
	data := strings.Repeat("A", 1_300_000)
	return types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{
		Type: types.ContentTypeToolResult, ToolUseID: &id,
		Content: []types.ContentBlock{{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: data}}},
	}}}
}

// Images are billed per image (~1–2k tokens), not by base64 length; counting
// base64 as text made one screenshot look like ~325k tokens and forced an
// immediate auto-compact.
func TestEstimateTokens_ImageIsFixedCost(t *testing.T) {
	got := EstimateTokens([]types.Message{bigImageMsg()})
	if got < imageTokenEstimate || got > imageTokenEstimate+200 {
		t.Errorf("EstimateTokens = %d, want ≈%d (fixed per-image cost)", got, imageTokenEstimate)
	}
}

// The summary request is text-in/text-out and may target a model that
// rejects images; images must be replaced by a placeholder.
func TestCompact_SummaryRequestOmitsImages(t *testing.T) {
	a := NewAutoCompactor()
	c := &recordingClient{}
	msgs := []types.Message{textMsg(types.RoleUser, "see screenshot"), bigImageMsg()}
	if _, err := a.Compact(context.Background(), msgs, CompactionParams{}, Target{Client: c, Model: "m"}, NewTracker()); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	for _, m := range c.last().Messages {
		if strings.Contains(string(m.Content), `"type":"image"`) {
			t.Fatalf("summary request carried an image: %.200s", m.Content)
		}
	}
	if !strings.Contains(string(c.last().Messages[len(c.last().Messages)-1].Content), "[image]") {
		t.Error("image should be replaced by an [image] placeholder")
	}
	if msgs[1].Content[0].Content[0].Type != types.ContentTypeImage {
		t.Error("caller's messages must not be mutated")
	}
}
