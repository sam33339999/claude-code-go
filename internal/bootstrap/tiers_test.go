package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tunsuy/glaude/internal/api"
	"github.com/tunsuy/glaude/internal/coordinator"
	"github.com/tunsuy/glaude/internal/engine"
	"github.com/tunsuy/glaude/internal/modeltier"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/pkg/types"
)

// providerClient is an identifiable fake api.Client built by the test factory.
type providerClient struct{ id string }

func (c *providerClient) Stream(context.Context, *api.MessageRequest) (api.StreamReader, error) {
	return nil, errors.New("not used")
}

func (c *providerClient) Complete(context.Context, *api.MessageRequest) (*api.MessageResponse, error) {
	return nil, errors.New("not used")
}

const tierTestConfig = `
[providers.zai]
format      = "openai"
base_url    = "https://api.z.ai/api/paas/v4"
api_key_env = "ZAI_API_KEY"

[providers.openrouter]
format      = "openai"
base_url    = "https://openrouter.ai/api/v1"
api_key_env = "OPENROUTER_API_KEY"

[tiers.lite]
provider   = "zai"
model      = "glm-5.3-flash"
max_tokens = 8192

[tiers.pro]
provider = "openrouter"
model    = "qwen/qwen3.8-27b"

[tiers.foo]
`

var mainTestClient = &providerClient{id: "main"}

// newTestRouter writes config to a temp home and loads a router whose
// provider clients are providerClient{id}.
func newTestRouter(t *testing.T, config string) *tierRouter {
	t.Helper()
	t.Setenv("ZAI_API_KEY", "zk")
	t.Setenv("OPENROUTER_API_KEY", "ok")
	t.Setenv("CLAUDE_CODE_SUBAGENT_MODEL", "")
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".glaude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modeltier.DefaultPath(home), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	factory := func(id string, _ modeltier.Provider, key string) (api.Client, error) {
		if key == "" {
			t.Errorf("factory called for %s without a key", id)
		}
		return &providerClient{id: id}, nil
	}
	r, warnings := loadTierRouter(home, mainTestClient, factory)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	return r
}

func clientID(c api.Client) string {
	if pc, ok := c.(*providerClient); ok {
		return pc.id
	}
	if c == nil {
		return "<nil>"
	}
	return "<other>"
}

func TestResolveSubAgentTarget_TierBound(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	got, err := resolveSubAgentTarget(r, "direct", "claude-parent", "", "lite")
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "glm-5.3-flash" || clientID(got.Client) != "zai" || got.MaxTokens != 8192 {
		t.Errorf("target = %+v (client %s)", got, clientID(got.Client))
	}
}

func TestResolveSubAgentTarget_MainProvider(t *testing.T) {
	r := newTestRouter(t, tierTestConfig+"\n[tiers.max]\nprovider = \"main\"\nmodel = \"claude-haiku-5-5\"\n")
	got, err := resolveSubAgentTarget(r, "direct", "claude-parent", "max", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "claude-haiku-5-5" || got.Client != nil {
		t.Errorf("target = %+v, want model claude-haiku-5-5 with nil (engine default) client", got)
	}
}

func TestResolveSubAgentTarget_UnboundLegacyMapsBack(t *testing.T) {
	r := newTestRouter(t, "")
	got, _ := resolveSubAgentTarget(r, "openai", "qwen4-max", "", "lite")
	if got.Model != "" || got.Client != nil {
		t.Errorf("openai unbound lite = %+v, want inherit", got)
	}
	got, _ = resolveSubAgentTarget(r, "direct", "claude-haiku-4-5", "", "lite")
	if got.Model != "claude-haiku-4-5" {
		t.Errorf("direct unbound lite = %+v, want parent haiku", got)
	}
}

func TestResolveSubAgentTarget_UnboundCustomInherits(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	got, err := resolveSubAgentTarget(r, "direct", "claude-parent", "foo", "")
	if err != nil || got.Model != "" || got.Client != nil {
		t.Errorf("unbound custom tier = %+v %v, want inherit", got, err)
	}
}

func TestResolveSubAgentTarget_NotTierUnchanged(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	got, _ := resolveSubAgentTarget(r, "openai", "qwen4-max", "qwen-turbo", "lite")
	if got.Model != "qwen-turbo" || got.Client != nil {
		t.Errorf("plain model ID = %+v, want qwen-turbo passthrough", got)
	}
}

func TestResolveSubAgentTarget_EnvUnboundTierOnOpenAI(t *testing.T) {
	r := newTestRouter(t, "")
	t.Setenv("CLAUDE_CODE_SUBAGENT_MODEL", "lite")
	got, _ := resolveSubAgentTarget(r, "openai", "qwen4-max", "", "")
	if got.Model != "" {
		t.Errorf("env unbound tier on openai = %q, want inherit", got.Model)
	}
}

func TestResolveSubAgentTarget_EnvBoundTier(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	t.Setenv("CLAUDE_CODE_SUBAGENT_MODEL", "pro")
	got, _ := resolveSubAgentTarget(r, "direct", "claude-parent", "lite", "lite")
	if got.Model != "qwen/qwen3.8-27b" || clientID(got.Client) != "openrouter" {
		t.Errorf("env tier = %+v, want pro binding", got)
	}
}

func TestResolveSubAgentTarget_MissingKeyErrors(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	t.Setenv("ZAI_API_KEY", "")
	r2, _ := loadTierRouter(filepath.Dir(filepath.Dir(r.store.Path())), mainTestClient, nil)
	got, err := resolveSubAgentTarget(r2, "direct", "claude-parent", "", "lite")
	if err == nil || !strings.Contains(err.Error(), "ZAI_API_KEY") || !strings.Contains(err.Error(), "lite") {
		t.Fatalf("err = %v, want one naming ZAI_API_KEY and lite", err)
	}
	if got.Client != nil {
		t.Error("must not fall back to any client")
	}
}

func TestResolveSubAgentTarget_NilRouter(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SUBAGENT_MODEL", "")
	got, _ := resolveSubAgentTarget(nil, "openai", "qwen4-max", "", "lite")
	if got.Model != "" {
		t.Errorf("nil router, openai, profile lite = %q, want inherit", got.Model)
	}
	got, _ = resolveSubAgentTarget(nil, "direct", "claude-parent", "qwen-turbo", "")
	if got.Model != "qwen-turbo" {
		t.Errorf("nil router passthrough = %q", got.Model)
	}
}

func exploreEngine() *stubQueryEngine {
	assistant := textMsg(types.RoleAssistant, "done")
	return &stubQueryEngine{events: []engine.Msg{
		{Type: engine.MsgTypeAssistantMessage, AssistantMsg: &assistant},
	}}
}

func TestBuildRunAgentFn_TierResumeKeepsBinding(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	eng := exploreEngine()
	fn := buildRunAgentFn(eng, tools.NewRegistry(), forkTestState(), nil, forkTestProfiles(), r)
	req := coordinator.SpawnRequest{SubagentType: "explore", Prompt: "look", MaxTurns: coordinator.DefaultMaxTurns}
	if _, err := fn(context.Background(), "a-ex1", req, make(chan string, 1)); err != nil {
		t.Fatal(err)
	}
	if err := r.store.Bind("lite", "openrouter", "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := fn(context.Background(), "a-ex1", req, make(chan string, 1)); err != nil {
		t.Fatal(err)
	}
	first, second := eng.calls[0], eng.calls[1]
	if clientID(first.Client) != "zai" || first.Model != "glm-5.3-flash" || first.MaxOutputTokensOverride != 8192 {
		t.Errorf("first dispatch = client %s model %q max %d", clientID(first.Client), first.Model, first.MaxOutputTokensOverride)
	}
	if clientID(second.Client) != "zai" || second.Model != "glm-5.3-flash" {
		t.Errorf("resume changed binding: client %s model %q", clientID(second.Client), second.Model)
	}
	if first.CompactTracker == nil || first.CompactTracker != second.CompactTracker {
		t.Error("resume must keep the same CompactTracker")
	}
}

func TestBuildRunAgentFn_EachSubAgentOwnTracker(t *testing.T) {
	eng := exploreEngine()
	fn := buildRunAgentFn(eng, tools.NewRegistry(), forkTestState(), nil, forkTestProfiles(), nil)
	req := coordinator.SpawnRequest{SubagentType: "worker", Prompt: "x", MaxTurns: coordinator.DefaultMaxTurns}
	for _, id := range []coordinator.AgentID{"a-1", "a-2"} {
		if _, err := fn(context.Background(), id, req, nil); err != nil {
			t.Fatal(err)
		}
	}
	a, b := eng.calls[0].CompactTracker, eng.calls[1].CompactTracker
	if a == nil || b == nil || a == b {
		t.Errorf("trackers = %p %p, want two distinct non-nil", a, b)
	}
}

func TestBuildRunAgentFn_DispatchErrorOnMissingKey(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	t.Setenv("ZAI_API_KEY", "")
	r2, _ := loadTierRouter(filepath.Dir(filepath.Dir(r.store.Path())), mainTestClient, nil)
	eng := exploreEngine()
	fn := buildRunAgentFn(eng, tools.NewRegistry(), forkTestState(), nil, forkTestProfiles(), r2)
	req := coordinator.SpawnRequest{SubagentType: "explore", Prompt: "look", MaxTurns: coordinator.DefaultMaxTurns}
	if _, err := fn(context.Background(), "a-ex2", req, nil); err == nil || !strings.Contains(err.Error(), "ZAI_API_KEY") {
		t.Errorf("err = %v, want dispatch failure naming ZAI_API_KEY", err)
	}
	if len(eng.calls) != 0 {
		t.Error("query must not run when routing fails")
	}
}

func TestBuildForkQuery_NeverSetsClient(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	req := coordinator.SpawnRequest{SubagentType: "fork", Prompt: "p", Model: "lite", MaxTurns: coordinator.DefaultMaxTurns}
	params, _, err := buildSubAgentQuery(req, tools.NewRegistry(), forkTestProfiles(), "claude-parent", "", nil, &sync.Map{}, "a-f1", r)
	if err != nil {
		t.Fatal(err)
	}
	if params.Client != nil || params.ContextWindow != 0 || params.Model != "claude-parent" {
		t.Errorf("fork params = client %v window %d model %q", params.Client, params.ContextWindow, params.Model)
	}
}

func TestResolveSubAgentTarget_ProviderAddedAfterStartup(t *testing.T) {
	r := newTestRouter(t, tierTestConfig)
	path := r.store.Path()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("\n[providers.late]\nformat = \"openai\"\nbase_url = \"https://late\"\napi_key_env = \"ZAI_API_KEY\"\n")
	f.Close()
	// Any rebind re-reads the file, so "late" becomes a known provider...
	if err := r.store.Bind("pro", "zai", "x"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := r.store.Bind("lite", "late", "m"); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	// ...but it has no client until restart: dispatch must fail, not use main.
	got, err := resolveSubAgentTarget(r, "direct", "claude-parent", "", "lite")
	if err == nil || !strings.Contains(err.Error(), "restart") {
		t.Fatalf("err = %v, want restart hint", err)
	}
	if got.Client != nil || got.Model != "" {
		t.Errorf("target = %+v, want empty", got)
	}
}
