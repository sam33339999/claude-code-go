package compact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/pkg/types"
)

// recordingClient records Complete requests and returns a fixed summary,
// or an error when fail is set.
type recordingClient struct {
	mu   sync.Mutex
	reqs []*api.MessageRequest
	fail func() bool
}

func (c *recordingClient) Stream(context.Context, *api.MessageRequest) (api.StreamReader, error) {
	return nil, errors.New("recordingClient: Stream not supported")
}

func (c *recordingClient) Complete(_ context.Context, req *api.MessageRequest) (*api.MessageResponse, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	c.mu.Unlock()
	if c.fail != nil && c.fail() {
		return nil, errors.New("boom")
	}
	return &api.MessageResponse{Content: []api.ContentBlock{{Type: "text", Text: "SUMMARY"}}}, nil
}

func (c *recordingClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

func (c *recordingClient) last() *api.MessageRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reqs[len(c.reqs)-1]
}

func failing() bool { return true }

func TestShouldCompact_UsesTargetWindow(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	a := NewAutoCompactor()
	tgt := Target{Model: "qwen", ContextWindow: 32000, MaxOutputTokens: 8192}
	threshold := 32000 - 8000 - 3200 // outputReserve=min(8192, 32000/4), buffer=min(13000, 32000/10)
	if got := AutoCompactThreshold(tgt); got != threshold {
		t.Fatalf("threshold = %d, want %d", got, threshold)
	}
	if !a.ShouldCompact(threshold, tgt, NewTracker()) {
		t.Error("est == threshold should compact")
	}
	if a.ShouldCompact(threshold-1, tgt, NewTracker()) {
		t.Error("est below threshold should not compact")
	}
}

func TestShouldCompact_PerTrackerBreaker(t *testing.T) {
	a := NewAutoCompactor()
	client := &recordingClient{fail: failing}
	tgt := Target{Client: client, Model: "m", ContextWindow: 32000}
	trA, trB := NewTracker(), NewTracker()
	msgs := []types.Message{textMsg(types.RoleUser, "hi")}
	for i := 0; i < MaxConsecutiveFailures; i++ {
		if _, err := a.Compact(context.Background(), msgs, CompactionParams{}, tgt, trA); err == nil {
			t.Fatal("expected Compact failure")
		}
	}
	if a.ShouldCompact(1_000_000, tgt, trA) {
		t.Error("tracker A breaker should be open")
	}
	if !a.ShouldCompact(1_000_000, tgt, trB) {
		t.Error("tracker B must be unaffected by A's failures")
	}
}

func TestCompact_UsesTargetClientAndModel(t *testing.T) {
	a := NewAutoCompactor()
	tier, main := &recordingClient{}, &recordingClient{}
	_ = main
	msgs := []types.Message{textMsg(types.RoleUser, "hi")}
	res, err := a.Compact(context.Background(), msgs, CompactionParams{}, Target{Client: tier, Model: "glm"}, NewTracker())
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if tier.count() != 1 || tier.last().Model != "glm" {
		t.Errorf("tier client got %d requests (model %q), want 1 with glm", tier.count(), tier.last().Model)
	}
	if main.count() != 0 {
		t.Errorf("main client got %d requests, want 0", main.count())
	}
	if len(res.SummaryMessages) != 1 {
		t.Errorf("summary messages = %d, want 1", len(res.SummaryMessages))
	}
}

func TestCompact_NilClientFails(t *testing.T) {
	a := NewAutoCompactor()
	tr := NewTracker()
	_, err := a.Compact(context.Background(), nil, CompactionParams{}, Target{Model: "m"}, tr)
	if err == nil || tr.Snapshot().ConsecutiveFailures != 1 {
		t.Errorf("err = %v, failures = %d; want error and 1 failure", err, tr.Snapshot().ConsecutiveFailures)
	}
}

func TestCompact_SummaryMaxTokensCapped(t *testing.T) {
	a := NewAutoCompactor()
	msgs := []types.Message{textMsg(types.RoleUser, "hi")}
	cases := map[int]int{8192: 8192, 0: MaxOutputTokensForSummary, 64000: MaxOutputTokensForSummary}
	for maxOut, want := range cases {
		c := &recordingClient{}
		_, err := a.Compact(context.Background(), msgs, CompactionParams{}, Target{Client: c, Model: "m", MaxOutputTokens: maxOut}, NewTracker())
		if err != nil {
			t.Fatalf("Compact: %v", err)
		}
		if got := c.last().MaxTokens; got != want {
			t.Errorf("MaxOutputTokens %d → summary MaxTokens %d, want %d", maxOut, got, want)
		}
	}
}

func TestCompact_FitsSummaryInput(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	a := NewAutoCompactor()
	c := &recordingClient{}
	tgt := Target{Client: c, Model: "m", ContextWindow: 60000, MaxOutputTokens: 8192}

	// ~80k tokens of history: tool pairs interleaved with user text turns.
	chunk := strings.Repeat("y", 8000) // ~2k tokens
	var msgs []types.Message
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("t%d", i)
		msgs = append(msgs, textMsg(types.RoleUser, "q"+id))
		msgs = append(msgs, toolUseMsg(id, "Read", map[string]any{"p": id}))
		msgs = append(msgs, toolResultMsg(id, chunk))
	}
	msgs = append(msgs, textMsg(types.RoleUser, "final question"))
	if EstimateTokens(msgs) < 70000 {
		t.Fatalf("fixture too small: %d", EstimateTokens(msgs))
	}

	if _, err := a.Compact(context.Background(), msgs, CompactionParams{}, tgt, NewTracker()); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	sent := c.last().Messages
	sentTokens := 0
	for _, m := range sent {
		sentTokens += len(m.Content) / 4
	}
	if budget := 60000 - 8192 - min(AutoCompactBufferTokens, 60000/10); sentTokens > budget {
		t.Errorf("sent ~%d tokens, budget %d", sentTokens, budget)
	}
	if sent[0].Role != string(types.RoleUser) || strings.Contains(string(sent[0].Content), "tool_result") {
		t.Errorf("first sent message must be a plain user message, got %s %s", sent[0].Role, sent[0].Content)
	}
	if !strings.Contains(string(sent[len(sent)-1].Content), "final question") {
		t.Error("last message must be preserved")
	}
}

func TestCompact_SuccessResetsTracker(t *testing.T) {
	a := NewAutoCompactor()
	n := 0
	c := &recordingClient{fail: func() bool { n++; return n <= 2 }}
	tr := NewTracker()
	tgt := Target{Client: c, Model: "m"}
	msgs := []types.Message{textMsg(types.RoleUser, "hi")}
	for i := 0; i < 3; i++ {
		_, _ = a.Compact(context.Background(), msgs, CompactionParams{}, tgt, tr)
	}
	snap := tr.Snapshot()
	if snap.ConsecutiveFailures != 0 || !snap.Compacted {
		t.Errorf("snapshot = %+v, want 0 failures and Compacted", snap)
	}
}

func TestTracker_ConcurrentUse(t *testing.T) {
	a := NewAutoCompactor()
	var mu sync.Mutex
	n := 0
	c := &recordingClient{fail: func() bool { mu.Lock(); defer mu.Unlock(); n++; return n%2 == 0 }}
	tr := NewTracker()
	tgt := Target{Client: c, Model: "m"}
	msgs := []types.Message{textMsg(types.RoleUser, "hi")}
	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				_, _ = a.Compact(context.Background(), msgs, CompactionParams{}, tgt, tr)
				_ = a.ShouldCompact(1_000_000, tgt, tr)
				_ = tr.Snapshot()
			}
		}()
	}
	wg.Wait()
}

// TestCompact_FitKeepsToolLoopTail: a sub-agent history is one task prompt
// followed by a long tool loop. Trimming must keep the task prompt and the
// most recent tool round trips, not collapse to the prompt alone.
func TestCompact_FitKeepsToolLoopTail(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	a := NewAutoCompactor()
	c := &recordingClient{}
	tgt := Target{Client: c, Model: "m", ContextWindow: 60000, MaxOutputTokens: 8192}
	chunk := strings.Repeat("z", 8000)
	msgs := []types.Message{textMsg(types.RoleUser, "TASK: audit the repo")}
	for i := 0; i < 40; i++ {
		id := fmt.Sprintf("t%d", i)
		msgs = append(msgs, toolUseMsg(id, "Read", map[string]any{"p": id}))
		msgs = append(msgs, toolResultMsg(id, fmt.Sprintf("result-%d %s", i, chunk)))
	}
	if _, err := a.Compact(context.Background(), msgs, CompactionParams{}, tgt, NewTracker()); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	sent := c.last().Messages
	if len(sent) < 3 {
		t.Fatalf("sent %d messages, want task prompt + tool round trips", len(sent))
	}
	if !strings.Contains(string(sent[0].Content), "TASK: audit the repo") || !strings.Contains(string(sent[0].Content), "truncated") {
		t.Errorf("first message must keep the task prompt and note the truncation: %s", sent[0].Content)
	}
	if sent[1].Role != string(types.RoleAssistant) {
		t.Errorf("suffix must start at an assistant boundary, got %s", sent[1].Role)
	}
	if !strings.Contains(string(sent[len(sent)-1].Content), "result-39") {
		t.Error("most recent tool result must be kept")
	}
	total := 0
	for _, m := range sent {
		total += len(m.Content) / 4
	}
	if budget := 60000 - 8192 - min(AutoCompactBufferTokens, 60000/10); total > budget {
		t.Errorf("sent ~%d tokens, budget %d", total, budget)
	}
}

// TestAutoCompactThreshold_SmallWindow: reserves scale down for small
// windows (32k models are the main tier use case); 200k is unchanged.
func TestAutoCompactThreshold_SmallWindow(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	cases := []struct {
		window, maxOut, want int
	}{
		{32768, 8192, 32768 - 8192 - 3276},
		{32768, 16384, 32768 - 8192 - 3276},
		{16384, 4096, 16384 - 4096 - 1638},
		{200_000, 8192, 200_000 - 8192 - AutoCompactBufferTokens},
	}
	for _, c := range cases {
		got := AutoCompactThreshold(Target{Model: "m", ContextWindow: c.window, MaxOutputTokens: c.maxOut})
		if got != c.want {
			t.Errorf("window %d maxOut %d: threshold %d, want %d", c.window, c.maxOut, got, c.want)
		}
		if got <= 0 {
			t.Errorf("window %d: threshold must stay positive", c.window)
		}
	}
}

// TestShouldCompact_NoRepeatWithoutGrowth: when the post-compaction prompt
// is still above the threshold (prompt overhead alone is too big),
// compaction must wait for real growth instead of firing every turn.
func TestShouldCompact_NoRepeatWithoutGrowth(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	a := NewAutoCompactor()
	tgt := Target{Model: "m", ContextWindow: 32768, MaxOutputTokens: 8192}
	thr := AutoCompactThreshold(tgt)
	tr := NewTracker()
	tr.RecordPostCompact(thr + 500)
	if a.ShouldCompact(thr+1000, tgt, tr) {
		t.Error("must not re-compact without window/10 growth since the last compaction")
	}
	if !a.ShouldCompact(thr+500+32768/10, tgt, tr) {
		t.Error("should compact again after enough growth")
	}
	tr2 := NewTracker()
	tr2.RecordPostCompact(1000) // normal case: summary is small
	if !a.ShouldCompact(thr, tgt, tr2) {
		t.Error("normal threshold behaviour must be unchanged")
	}
}
