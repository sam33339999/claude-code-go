package api

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
)

// openaiSSEReader implements StreamReader for OpenAI streaming responses.
// It translates OpenAI SSE events to the Anthropic StreamEvent format.
type openaiSSEReader struct {
	resp               *http.Response
	scanner            *bufio.Scanner
	done               bool
	messageID          string
	model              string
	started            bool
	thinkingBlockIndex int                          // allocated block index for reasoning_content (-1 = not started)
	textBlockIndex     int                          // allocated block index for text (-1 = not started)
	toolCalls          map[int]*accumulatedToolCall // OpenAI tool index -> accumulated tool call
	toolBlockIndex     map[int]int                  // OpenAI tool index -> allocated block index
	nextBlockIndex     int                          // next content block index to allocate
	blocksClosed       bool                         // content_block_stop events already emitted
	inputUsage         int                          // accumulated from usage chunks
	cachedUsage        int                          // prompt cache hit tokens from usage chunks
	outputUsage        int                          // completion tokens from usage chunks
	lastStopReason     string                       // stop reason from the finish chunk
	pendingEvents      []*StreamEvent               // events to return before processing more chunks
	debugLogger        *DebugLogger                 // debug logger for SSE event tracing
}

// accumulatedToolCall tracks tool call data being streamed incrementally.
type accumulatedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// newOpenAISSEReader creates a new SSE reader for OpenAI streaming responses.
func newOpenAISSEReader(resp *http.Response, dl *DebugLogger) *openaiSSEReader {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &openaiSSEReader{
		resp:               resp,
		scanner:            scanner,
		thinkingBlockIndex: -1,
		textBlockIndex:     -1,
		toolCalls:          make(map[int]*accumulatedToolCall),
		toolBlockIndex:     make(map[int]int),
		debugLogger:        dl,
	}
}

// Next returns the next SSE event from the OpenAI stream, converted to Anthropic format.
// Returns (nil, io.EOF) when the stream ends normally.
func (r *openaiSSEReader) Next() (*StreamEvent, error) {
	// Return any pending events first (even if done)
	if len(r.pendingEvents) > 0 {
		ev := r.pendingEvents[0]
		r.pendingEvents = r.pendingEvents[1:]
		return ev, nil
	}

	if r.done {
		return nil, io.EOF
	}

	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for data prefix
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end sentinel
		if data == "[DONE]" {
			r.debugLogger.LogSSEEvent("DONE", "[DONE]")
			r.done = true
			// Close any open content blocks (thinking/text/tools).
			r.pendingEvents = append(r.pendingEvents, r.closeOpenBlocks()...)
			// Send message_stop event
			r.pendingEvents = append(r.pendingEvents, &StreamEvent{
				Type: EventMessageStop,
			})
			// Return first pending event
			if len(r.pendingEvents) > 0 {
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}
			return nil, io.EOF
		}

		// Parse the chunk
		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			r.debugLogger.LogError("SSE chunk parse error", err)
			continue // Skip malformed chunks
		}

		// Log the SSE event data.
		r.debugLogger.LogSSEEvent("chunk", data)

		// Debug: log tool calls in chunk.
		if r.debugLogger.Enabled() && len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			tc := chunk.Choices[0].Delta.ToolCalls[0]
			r.debugLogger.Logf("Stream tool call chunk: Index=%d, ID=%q, Name=%q, Args=%q",
				tc.Index, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}

		// Store message metadata
		if chunk.ID != "" {
			r.messageID = chunk.ID
		}
		if chunk.Model != "" {
			r.model = chunk.Model
		}

		// Track usage if provided. OpenAI-compatible providers send usage in a
		// final chunk (often after the finish chunk, with empty choices), so
		// re-emit a message_delta carrying the final counts; the accumulator
		// overwrites the earlier zero-usage snapshot.
		// Some providers (Zhipu GLM) put usage in the SAME chunk as
		// finish_reason; only short-circuit when the chunk carries no choices,
		// otherwise fall through so the finish is processed — its
		// message_delta already carries the counts recorded here.
		if chunk.Usage != nil {
			r.inputUsage = chunk.Usage.uncachedInputTokens()
			r.cachedUsage = chunk.Usage.cachedTokens()
			r.outputUsage = chunk.Usage.CompletionTokens
			if r.started && len(chunk.Choices) == 0 {
				r.pendingEvents = append(r.pendingEvents, r.createUsageDeltaEvent())
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}
		}

		// Send message_start on first chunk
		if !r.started {
			r.started = true
			// Queue message_start event and continue processing the same chunk
			r.pendingEvents = append(r.pendingEvents, r.createMessageStartEvent(&chunk))
		}

		// Process choices
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]

			// Check for finish
			if choice.FinishReason != "" {
				// Close any open content blocks (thinking/text/tools).
				r.pendingEvents = append(r.pendingEvents, r.closeOpenBlocks()...)
				// Queue message_delta event
				r.pendingEvents = append(r.pendingEvents, r.createMessageDeltaEvent(&choice))
				// Return first pending event
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}

			// Process reasoning content (thinking). Reasoning models stream
			// chain-of-thought via the non-standard reasoning_content field;
			// translate it to Anthropic thinking blocks so the engine/TUI
			// display chain works unchanged. Events are queued before text
			// (reasoning precedes the answer); fall through so a chunk carrying
			// both reasoning and content is not lost.
			if choice.Delta.ReasoningContent != "" {
				if r.thinkingBlockIndex < 0 {
					r.thinkingBlockIndex = r.nextBlockIndex
					r.nextBlockIndex++
					r.pendingEvents = append(r.pendingEvents, r.createContentBlockStartEvent(r.thinkingBlockIndex, "thinking"))
				}
				r.pendingEvents = append(r.pendingEvents, r.createThinkingDeltaEvent(r.thinkingBlockIndex, choice.Delta.ReasoningContent))
			}

			// Process delta content
			if choice.Delta.Content != "" {
				if r.textBlockIndex < 0 {
					r.textBlockIndex = r.nextBlockIndex
					r.nextBlockIndex++
					r.pendingEvents = append(r.pendingEvents, r.createContentBlockStartEvent(r.textBlockIndex, "text"))
				}
				r.pendingEvents = append(r.pendingEvents, r.createContentDeltaEvent(r.textBlockIndex, choice.Delta.Content))
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}

			// Process tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				r.processToolCallDelta(&choice.Delta.ToolCalls[0])
				if len(r.pendingEvents) > 0 {
					retEv := r.pendingEvents[0]
					r.pendingEvents = r.pendingEvents[1:]
					return retEv, nil
				}
				continue
			}

			// No content or tool calls, but we may have pending events (e.g., message_start)
			if len(r.pendingEvents) > 0 {
				ev := r.pendingEvents[0]
				r.pendingEvents = r.pendingEvents[1:]
				return ev, nil
			}
		}
	}

	if err := r.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// createMessageStartEvent creates the initial message_start event.
func (r *openaiSSEReader) createMessageStartEvent(_ *openaiStreamChunk) *StreamEvent {
	msgStart := &MessageStartData{
		Message: MessageResponse{
			ID:    r.messageID,
			Type:  "message",
			Role:  "assistant",
			Model: r.model,
			Usage: Usage{
				InputTokens:          r.inputUsage,
				CacheReadInputTokens: r.cachedUsage,
			},
		},
	}
	data, _ := json.Marshal(msgStart)
	return &StreamEvent{
		Type:         EventMessageStart,
		Data:         data,
		MessageStart: msgStart,
	}
}

// createContentBlockStartEvent creates a content_block_start event.
func (r *openaiSSEReader) createContentBlockStartEvent(index int, blockType string) *StreamEvent {
	blockData := struct {
		Index        int          `json:"index"`
		ContentBlock ContentBlock `json:"content_block"`
	}{
		Index: index,
		ContentBlock: ContentBlock{
			Type: blockType,
		},
	}
	data, _ := json.Marshal(blockData)
	return &StreamEvent{
		Type: EventContentBlockStart,
		Data: data,
	}
}

// createContentBlockStopEvent creates a content_block_stop event.
func (r *openaiSSEReader) createContentBlockStopEvent(index int) *StreamEvent {
	stopData := struct {
		Index int `json:"index"`
	}{
		Index: index,
	}
	data, _ := json.Marshal(stopData)
	return &StreamEvent{
		Type: EventContentBlockStop,
		Data: data,
	}
}

// createContentDeltaEvent creates a content_block_delta event for text content.
func (r *openaiSSEReader) createContentDeltaEvent(index int, text string) *StreamEvent {
	delta := &ContentBlockDeltaData{
		Index: index,
		Delta: Delta{
			Type: "text_delta",
			Text: text,
		},
	}
	data, _ := json.Marshal(delta)
	return &StreamEvent{
		Type:              EventContentBlockDelta,
		Data:              data,
		ContentBlockDelta: delta,
	}
}

// createThinkingDeltaEvent creates a content_block_delta event carrying
// chain-of-thought text (Anthropic thinking_delta semantics).
func (r *openaiSSEReader) createThinkingDeltaEvent(index int, thinking string) *StreamEvent {
	delta := &ContentBlockDeltaData{
		Index: index,
		Delta: Delta{
			Type:     "thinking_delta",
			Thinking: thinking,
		},
	}
	data, _ := json.Marshal(delta)
	return &StreamEvent{
		Type:              EventContentBlockDelta,
		Data:              data,
		ContentBlockDelta: delta,
	}
}

// closeOpenBlocks emits content_block_stop for every open block in allocated
// order (thinking, text, tools). Idempotent: finish and [DONE] both call it.
func (r *openaiSSEReader) closeOpenBlocks() []*StreamEvent {
	if r.blocksClosed {
		return nil
	}
	r.blocksClosed = true
	var events []*StreamEvent
	if r.thinkingBlockIndex >= 0 {
		events = append(events, r.createContentBlockStopEvent(r.thinkingBlockIndex))
	}
	if r.textBlockIndex >= 0 {
		events = append(events, r.createContentBlockStopEvent(r.textBlockIndex))
	}
	toolIdxs := make([]int, 0, len(r.toolBlockIndex))
	for _, bi := range r.toolBlockIndex {
		toolIdxs = append(toolIdxs, bi)
	}
	sort.Ints(toolIdxs)
	for _, bi := range toolIdxs {
		events = append(events, r.createContentBlockStopEvent(bi))
	}
	return events
}

// processToolCallDelta processes incremental tool call data.
func (r *openaiSSEReader) processToolCallDelta(tc *openaiToolCall) {
	toolIndex := tc.Index

	acc, exists := r.toolCalls[toolIndex]
	if !exists {
		acc = &accumulatedToolCall{}
		r.toolCalls[toolIndex] = acc
	}

	if tc.ID != "" {
		acc.ID = tc.ID
	}
	if tc.Function.Name != "" {
		acc.Name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		acc.Arguments.WriteString(tc.Function.Arguments)
	}

	blockIndex, exists := r.toolBlockIndex[toolIndex]
	if !exists {
		blockIndex = r.nextBlockIndex
		r.nextBlockIndex++
		r.toolBlockIndex[toolIndex] = blockIndex
		r.pendingEvents = append(r.pendingEvents, r.createToolUseBlockStartEvent(blockIndex, acc.ID, acc.Name))
	}

	if tc.Function.Arguments != "" {
		r.pendingEvents = append(r.pendingEvents, r.createToolCallDeltaEvent(blockIndex, tc.Function.Arguments))
	}
}

// createToolCallDeltaEvent creates a content_block_delta event for tool input JSON.
func (r *openaiSSEReader) createToolCallDeltaEvent(index int, partialJSON string) *StreamEvent {
	delta := &ContentBlockDeltaData{
		Index: index,
		Delta: Delta{
			Type:        "input_json_delta",
			PartialJSON: partialJSON,
		},
	}
	data, _ := json.Marshal(delta)
	return &StreamEvent{
		Type:              EventContentBlockDelta,
		Data:              data,
		ContentBlockDelta: delta,
	}
}

// createToolUseBlockStartEvent creates a content_block_start event for tool use.
func (r *openaiSSEReader) createToolUseBlockStartEvent(index int, id, name string) *StreamEvent {
	blockData := struct {
		Index        int          `json:"index"`
		ContentBlock ContentBlock `json:"content_block"`
	}{
		Index: index,
		ContentBlock: ContentBlock{
			Type: "tool_use",
			ID:   id,
			Name: name,
		},
	}
	data, _ := json.Marshal(blockData)
	return &StreamEvent{
		Type: EventContentBlockStart,
		Data: data,
	}
}

// createMessageDeltaEvent creates the final message_delta event with stop reason.
func (r *openaiSSEReader) createMessageDeltaEvent(choice *openaiStreamChoice) *StreamEvent {
	stopReason := r.convertFinishReason(choice.FinishReason)
	r.lastStopReason = stopReason
	msgDelta := &MessageDeltaData{
		Usage: Usage{
			InputTokens:          r.inputUsage,
			OutputTokens:         r.outputUsage,
			CacheReadInputTokens: r.cachedUsage,
		},
	}
	msgDelta.Delta.StopReason = stopReason
	data, _ := json.Marshal(msgDelta)
	return &StreamEvent{
		Type:         EventMessageDelta,
		Data:         data,
		MessageDelta: msgDelta,
	}
}

// createUsageDeltaEvent re-emits message_delta with the final usage counts
// when the provider's usage chunk arrives after the finish chunk. The stop
// reason is repeated so the accumulator keeps it unchanged.
func (r *openaiSSEReader) createUsageDeltaEvent() *StreamEvent {
	msgDelta := &MessageDeltaData{
		Usage: Usage{
			InputTokens:          r.inputUsage,
			OutputTokens:         r.outputUsage,
			CacheReadInputTokens: r.cachedUsage,
		},
	}
	msgDelta.Delta.StopReason = r.lastStopReason
	data, _ := json.Marshal(msgDelta)
	return &StreamEvent{
		Type:         EventMessageDelta,
		Data:         data,
		MessageDelta: msgDelta,
	}
}

// convertFinishReason converts OpenAI finish_reason to Anthropic stop_reason.
func (r *openaiSSEReader) convertFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn" // Map content filter to end_turn
	default:
		return reason
	}
}

// Close closes the underlying HTTP response body.
func (r *openaiSSEReader) Close() error {
	r.done = true
	return r.resp.Body.Close()
}
