package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/pkg/types"
)

// snipTarget is a 200k-window target; its snip trigger is
// SnipTriggerFraction × AutoCompactThreshold.
var snipTarget = Target{Model: "claude-sonnet-5-5", MaxOutputTokens: 8192}

func snipTrigger() int {
	return int(SnipTriggerFraction * float64(AutoCompactThreshold(snipTarget)))
}

func seedPairs(n int) []types.Message {
	var msgs []types.Message
	for i := range n {
		id := fmt.Sprintf("seed-%d", i)
		msgs = append(msgs, toolUseMsg(id, "Read", map[string]any{"path": "/f" + id}))
		msgs = append(msgs, toolResultMsg(id, "tool output for "+id))
	}
	return msgs
}

// simulateSnipTurns mirrors the engine loop: each turn the snip result
// becomes the persisted history, then one assistant tool_use + user
// tool_result pair is appended. estFor(turn) supplies the prompt estimate.
// It returns each turn's request payload (JSON per message) and whether that
// turn snipped.
func simulateSnipTurns(t *testing.T, turns int, estFor func(turn int) int) ([][]string, []bool) {
	t.Helper()
	msgs := seedPairs(7)
	var requests [][]string
	var snipped []bool
	for turn := range turns {
		res := SnipCompactIfNeeded(msgs, estFor(turn), snipTarget)
		msgs = res.Messages
		requests = append(requests, marshalEach(t, msgs))
		snipped = append(snipped, res.Snipped)
		id := fmt.Sprintf("turn-%d", turn)
		msgs = append(msgs, toolUseMsg(id, "Read", map[string]any{"path": "/f" + id}))
		msgs = append(msgs, toolResultMsg(id, "tool output for "+id))
	}
	return requests, snipped
}

func marshalEach(t *testing.T, msgs []types.Message) []string {
	t.Helper()
	out := make([]string, len(msgs))
	for i, m := range msgs {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out[i] = string(b)
	}
	return out
}

// assertPrefix fails when request n is not a message-wise prefix of n+1.
func assertPrefix(t *testing.T, requests [][]string, from int) {
	t.Helper()
	for n := from; n+1 < len(requests); n++ {
		prev, next := requests[n], requests[n+1]
		if len(next) < len(prev) {
			t.Fatalf("turn %d→%d: history shrank", n, n+1)
		}
		for i := range prev {
			if prev[i] != next[i] {
				t.Errorf("turn %d→%d: prefix message %d rewritten (cache break)", n, n+1, i)
				break
			}
		}
	}
}

// TestSnip_PrefixStableBelowTrigger pins the Cache-first rule
// (docs/PROMPT_CACHE_AND_COST.md): only compaction may rewrite the prefix.
// Below the trigger, snip must never touch history.
func TestSnip_PrefixStableBelowTrigger(t *testing.T) {
	requests, snipped := simulateSnipTurns(t, 6, func(int) int { return snipTrigger() - 1 })
	assertPrefix(t, requests, 0)
	for turn, s := range snipped {
		if s {
			t.Errorf("turn %d snipped below trigger", turn)
		}
	}
}

// TestSnip_SingleBatchAtTrigger: crossing the trigger rewrites history once,
// snipping everything before the protected tail; later turns are stable.
func TestSnip_SingleBatchAtTrigger(t *testing.T) {
	est := func(turn int) int {
		if turn == 3 {
			return snipTrigger()
		}
		return snipTrigger() - 1
	}
	requests, snipped := simulateSnipTurns(t, 6, est)
	if !snipped[3] {
		t.Fatal("turn 3 should snip")
	}
	at := requests[3]
	protectedFrom := len(at) - snipKeepRecentTurns*2
	for i := range protectedFrom {
		if strings.Contains(at[i], "tool output for") {
			t.Errorf("message %d before protected tail not snipped: %s", i, at[i])
		}
	}
	for i := protectedFrom; i < len(at); i++ {
		if strings.Contains(at[i], snipPlaceholder) {
			t.Errorf("protected message %d was snipped", i)
		}
	}
	assertPrefix(t, requests, 3)
}

func TestSnip_NoBoundaryMessage(t *testing.T) {
	requests, _ := simulateSnipTurns(t, 6, func(int) int { return snipTrigger() })
	for turn, req := range requests {
		if want := 14 + 2*turn; len(req) != want {
			t.Errorf("turn %d: %d messages, want %d (no extra boundary messages)", turn, len(req), want)
		}
		for _, m := range req {
			if strings.Contains(m, "Earlier tool call inputs and outputs have been removed") {
				t.Fatalf("turn %d: boundary message present", turn)
			}
		}
	}
}

func TestSnip_PlaceholderText(t *testing.T) {
	res := SnipCompactIfNeeded(seedPairs(7), snipTrigger(), snipTarget)
	blk := res.Messages[1].Content[0]
	if len(blk.Content) != 1 || blk.Content[0].Text == nil ||
		*blk.Content[0].Text != "[snipped: earlier tool output removed to reduce context length]" {
		t.Errorf("placeholder = %+v", blk.Content)
	}
}

func TestSnip_IdempotentOnSnipped(t *testing.T) {
	first := SnipCompactIfNeeded(seedPairs(7), snipTrigger(), snipTarget)
	before := marshalEach(t, first.Messages)
	again := SnipCompactIfNeeded(first.Messages, snipTrigger(), snipTarget)
	if again.TokensFreed != 0 || again.Snipped {
		t.Errorf("second snip freed %d (snipped=%v), want 0/false", again.TokensFreed, again.Snipped)
	}
	after := marshalEach(t, again.Messages)
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("message %d changed on idempotent snip", i)
		}
	}
}

// TestSnip_PrefixStableWhileAboveTrigger: once history has been snipped, an
// estimate that stays in [trigger, threshold) must not snip again every turn
// (that would recreate C9); the next prefix reset belongs to auto-compaction.
func TestSnip_PrefixStableWhileAboveTrigger(t *testing.T) {
	requests, snipped := simulateSnipTurns(t, 6, func(int) int { return snipTrigger() })
	if !snipped[0] {
		t.Fatal("turn 0 should snip")
	}
	for turn := 1; turn < len(snipped); turn++ {
		if snipped[turn] {
			t.Errorf("turn %d snipped again while above trigger", turn)
		}
	}
	assertPrefix(t, requests, 0)
}
