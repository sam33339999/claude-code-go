package compact

import "github.com/tunsuy/glaude/pkg/types"

// imageTokenEstimate approximates the prompt cost of one image. Providers
// bill images by pixel area (Anthropic ≈ w×h/750, capped near 1.6k tokens
// after server-side downscaling), not by base64 length.
const imageTokenEstimate = 1600

// imagePlaceholder replaces images in summary requests (text in, text out;
// the summariser may not accept images).
const imagePlaceholder = "[image]"

// withoutImages returns a copy of messages with every image block (top
// level or inside a tool_result) replaced by imagePlaceholder text, and the
// number of images replaced. The input is never mutated.
func withoutImages(messages []types.Message) ([]types.Message, int) {
	total := 0
	var out []types.Message
	for i, m := range messages {
		blocks, n := replaceImageBlocks(m.Content)
		if n == 0 {
			if out != nil {
				out[i] = m
			}
			continue
		}
		if out == nil {
			out = make([]types.Message, len(messages))
			copy(out, messages[:i])
		}
		m.Content = blocks
		out[i] = m
		total += n
	}
	if out == nil {
		return messages, 0
	}
	return out, total
}

// replaceImageBlocks returns a copy of blocks with images replaced, or the
// original slice and 0 when it holds none.
func replaceImageBlocks(blocks []types.ContentBlock) ([]types.ContentBlock, int) {
	total := 0
	var out []types.ContentBlock
	for i, b := range blocks {
		n := 0
		switch {
		case b.Type == types.ContentTypeImage:
			b = types.ContentBlock{Type: types.ContentTypeText, Text: strPtr(imagePlaceholder)}
			n = 1
		case len(b.Content) > 0:
			var inner []types.ContentBlock
			if inner, n = replaceImageBlocks(b.Content); n > 0 {
				b.Content = inner
			}
		}
		if n == 0 {
			if out != nil {
				out[i] = b
			}
			continue
		}
		if out == nil {
			out = make([]types.ContentBlock, len(blocks))
			copy(out, blocks[:i])
		}
		out[i] = b
		total += n
	}
	if out == nil {
		return blocks, 0
	}
	return out, total
}
