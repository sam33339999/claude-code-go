package api

import (
	"net/http"
	"strings"
	"time"
)

// Provider enumerates supported API providers.
type Provider string

const (
	ProviderDirect  Provider = "direct"
	ProviderBedrock Provider = "bedrock"
	ProviderVertex  Provider = "vertex"
	ProviderFoundry Provider = "foundry"
	ProviderOpenAI  Provider = "openai"
)

// ClientConfig aggregates client configuration.
type ClientConfig struct {
	Provider       Provider
	APIKey         string
	BaseURL        string
	MaxRetries     int
	TimeoutSeconds int
	// RatePolicy sets client-side pacing/concurrency limits. When empty,
	// NewClient falls back to CLAUDE_CODE_API_RPM / CLAUDE_CODE_API_MAX_CONCURRENT.
	RatePolicy RatePolicy
	// CustomHeaders from ANTHROPIC_CUSTOM_HEADERS env var
	CustomHeaders map[string]string
	// Bedrock-specific
	AWSRegion string
	// Vertex-specific
	GCPProject string
	GCPRegion  string
	// OpenAI-specific
	OpenAIOrganization string // Optional organization ID
	OpenAIProject      string // Optional project ID
	// Debug options
	Debug     bool   // Enable debug logging
	DebugFile string // Write debug log to this file path (empty = stderr)
}

// defaultBaseURL is the Anthropic API base URL.
const defaultBaseURL = "https://api.anthropic.com"

// defaultOpenAIBaseURL is the OpenAI API base URL.
const defaultOpenAIBaseURL = "https://api.openai.com"

// appVersion is the application version string embedded in User-Agent.
const appVersion = "0.1.0"

// buildDefaultHeaders returns the default HTTP headers for Anthropic API requests.
func buildDefaultHeaders(version string) map[string]string {
	return map[string]string{
		"User-Agent": "glaude/" + version,
	}
}

// normalizeBaseURL removes a trailing version segment from baseURL so that
// clients can append their own versioned path without duplicating it.
//
// Both clients append a versioned path ("/v1/messages", "/v1/chat/completions"),
// but most OpenAI-compatible endpoints document their base URL with the version
// already included (e.g. "https://host/compatible-mode/v1"). Without this trim the
// request would go to ".../v1/v1/chat/completions" and the server answers 404.
func normalizeBaseURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	return strings.TrimSuffix(trimmed, "/v1")
}

// NewClient creates an API client based on the provided configuration.
func NewClient(cfg ClientConfig, httpClient *http.Client) (Client, error) {
	cfg.BaseURL = normalizeBaseURL(cfg.BaseURL)
	if httpClient == nil {
		timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
		if timeout == 0 {
			timeout = 600 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		if cfg.Provider == ProviderOpenAI {
			baseURL = defaultOpenAIBaseURL
		} else {
			baseURL = defaultBaseURL
		}
	}

	headers := buildDefaultHeaders(appVersion)
	for k, v := range cfg.CustomHeaders {
		headers[k] = v
	}

	client, err := buildProviderClient(cfg, httpClient, headers, baseURL)
	if err != nil {
		return nil, err
	}

	// Uniform client-side rate control at the provider boundary: explicit
	// config wins, otherwise environment variables apply. With no policy the
	// client is returned unwrapped (zero overhead).
	policy := cfg.RatePolicy
	if policy.RPM <= 0 && policy.MaxConcurrent <= 0 {
		policy = ratePolicyFromEnv()
	}
	return applyRatePolicy(client, policy), nil
}

// buildProviderClient constructs the provider-specific client.
func buildProviderClient(cfg ClientConfig, httpClient *http.Client, headers map[string]string, baseURL string) (Client, error) {
	switch cfg.Provider {
	case ProviderDirect, "":
		return &directClient{
			apiKey:     cfg.APIKey,
			baseURL:    baseURL,
			httpClient: httpClient,
			headers:    headers,
		}, nil
	case ProviderBedrock:
		return newBedrockClient(cfg, httpClient, headers)
	case ProviderVertex:
		return newVertexClient(cfg, httpClient, headers)
	case ProviderFoundry:
		return &directClient{
			apiKey:     cfg.APIKey,
			baseURL:    baseURL,
			httpClient: httpClient,
			headers:    headers,
		}, nil
	case ProviderOpenAI:
		return newOpenAIClient(cfg, httpClient, headers)
	default:
		return &directClient{
			apiKey:     cfg.APIKey,
			baseURL:    baseURL,
			httpClient: httpClient,
			headers:    headers,
		}, nil
	}
}

// bedrockClient delegates to directClient with AWS-signed requests.
// Full Bedrock signing is out of scope; this is a structural placeholder
// that uses AWS environment credentials via standard chain.
type bedrockClient struct {
	directClient
	awsRegion string
}

func newBedrockClient(cfg ClientConfig, httpClient *http.Client, headers map[string]string) (Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://bedrock-runtime." + cfg.AWSRegion + ".amazonaws.com"
	}
	return &bedrockClient{
		directClient: directClient{
			apiKey:     cfg.APIKey,
			baseURL:    baseURL,
			httpClient: httpClient,
			headers:    headers,
		},
		awsRegion: cfg.AWSRegion,
	}, nil
}

// vertexClient delegates to directClient with GCP auth.
type vertexClient struct {
	directClient
	gcpProject string
	gcpRegion  string
}

func newVertexClient(cfg ClientConfig, httpClient *http.Client, headers map[string]string) (Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://" + cfg.GCPRegion + "-aiplatform.googleapis.com/v1/projects/" + cfg.GCPProject + "/locations/" + cfg.GCPRegion + "/publishers/anthropic"
	}
	return &vertexClient{
		directClient: directClient{
			apiKey:     cfg.APIKey,
			baseURL:    baseURL,
			httpClient: httpClient,
			headers:    headers,
		},
		gcpProject: cfg.GCPProject,
		gcpRegion:  cfg.GCPRegion,
	}, nil
}
