// Package agenttype defines agent type profiles, tool filters, and a thread-safe
// registry for built-in and custom agent types.
//
// This package sits at the same layer as internal/config — both coordinator/ and
// bootstrap/ may import it without creating dependency cycles.
package agenttype

import (
	"os"
	"strings"
)

// AgentType is the canonical string identifier for a sub-agent kind.
type AgentType string

const (
	// AgentTypeWorker is the default general-purpose agent.
	AgentTypeWorker AgentType = "worker"
	// AgentTypeExplore is a read-only exploration agent.
	AgentTypeExplore AgentType = "explore"
	// AgentTypePlan produces structured implementation plans without making changes.
	AgentTypePlan AgentType = "plan"
	// AgentTypeVerify runs tests and linting to validate changes.
	AgentTypeVerify AgentType = "verify"
	// AgentTypeGuide helps with Claude Code usage and documentation.
	AgentTypeGuide AgentType = "guide"
	// AgentTypeFork forks the parent agent: it inherits the parent's full
	// conversation context, system prompt, tool list, and model so the request
	// prefix hits the parent's prompt cache. One-shot background execution.
	AgentTypeFork AgentType = "fork"
	// AgentTypeCustom is for user-defined agent types loaded from .glaude/agents/.
	AgentTypeCustom AgentType = "custom"
)

// String returns the string representation of the agent type.
func (t AgentType) String() string { return string(t) }

// IsBuiltin returns true if the agent type is a built-in type.
func (t AgentType) IsBuiltin() bool {
	switch t {
	case AgentTypeWorker, AgentTypeExplore, AgentTypePlan, AgentTypeVerify, AgentTypeGuide, AgentTypeFork:
		return true
	default:
		return false
	}
}

// ToolFilterMode specifies how tool access is controlled.
type ToolFilterMode string

const (
	// ToolFilterAllowlist only permits the listed tools.
	ToolFilterAllowlist ToolFilterMode = "allowlist"
	// ToolFilterDenylist permits all tools except the listed ones.
	ToolFilterDenylist ToolFilterMode = "denylist"
)

// ToolFilter specifies which tools an agent type may or may not use.
type ToolFilter struct {
	// Mode is "allowlist" or "denylist".
	Mode ToolFilterMode `json:"mode" yaml:"mode"`
	// Tools is the list of tool names for the selected mode.
	Tools []string `json:"list" yaml:"list"`
}

// AgentProfile defines the full configuration for an agent type.
type AgentProfile struct {
	// Type is the canonical agent type identifier.
	Type AgentType `json:"type" yaml:"type"`
	// DisplayName is a human-readable name for TUI display.
	DisplayName string `json:"display_name" yaml:"display_name"`
	// Description is a short description of the agent's purpose.
	Description string `json:"description" yaml:"description"`
	// WhenToUse explains when the main agent should invoke this type.
	WhenToUse string `json:"when_to_use" yaml:"when_to_use"`
	// SystemPrompt is the system prompt template for this agent type.
	SystemPrompt string `json:"system_prompt" yaml:"system_prompt"`
	// ToolFilter defines the tool access policy.
	ToolFilter ToolFilter `json:"tools" yaml:"tools"`
	// Model overrides the default model; empty means inherit parent.
	// Supported values: "", "inherit", a tier name (lite / pro / max, or a
	// custom tier from ~/.glaude/config.toml), a legacy alias (haiku / sonnet /
	// opus, mapped to lite / pro / max), a full model ID, or
	// "providerID:modelID" for a provider registered in config.toml.
	Model string `json:"model" yaml:"model"`
	// MaxTurns default; 0 means use coordinator default (30).
	MaxTurns int `json:"max_turns" yaml:"max_turns"`
	// CanSpawnSubAgents controls whether this type can use Agent/SendMessage tools.
	CanSpawnSubAgents bool `json:"can_spawn_sub_agents" yaml:"can_spawn_sub_agents"`
}

// EffectiveModel returns the model to use, resolving "inherit" and "" to empty
// (meaning inherit from parent).
func (p *AgentProfile) EffectiveModel() string {
	if p.Model == "" || p.Model == "inherit" {
		return ""
	}
	return p.Model
}

// ResolveModel applies the sub-agent model selection policy for Anthropic
// providers (direct/bedrock/vertex/foundry). See ResolveModelForProvider.
func ResolveModel(parentModel, toolModel, profileModel string) string {
	return ResolveModelForProvider("", parentModel, toolModel, profileModel)
}

// ResolveModelForProvider applies the sub-agent model selection policy.
//
// Priority:
//  1. CLAUDE_CODE_SUBAGENT_MODEL (explicit user override, passed through as-is)
//  2. tool-specified model
//  3. agent profile model
//  4. inherit parent model (empty return value)
//
// Short aliases (haiku/sonnet/opus) are Anthropic family names. On Anthropic
// providers they inherit the exact parent model when it belongs to that
// family, otherwise they pass through for the API layer to resolve. On the
// openai provider they are meaningless (the endpoint serves other models and
// answers 404 "Model not exist" for them), so they fall back to inheriting
// the parent model — the only model known to be valid on that endpoint.
// Full model IDs always pass through unchanged on every provider.
func ResolveModelForProvider(provider, parentModel, toolModel, profileModel string) string {
	openai := strings.EqualFold(strings.TrimSpace(provider), "openai")
	if envModel := strings.TrimSpace(os.Getenv("CLAUDE_CODE_SUBAGENT_MODEL")); envModel != "" {
		if openai {
			// Explicit user override on a custom endpoint: pass through as-is
			// (the user knows which models their endpoint serves).
			return envModel
		}
		return resolveModelAlias(parentModel, envModel)
	}
	if strings.TrimSpace(toolModel) != "" {
		return resolveModelAliasForProvider(openai, parentModel, toolModel)
	}
	if profileModel == "" || profileModel == "inherit" {
		return ""
	}
	return resolveModelAliasForProvider(openai, parentModel, profileModel)
}

func resolveModelAliasForProvider(openai bool, parentModel, model string) string {
	model = strings.TrimSpace(model)
	if model == "" || model == "inherit" {
		return ""
	}
	switch strings.ToLower(model) {
	case "haiku", "sonnet", "opus":
		if openai {
			// Anthropic alias on a non-Anthropic endpoint: inherit the parent.
			return ""
		}
		if strings.Contains(strings.ToLower(parentModel), strings.ToLower(model)) {
			return parentModel
		}
	}
	return model
}

// ResolveAliasForProvider resolves a single model value (alias or full ID)
// for provider, with the same alias rules as ResolveModelForProvider but
// without consulting CLAUDE_CODE_SUBAGENT_MODEL. Used by tier routing to
// fall back to legacy aliases for unbound tiers.
func ResolveAliasForProvider(provider, parentModel, model string) string {
	openai := strings.EqualFold(strings.TrimSpace(provider), "openai")
	return resolveModelAliasForProvider(openai, parentModel, model)
}

func resolveModelAlias(parentModel, model string) string {
	return resolveModelAliasForProvider(false, parentModel, model)
}

// CoordinatorOnlyTools is the list of tools that should generally be excluded
// from sub-agents to prevent infinite recursion and unauthorized coordinator
// operations.
var CoordinatorOnlyTools = []string{
	"Agent",
	"SendMessage",
	"GetAgentStatus",
	"TaskCreate",
	"TaskGet",
	"TaskList",
	"TaskUpdate",
	"TaskStop",
	"TaskOutput",
	"EnterPlanMode",
	"ExitPlanMode",
}
