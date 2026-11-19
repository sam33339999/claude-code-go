package compact

import (
	"github.com/tunsuy/glaude/pkg/types"
)

// SnipResult is the output of a SnipCompactIfNeeded call.
type SnipResult struct {
	// Messages is the (possibly) pruned message list.
	Messages []types.Message
	// TokensFreed is the estimated number of tokens removed.
	TokensFreed int
	// Snipped reports whether this call rewrote history (TokensFreed > 0).
	// A snip moves the prompt prefix, so callers must treat it like a
	// compaction (e.g. discard API-reported prompt usage).
	Snipped bool
}

// snipKeepRecentTurns is the number of most-recent assistant/user turn pairs
// that are never pruned by the snip strategy.
const snipKeepRecentTurns = 5

// SnipTriggerFraction is the fraction of the auto-compact threshold at which
// snip fires. Snipping rewrites the cached prompt prefix (a cache write costs
// far more than the cache reads it saves), so it only runs when the context is
// genuinely close to the limit, and then snips everything at once.
const SnipTriggerFraction = 0.8

// snipPlaceholder replaces snipped tool_result content. It also tells the
// model why the content is gone (there is no separate boundary message).
const snipPlaceholder = "[snipped: earlier tool output removed to reduce context length]"

// SnipCompactIfNeeded clears old tool_use inputs and tool_result contents
// when estTokens reaches SnipTriggerFraction × AutoCompactThreshold(t).
//
// Strategy (Cache-first: a snip is a prefix reset, like a compaction):
//   - Below the trigger, history is returned untouched — the prefix is stable.
//   - At the trigger, every message before the protected tail (the most recent
//     snipKeepRecentTurns pairs) is snipped in one batch.
//   - Already-snipped blocks are skipped, so the call is idempotent.
//
// The operation is purely local (no LLM call required).
func SnipCompactIfNeeded(messages []types.Message, estTokens int, t Target) SnipResult {
	trigger := int(SnipTriggerFraction * float64(AutoCompactThreshold(t)))
	if !needsSnip(messages) || estTokens < trigger {
		return SnipResult{Messages: messages}
	}
	snipped, freed := snipMessages(messages)
	if freed == 0 {
		return SnipResult{Messages: messages}
	}
	return SnipResult{Messages: snipped, TokensFreed: freed, Snipped: true}
}

// needsSnip returns true when the message list is long enough to be worth snipping.
// Threshold: more than (snipKeepRecentTurns * 2 + 2) messages.
func needsSnip(messages []types.Message) bool {
	return len(messages) > snipKeepRecentTurns*2+2
}

// snipMessages removes tool_use inputs and tool_result contents from every
// message before the protected tail, returning the modified list and the
// estimated tokens freed. Input messages are not mutated.
func snipMessages(messages []types.Message) ([]types.Message, int) {
	protectedFromIdx := max(len(messages)-snipKeepRecentTurns*2, 0)
	result := make([]types.Message, len(messages))
	copy(result, messages)
	freed := 0
	for i := range protectedFromIdx {
		msg := messages[i]
		newContent := make([]types.ContentBlock, len(msg.Content))
		for j, blk := range msg.Content {
			switch blk.Type {
			case types.ContentTypeToolUse:
				if blk.Input != nil {
					freed += estimateMapBytes(blk.Input) / 4 // chars → tokens estimate
					blk.Input = nil
				}
			case types.ContentTypeToolResult:
				if isSnipped(blk) {
					break
				}
				for _, c := range blk.Content {
					if c.Text != nil {
						freed += len(*c.Text) / 4
					}
				}
				placeholder := snipPlaceholder
				blk.Content = []types.ContentBlock{{Type: types.ContentTypeText, Text: &placeholder}}
			}
			newContent[j] = blk
		}
		msg.Content = newContent
		result[i] = msg
	}
	return result, freed
}

func isSnipped(blk types.ContentBlock) bool {
	return len(blk.Content) == 1 && blk.Content[0].Text != nil && *blk.Content[0].Text == snipPlaceholder
}

// estimateMapBytes returns a rough byte estimate for a map[string]any by
// counting key + value string lengths.
func estimateMapBytes(m map[string]any) int {
	total := 0
	for k, v := range m {
		total += len(k)
		if s, ok := v.(string); ok {
			total += len(s)
		} else {
			total += 8 // rough estimate for non-string values
		}
	}
	return total
}
