package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/compact"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// countingClient streams a fixed end_turn reply and records Stream/Complete
// requests. completeErr makes every summarisation fail.
type countingClient struct {
	mu          sync.Mutex
	streams     []*api.MessageRequest
	completes   []*api.MessageRequest
	completeErr error
	streamFn    func(req *api.MessageRequest) (api.StreamReader, error)
}

func (c *countingClient) Stream(_ context.Context, req *api.MessageRequest) (api.StreamReader, error) {
	c.mu.Lock()
	c.streams = append(c.streams, req)
	fn := c.streamFn
	c.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return newStaticReader(buildEndTurnEvents("ok")...), nil
}

func (c *countingClient) Complete(_ context.Context, req *api.MessageRequest) (*api.MessageResponse, error) {
	c.mu.Lock()
	c.completes = append(c.completes, req)
	c.mu.Unlock()
	if c.completeErr != nil {
		return nil, c.completeErr
	}
	return &api.MessageResponse{Content: []api.ContentBlock{{Type: "text", Text: "SUMMARY"}}}, nil
}

func (c *countingClient) counts() (streams, completes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.streams), len(c.completes)
}

func userText(s string) types.Message {
	return types.Message{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr(s)}}}
}

// bigHistory returns a single user message of roughly tokens tokens.
func bigHistory(tokens int) []types.Message {
	return []types.Message{userText(strings.Repeat("x", tokens*4))}
}

func runQuery(t *testing.T, eng QueryEngine, p QueryParams) []Msg {
	t.Helper()
	if p.ToolUseContext == nil {
		p.ToolUseContext = &tools.UseContext{Ctx: context.Background()}
	}
	ch, err := eng.Query(context.Background(), p)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	return drainMsgs(ch)
}

func TestQuery_UsesParamsClient(t *testing.T) {
	main, tier := &countingClient{}, &countingClient{}
	eng := New(Config{Client: main, Registry: tools.NewRegistry(), Model: "main-model"})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("hi")}, Client: tier, Model: "glm", NoWriteBack: true})
	if s, _ := tier.counts(); s != 1 {
		t.Errorf("tier streams = %d, want 1", s)
	}
	if s, _ := main.counts(); s != 0 {
		t.Errorf("main streams = %d, want 0", s)
	}
}

func TestQuery_DefaultClientWithoutParamsClient(t *testing.T) {
	main := &countingClient{}
	eng := New(Config{Client: main, Registry: tools.NewRegistry(), Model: "main-model"})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("hi")}})
	if s, _ := main.counts(); s != 1 {
		t.Errorf("main streams = %d, want 1", s)
	}
}

func TestQuery_TierCompactionHitsTierClient(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	main, tier := &countingClient{}, &countingClient{}
	eng := New(Config{Client: main, Registry: tools.NewRegistry(), Model: "main-model"})
	runQuery(t, eng, QueryParams{
		Messages: bigHistory(25_000), Client: tier, Model: "glm", ContextWindow: 40_000,
		CompactTracker: compact.NewTracker(), NoWriteBack: true,
	})
	_, tc := tier.counts()
	if tc != 1 || tier.completes[0].Model != "glm" {
		t.Fatalf("tier summary requests = %d, want 1 with model glm", tc)
	}
	if _, mc := main.counts(); mc != 0 {
		t.Errorf("main summary requests = %d, want 0", mc)
	}
}

func TestQuery_SubAgentBreakerIsolated(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	main := &countingClient{completeErr: errors.New("boom")}
	eng := New(Config{Client: main, Registry: tools.NewRegistry(), Model: "main-model"})
	sub := compact.NewTracker()
	for range compact.MaxConsecutiveFailures {
		runQuery(t, eng, QueryParams{Messages: bigHistory(25_000), ContextWindow: 40_000, CompactTracker: sub, NoWriteBack: true})
	}
	if sub.Snapshot().ConsecutiveFailures != compact.MaxConsecutiveFailures {
		t.Fatalf("sub tracker failures = %d", sub.Snapshot().ConsecutiveFailures)
	}
	impl := eng.(*engineImpl)
	if got := impl.mainTracker.Snapshot().ConsecutiveFailures; got != 0 {
		t.Errorf("main tracker failures = %d, want 0 (sub-agent failures must not count)", got)
	}
}

func TestQuery_ParallelSubAgentsNoRace(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	eng := New(Config{Client: &countingClient{}, Registry: tools.NewRegistry(), Model: "main-model"})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runQuery(t, eng, QueryParams{
				Messages: bigHistory(25_000), Client: &countingClient{}, Model: "glm",
				ContextWindow: 40_000, CompactTracker: compact.NewTracker(), NoWriteBack: true,
			})
		}()
	}
	wg.Wait()
}

func TestQuery_MaxOutputOverrideFromBinding(t *testing.T) {
	tier := &countingClient{}
	eng := New(Config{Client: &countingClient{}, Registry: tools.NewRegistry(), Model: "m", MaxTokens: 32_000})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("hi")}, Client: tier, Model: "glm", MaxOutputTokensOverride: 8192, NoWriteBack: true})
	if got := tier.streams[0].MaxTokens; got != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", got)
	}
}

func TestQuery_FallbackIgnoredWithParamsClient(t *testing.T) {
	tier := &countingClient{streamFn: func(*api.MessageRequest) (api.StreamReader, error) {
		return nil, &api.APIError{StatusCode: 400, Message: "prompt is too long", Kind: api.ErrKindContextWindow}
	}}
	eng := New(Config{Client: &countingClient{}, Registry: tools.NewRegistry(), Model: "m"})
	msgs := runQuery(t, eng, QueryParams{Messages: []types.Message{userText("hi")}, Client: tier, Model: "glm", FallbackModel: "x", NoWriteBack: true})
	if s, _ := tier.counts(); s != 1 {
		t.Errorf("tier streams = %d, want 1 (no fallback retry with a tier client)", s)
	}
	if _, ok := findMsg(msgs, MsgTypeError); !ok {
		t.Error("expected an error message")
	}
}

func TestSetModel_ConcurrentSafe(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	main := &countingClient{}
	eng := New(Config{Client: main, Registry: tools.NewRegistry(), Model: "old", MainContextWindow: 40_000})
	var stop atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !stop.Load() {
			eng.SetModel("old")
		}
	}()
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("hi")}, NoWriteBack: true})
	stop.Store(true)
	<-done

	eng.SetModel("new")
	runQuery(t, eng, QueryParams{Messages: bigHistory(25_000), NoWriteBack: true})
	_, c := main.counts()
	if c != 1 || main.completes[0].Model != "new" {
		t.Errorf("summary requests = %d, want 1 with model new (C5)", c)
	}
}

func TestThreshold_IncludesPromptOverhead(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	main := &countingClient{}
	eng := New(Config{Client: main, Registry: tools.NewRegistry(), Model: "m", MainContextWindow: 40_000})
	runQuery(t, eng, QueryParams{
		Messages:     []types.Message{userText("hi")},
		SystemPrompt: SystemPrompt{Parts: []SystemPromptPart{{Text: strings.Repeat("s", 25_000*4)}}},
		NoWriteBack:  true,
	})
	if _, c := main.counts(); c != 1 {
		t.Errorf("summary requests = %d, want 1 (system prompt counts toward threshold)", c)
	}
}

func TestThreshold_PrefersLastUsage(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	turn := 0
	main := &countingClient{}
	main.streamFn = func(*api.MessageRequest) (api.StreamReader, error) {
		turn++
		if turn == 1 {
			evs := buildToolUseEvents("tu1", "Read", `{}`)
			evs[0].MessageStart.Message.Usage.InputTokens = 30_000
			return newStaticReader(evs...), nil
		}
		return newStaticReader(buildEndTurnEvents("done")...), nil
	}
	reg := newTestRegistry(&stubTool{name: "Read", safe: true, readOnly: true})
	eng := New(Config{Client: main, Registry: reg, Model: "m", MainContextWindow: 40_000})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("hi")}, NoWriteBack: true})
	if _, c := main.counts(); c != 1 {
		t.Errorf("summary requests = %d, want 1 (API usage above threshold)", c)
	}
}
