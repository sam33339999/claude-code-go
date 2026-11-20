package api

import (
	"errors"
	"regexp"
)

// toolImagePlaceholder is the tool-message text for a result that holds only
// images (role "tool" cannot carry them; they follow in a user message).
const toolImagePlaceholder = "[image returned by tool; attached in the next message]"

// toolImagesNote leads the user message that carries tool-result images.
const toolImagesNote = "Images returned by the tool call(s) above:"

// imagePart converts an Anthropic image source into an OpenAI image_url part.
// Base64 sources become data URLs; URL sources pass through.
func imagePart(src *ImageSource) (openaiContentPart, bool) {
	if src == nil {
		return openaiContentPart{}, false
	}
	var url string
	switch {
	case src.Type == "base64" && src.Data != "":
		mediaType := src.MediaType
		if mediaType == "" {
			mediaType = "image/png"
		}
		url = "data:" + mediaType + ";base64," + src.Data
	case src.Type == "url" && src.URL != "":
		url = src.URL
	default:
		return openaiContentPart{}, false
	}
	return openaiContentPart{Type: "image_url", ImageURL: &openaiImageURL{URL: url}}, true
}

// imageUnsupportedRe matches provider messages rejecting image input, e.g.
// z.ai "messages.content.type is invalid, allowed values: ['text']", OpenAI
// "image_url is only supported by certain models", DeepSeek "unknown variant
// `image_url`, expected `text`".
var imageUnsupportedRe = regexp.MustCompile(`(?i)image|content\.type|content type|expected .?text`)

// IsImageUnsupportedError reports whether err is a 400 rejecting the request
// because the model does not accept image input. The check is a heuristic
// on the provider's message; callers should only act on it when the request
// actually carried images.
func IsImageUnsupportedError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		return false
	}
	return imageUnsupportedRe.MatchString(apiErr.Message)
}
