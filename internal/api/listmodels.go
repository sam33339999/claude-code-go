package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// ModelListing is one entry from a provider's model list.
type ModelListing struct {
	// ID is the model identifier as the endpoint expects it.
	ID string
	// ContextWindow is the reported context length (0 = not reported).
	ContextWindow int
}

// ListModelsErrorKind classifies a ListModels failure for display.
type ListModelsErrorKind string

// ListModels error kinds.
const (
	ListModelsUnreachable  ListModelsErrorKind = "unreachable"
	ListModelsUnauthorized ListModelsErrorKind = "unauthorized"
	ListModelsForbidden    ListModelsErrorKind = "forbidden"
	ListModelsNotSupported ListModelsErrorKind = "not_supported"
	ListModelsTimeout      ListModelsErrorKind = "timeout"
	ListModelsHTTP         ListModelsErrorKind = "http"
)

// ListModelsError is returned by ListModels. Message never contains the API key.
type ListModelsError struct {
	Kind       ListModelsErrorKind
	StatusCode int
	Message    string
}

// Error implements error.
func (e *ListModelsError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("list models: %s (HTTP %d): %s", e.Kind, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("list models: %s: %s", e.Kind, e.Message)
}

// listModelsMaxPages caps Anthropic pagination.
const listModelsMaxPages = 10

// ListModels fetches the model list from cfg's endpoint with GET /v1/models.
// It verifies reachability and credentials without calling an LLM (no tokens
// are spent). Results are sorted by ID. A nil httpClient uses
// http.DefaultClient; the caller bounds the call with ctx.
func ListModels(ctx context.Context, cfg ClientConfig, httpClient *http.Client) ([]ModelListing, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	base := normalizeBaseURL(cfg.BaseURL)
	var models []ModelListing
	var err error
	switch cfg.Provider {
	case ProviderDirect, "":
		if base == "" {
			base = defaultBaseURL
		}
		models, err = listAnthropicModels(ctx, httpClient, base, cfg.APIKey)
	case ProviderOpenAI:
		if base == "" {
			base = defaultOpenAIBaseURL
		}
		models, err = listOpenAIModels(ctx, httpClient, base, cfg.APIKey)
	default:
		return nil, &ListModelsError{Kind: ListModelsNotSupported, Message: fmt.Sprintf("provider %q cannot list models", cfg.Provider)}
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, nil
}

func listAnthropicModels(ctx context.Context, c *http.Client, base, key string) ([]ModelListing, error) {
	headers := map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}
	var out []ModelListing
	after := ""
	for range listModelsMaxPages {
		u := base + "/v1/models?limit=1000"
		if after != "" {
			u += "&after_id=" + url.QueryEscape(after)
		}
		var page struct {
			Data *[]struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := getJSON(ctx, c, u, headers, key, &page); err != nil {
			return nil, err
		}
		if page.Data == nil {
			return nil, notAModelList()
		}
		for _, m := range *page.Data {
			out = append(out, ModelListing{ID: m.ID})
		}
		if !page.HasMore || page.LastID == "" {
			break
		}
		after = page.LastID
	}
	return out, nil
}

func listOpenAIModels(ctx context.Context, c *http.Client, base, key string) ([]ModelListing, error) {
	var resp struct {
		Data *[]struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if err := getJSON(ctx, c, base+"/v1/models", map[string]string{"Authorization": "Bearer " + key}, key, &resp); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		return nil, notAModelList()
	}
	out := make([]ModelListing, 0, len(*resp.Data))
	for _, m := range *resp.Data {
		out = append(out, ModelListing{ID: m.ID, ContextWindow: max(m.ContextLength, 0)})
	}
	return out, nil
}

// notAModelList reports a 200 response without a "data" array.
func notAModelList() error {
	return &ListModelsError{Kind: ListModelsNotSupported, StatusCode: http.StatusOK, Message: "response is not a model list"}
}

// getJSON performs a GET and decodes a JSON object into v, classifying
// failures. key is redacted from any error text before it is truncated.
func getJSON(ctx context.Context, c *http.Client, u string, headers map[string]string, key string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return &ListModelsError{Kind: ListModelsUnreachable, Message: err.Error()}
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := c.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &ListModelsError{Kind: ListModelsTimeout, Message: "request timed out"}
		}
		return &ListModelsError{Kind: ListModelsUnreachable, Message: redact(err.Error(), key)}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		kind := ListModelsHTTP
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			kind = ListModelsUnauthorized
		case http.StatusForbidden:
			kind = ListModelsForbidden
		case http.StatusNotFound:
			kind = ListModelsNotSupported
		}
		return &ListModelsError{Kind: kind, StatusCode: resp.StatusCode, Message: snippet(redact(string(body), key))}
	}
	if err := json.Unmarshal(body, v); err != nil {
		return &ListModelsError{Kind: ListModelsNotSupported, StatusCode: resp.StatusCode, Message: "response is not a model list"}
	}
	return nil
}

// snippet trims s to at most 200 runes for display.
func snippet(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}

// redact removes key from s (endpoints sometimes echo the Authorization
// header back). It must run before snippet so a key spanning the cut is
// still found whole.
func redact(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "[redacted]")
}
