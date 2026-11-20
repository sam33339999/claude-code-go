package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/msgqueue"
	"github.com/tunsuy/glaude/internal/permissions"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// callCountTool records how many times Call ran.
type callCountTool struct {
	stubTool
	calls atomic.Int32
}

func (c *callCountTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	c.calls.Add(1)
	return &tools.Result{Content: "ok"}, nil
}

// toolThenDone streams one tool_use for name, then an end_turn reply.
func toolThenDone(name string) *countingClient {
	c := &countingClient{}
	turn := 0
	c.streamFn = func(*api.MessageRequest) (api.StreamReader, error) {
		turn++
		if turn == 1 {
			return newStaticReader(buildToolUseEvents("tu1", name, `{}`)...), nil
		}
		return newStaticReader(buildEndTurnEvents("done")...), nil
	}
	return c
}

// lastToolResultText returns the text of the tool_result sent in the second request.
func lastToolResultText(t *testing.T, c *countingClient) string {
	t.Helper()
	if len(c.streams) < 2 {
		t.Fatalf("expected a second request, got %d", len(c.streams))
	}
	msgs := c.streams[1].Messages
	raw := string(msgs[len(msgs)-1].Content)
	return raw
}

func TestDrainMidTurn_OnlyMainLoop(t *testing.T) {
	q := msgqueue.NewMessageQueue()
	q.Enqueue(msgqueue.NewCommand("queued for main", msgqueue.ModePrompt, msgqueue.PriorityNext))
	c := toolThenDone("Read")
	reg := newTestRegistry(&stubTool{name: "Read", safe: true, readOnly: true})
	eng := New(Config{Client: c, Registry: reg, Model: "m", MsgQueue: q})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("side")}, NoWriteBack: true})
	if q.Len() != 1 {
		t.Fatalf("a NoWriteBack query consumed the main loop's queue (len %d)", q.Len())
	}
	for _, r := range c.streams {
		for _, m := range r.Messages {
			if strings.Contains(string(m.Content), "queued for main") {
				t.Fatal("main-loop message leaked into a NoWriteBack query")
			}
		}
	}

	c2 := toolThenDone("Read")
	eng2 := New(Config{Client: c2, Registry: reg, Model: "m", MsgQueue: q})
	runQuery(t, eng2, QueryParams{Messages: []types.Message{userText("main")}})
	if q.Len() != 0 {
		t.Errorf("the main loop must drain its queue (len %d)", q.Len())
	}
	if len(c2.streams) < 2 {
		t.Fatal("expected a second request")
	}
	last := c2.streams[1].Messages[len(c2.streams[1].Messages)-1].Content
	var blocks []types.ContentBlock
	if err := json.Unmarshal(last, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].Text == nil || *blocks[0].Text != "queued for main" {
		t.Fatalf("nil snapshot must stay one joined text block, got %s", last)
	}
}

func TestDrainMidTurn_SnapshotExpandsUserKeepsBTWLiteral(t *testing.T) {
	q := msgqueue.NewMessageQueue()
	marker := "see [Pasted text #2]"
	user := msgqueue.NewCommand(marker, msgqueue.ModePrompt, msgqueue.PriorityNext)
	user.Content = []types.ContentBlock{
		{Type: types.ContentTypeImage, Source: &types.ImageSource{Type: "base64", MediaType: "image/png", Data: "QUJD"}},
		{Type: types.ContentTypeText, Text: strPtr("the cat sat")},
	}
	q.Enqueue(user)
	btw := msgqueue.NewCommand("note [Image #9]", msgqueue.ModePrompt, msgqueue.PriorityNext)
	btw.Origin = msgqueue.OriginBTW
	q.Enqueue(btw)

	c := toolThenDone("Read")
	reg := newTestRegistry(&stubTool{name: "Read", safe: true, readOnly: true})
	eng := New(Config{Client: c, Registry: reg, Model: "m", MsgQueue: q})
	msgs := runQuery(t, eng, QueryParams{Messages: []types.Message{userText("main")}})
	if q.Len() != 0 {
		t.Fatalf("queue len %d, want 0", q.Len())
	}

	var note string
	for _, msg := range msgs {
		if msg.Type != MsgTypeUserMessage || msg.UserMsg == nil {
			continue
		}
		var text string
		image := false
		for _, b := range msg.UserMsg.Content {
			if b.Type == types.ContentTypeImage {
				image = true
			}
			if b.Text != nil {
				text += *b.Text
			}
		}
		if !strings.Contains(text, marker) {
			continue
		}
		if image || strings.Contains(text, "the cat sat") {
			t.Fatalf("TUI notification expanded: %+v", msg.UserMsg.Content)
		}
		note = text
	}
	wantNote := marker + "\n\n" + "note [Image #9]"
	if note != wantNote {
		t.Fatalf("TUI notification = %q, want %q", note, wantNote)
	}

	if len(c.streams) < 2 {
		t.Fatal("expected a second request")
	}
	raw := c.streams[1].Messages[len(c.streams[1].Messages)-1].Content
	var blocks []types.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 4 {
		t.Fatalf("engine blocks = %s", raw)
	}
	if blocks[0].Type != types.ContentTypeImage || blocks[0].Source == nil || blocks[0].Source.Data != "QUJD" {
		t.Fatalf("image block = %+v", blocks[0])
	}
	if blocks[1].Text == nil || *blocks[1].Text != "the cat sat" {
		t.Fatalf("expanded text = %+v", blocks[1])
	}
	if blocks[2].Text == nil || *blocks[2].Text != "\n\n" {
		t.Fatalf("separator = %+v", blocks[2])
	}
	if blocks[3].Text == nil || *blocks[3].Text != "note [Image #9]" || blocks[3].Source != nil {
		t.Fatalf("btw block = %+v", blocks[3])
	}
}

func TestToolFilter_BlocksExecutionKeepsSchema(t *testing.T) {
	w := &callCountTool{stubTool: stubTool{name: "Write"}}
	c := toolThenDone("Write")
	eng := New(Config{Client: c, Registry: newTestRegistry(w), Model: "m"})
	runQuery(t, eng, QueryParams{
		Messages: []types.Message{userText("x")}, NoWriteBack: true,
		ToolFilter: func(t tools.Tool, _ tools.Input) (bool, string) {
			return t.Name() != "Write", "Write is not available in /btw (read-only)"
		},
	})
	if w.calls.Load() != 0 {
		t.Error("filtered tool executed")
	}
	if got := lastToolResultText(t, c); !strings.Contains(got, "not available in /btw") {
		t.Errorf("tool_result = %s", got)
	}
	if len(c.streams[0].Tools) != 1 || c.streams[0].Tools[0].Name != "Write" {
		t.Error("tool schema must still include the filtered tool (cache identity)")
	}
}

func TestToolFilter_AllowsReadOnly(t *testing.T) {
	r := &callCountTool{stubTool: stubTool{name: "Read", safe: true, readOnly: true}}
	eng := New(Config{Client: toolThenDone("Read"), Registry: newTestRegistry(r), Model: "m"})
	runQuery(t, eng, QueryParams{
		Messages: []types.Message{userText("x")}, NoWriteBack: true,
		ToolFilter: func(t tools.Tool, in tools.Input) (bool, string) { return t.IsReadOnly(in), "" },
	})
	if r.calls.Load() != 1 {
		t.Errorf("allowed tool calls = %d", r.calls.Load())
	}
}

// askChecker answers Ask from CanUseTool and records whether the interactive
// flow (RequestPermission) was used.
type askChecker struct{ requested atomic.Int32 }

func (a *askChecker) CanUseTool(context.Context, string, tools.Input, *tools.UseContext) (tools.PermissionResult, error) {
	return tools.PermissionResult{Behavior: tools.PermissionAsk}, nil
}
func (a *askChecker) RequestPermission(context.Context, permissions.PermissionRequest, *tools.UseContext) (tools.PermissionResult, error) {
	a.requested.Add(1)
	return tools.PermissionResult{Behavior: tools.PermissionAllow}, nil
}
func (a *askChecker) GetDenialCount() int { return 0 }

func TestNonInteractivePermissions_AskBecomesDeny(t *testing.T) {
	r := &callCountTool{stubTool: stubTool{name: "Read", safe: true, readOnly: true}}
	chk := &askChecker{}
	c := toolThenDone("Read")
	eng := New(Config{Client: c, Registry: newTestRegistry(r), Model: "m", PermissionChecker: chk})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("x")}, NoWriteBack: true, NonInteractivePermissions: true})
	if chk.requested.Load() != 0 || r.calls.Load() != 0 {
		t.Errorf("interactive flow used (%d) or tool ran (%d)", chk.requested.Load(), r.calls.Load())
	}
	if got := lastToolResultText(t, c); !strings.Contains(got, "needs permission") {
		t.Errorf("tool_result = %s", got)
	}
}

func TestNoCompaction_SkipsAllCompaction(t *testing.T) {
	t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW", "")
	c := &countingClient{}
	eng := New(Config{Client: c, Registry: tools.NewRegistry(), Model: "m", MainContextWindow: 40_000})
	runQuery(t, eng, QueryParams{Messages: bigHistory(32_000), NoWriteBack: true, NoCompaction: true})
	if _, n := c.counts(); n != 0 {
		t.Errorf("summary requests = %d, want 0", n)
	}
}

func TestBTW_RequestPrefixMatchesMain(t *testing.T) {
	reg := newTestRegistry(&stubTool{name: "Read"}, &stubTool{name: "Write"})
	eng := New(Config{Client: &countingClient{}, Registry: reg, Model: "m"}).(*engineImpl)
	sys := SystemPrompt{Parts: []SystemPromptPart{{Text: "stable", CacheControl: "ephemeral"}, {Text: "tail"}}}
	hist := []types.Message{userText("a"), {Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.ContentTypeText, Text: strPtr("b")}}}, userText("c")}
	uctx := &tools.UseContext{Ctx: context.Background()}
	mainReq, err := eng.buildRequestWithModel(QueryParams{SystemPrompt: sys, ToolUseContext: uctx}, hist, "m")
	if err != nil {
		t.Fatal(err)
	}
	btwParams := QueryParams{SystemPrompt: sys, ToolUseContext: uctx, NoWriteBack: true, NoCompaction: true, NonInteractivePermissions: true,
		ToolFilter: func(tools.Tool, tools.Input) (bool, string) { return false, "" }}
	btwReq, err := eng.buildRequestWithModel(btwParams, append(append([]types.Message(nil), hist...), userText("side question")), "m")
	if err != nil {
		t.Fatal(err)
	}
	same := func(a, b any) bool { x, _ := json.Marshal(a); y, _ := json.Marshal(b); return string(x) == string(y) }
	if !same(mainReq.Tools, btwReq.Tools) || !same(mainReq.SystemBlocks, btwReq.SystemBlocks) {
		t.Error("btw tools/system differ from the main request")
	}
	// The history before the last user message must match byte for byte
	// (the rolling cache breakpoint moves to btw's own last user message).
	for i := 0; i < len(hist)-1; i++ {
		if string(mainReq.Messages[i].Content) != string(btwReq.Messages[i].Content) {
			t.Errorf("message %d differs", i)
		}
	}
}

// Review I-1: per-query switches must not leak into the caller's UseContext.
func TestQuerySwitches_DoNotMutateCallerUseContext(t *testing.T) {
	uctx := &tools.UseContext{Ctx: context.Background()}
	eng := New(Config{Client: &countingClient{}, Registry: tools.NewRegistry(), Model: "m"})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("x")}, NoWriteBack: true, ToolUseContext: uctx,
		NonInteractivePermissions: true, ToolFilter: func(tools.Tool, tools.Input) (bool, string) { return false, "" }})
	if uctx.ExecFilter != nil || uctx.NonInteractivePermissions {
		t.Error("caller's UseContext was mutated by a filtered query")
	}
}

// Review I-2: switches apply even when the caller passes no UseContext.
func TestQuerySwitches_NoUseContextFailsClosed(t *testing.T) {
	w := &callCountTool{stubTool: stubTool{name: "Write"}}
	eng := New(Config{Client: toolThenDone("Write"), Registry: newTestRegistry(w), Model: "m"})
	ch, err := eng.Query(context.Background(), QueryParams{Messages: []types.Message{userText("x")}, NoWriteBack: true,
		ToolFilter: func(tools.Tool, tools.Input) (bool, string) { return false, "blocked" }})
	if err != nil {
		t.Fatal(err)
	}
	drainMsgs(ch)
	if w.calls.Load() != 0 {
		t.Error("ToolFilter ignored without a UseContext (fail-open)")
	}
}

// Review: the snapshot slot is safe to read from another goroutine while the
// loop writes it.
func TestSnapshotSlot_ConcurrentRead(t *testing.T) {
	slot := &tools.SnapshotSlot{}
	reg := newTestRegistry(&stubTool{name: "Read", safe: true, readOnly: true})
	eng := New(Config{Client: toolThenDone("Read"), Registry: reg, Model: "m"})
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = slot.Load()
			}
		}
	}()
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("x")}, ToolUseContext: &tools.UseContext{Ctx: context.Background(), Snapshot: slot}})
	close(stop)
	if snap, ok := slot.Load().(*CacheSafeParams); !ok || snap.Model != "m" {
		t.Errorf("slot = %+v", slot.Load())
	}
}

// Final review I-3: in append-only mode TodoWrite runs under the real
// permission checker in default mode (it never prompts, so Ask must not
// turn it into a denial).
func TestNonInteractive_TodoWriteAppendAllowed(t *testing.T) {
	todo := &callCountTool{stubTool: stubTool{name: "TodoWrite"}}
	chk := &askChecker{}
	eng := New(Config{Client: toolThenDone("TodoWrite"), Registry: newTestRegistry(todo), Model: "m", PermissionChecker: chk})
	runQuery(t, eng, QueryParams{Messages: []types.Message{userText("x")}, NoWriteBack: true, NonInteractivePermissions: true,
		ToolUseContext: &tools.UseContext{Ctx: context.Background(), TodoAppendOnly: true}})
	if todo.calls.Load() != 1 || chk.requested.Load() != 0 {
		t.Errorf("TodoWrite calls %d, interactive %d", todo.calls.Load(), chk.requested.Load())
	}
}
