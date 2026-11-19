package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/internal/tools/agent"
)

// TestGetAgentStatus_ResolvesName guards the live-fire fix: GetAgentStatus
// must resolve a human-readable agent_name to the generated AgentID before
// the result lookup (parity with SendMessage). Previously it passed the name
// straight to the ID-keyed map and returned "agent not found".
func TestGetAgentStatus_ResolvesName(t *testing.T) {
	mc := &mockCoordinator{
		resolveFn: func(_ context.Context, target string) (string, error) {
			if target == "code-reader" {
				return "aworker-abc123", nil
			}
			return "", errors.New("unknown agent " + target)
		},
		resultFn: func(_ context.Context, agentID string) (string, string, error) {
			if agentID == "aworker-abc123" {
				return "the summary", "completed", nil
			}
			return "", "", errors.New("unknown agent " + agentID)
		},
	}
	uctx := &tools.UseContext{Ctx: context.Background(), Coordinator: mc}

	result, err := agent.GetAgentStatusTool.Call(tools.Input(`{"agent_id":"code-reader"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.IsError {
		t.Fatalf("name lookup must succeed, got error: %v", result.Content)
	}
	var out struct {
		AgentID string `json:"agent_id"`
		Status  string `json:"status"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content.(string)), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.AgentID != "aworker-abc123" {
		t.Errorf("output agent_id = %q, want resolved ID aworker-abc123", out.AgentID)
	}
	if out.Status != "completed" || out.Result != "the summary" {
		t.Errorf("status/result = %q/%q, want completed/the summary", out.Status, out.Result)
	}
}

func TestGetAgentStatus_UnknownNameErrors(t *testing.T) {
	mc := &mockCoordinator{
		resolveFn: func(_ context.Context, target string) (string, error) {
			return "", errors.New("unknown agent " + target)
		},
	}
	uctx := &tools.UseContext{Ctx: context.Background(), Coordinator: mc}

	result, err := agent.GetAgentStatusTool.Call(tools.Input(`{"agent_id":"nope"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !result.IsError {
		t.Fatal("unknown name must produce an error result")
	}
	if !strings.Contains(result.Content.(string), "agent not found") {
		t.Errorf("error content = %v, want 'agent not found'", result.Content)
	}
}

// TestGetAgentStatus_IDStillWorks verifies direct ID lookup is unchanged.
func TestGetAgentStatus_IDStillWorks(t *testing.T) {
	mc := &mockCoordinator{
		resultFn: func(_ context.Context, agentID string) (string, string, error) {
			if agentID == "aworker-xyz" {
				return "", "running", nil
			}
			return "", "", errors.New("unknown agent " + agentID)
		},
	}
	uctx := &tools.UseContext{Ctx: context.Background(), Coordinator: mc}

	result, err := agent.GetAgentStatusTool.Call(tools.Input(`{"agent_id":"aworker-xyz"}`), uctx, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.IsError {
		t.Fatalf("ID lookup must succeed: %v", result.Content)
	}
	if !strings.Contains(result.Content.(string), "running") {
		t.Errorf("content = %v, want running status", result.Content)
	}
}
