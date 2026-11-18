package agenttype

import (
	"testing"
)

func TestRegisterBuiltins(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)

	// Should have exactly 6 built-in types.
	if r.Len() != 6 {
		t.Errorf("Len() = %d, want 6", r.Len())
	}

	expectedTypes := []AgentType{
		AgentTypeWorker,
		AgentTypeExplore,
		AgentTypePlan,
		AgentTypeVerify,
		AgentTypeGuide,
		AgentTypeFork,
	}

	for _, at := range expectedTypes {
		t.Run(string(at), func(t *testing.T) {
			p, ok := r.Get(at)
			if !ok {
				t.Fatalf("built-in type %q not registered", at)
			}
			if p.DisplayName == "" {
				t.Error("DisplayName is empty")
			}
			if p.Description == "" {
				t.Error("Description is empty")
			}
			if p.ToolFilter.Mode == "" {
				t.Error("ToolFilter.Mode is empty")
			}
			// Fork intentionally inherits the parent's system prompt and
			// full tool list (prompt-cache byte identity); see
			// TestBuiltin_ForkProfile.
			if at != AgentTypeFork {
				if p.SystemPrompt == "" {
					t.Error("SystemPrompt is empty")
				}
				if len(p.ToolFilter.Tools) == 0 {
					t.Error("ToolFilter.Tools is empty")
				}
			}
		})
	}
}

func TestBuiltin_ForkProfile(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)
	p := r.MustGet(AgentTypeFork)

	// Cache identity: a fork inherits the parent's system prompt and full
	// tool list, so both must be empty in the profile.
	if p.SystemPrompt != "" {
		t.Error("Fork SystemPrompt must be empty (inherits parent's byte-for-byte)")
	}
	if len(p.ToolFilter.Tools) != 0 {
		t.Error("Fork ToolFilter.Tools must be empty (inherits parent's full tool list)")
	}
	if p.EffectiveModel() != "" {
		t.Errorf("Fork must inherit parent model, got %q", p.EffectiveModel())
	}
	if p.CanSpawnSubAgents {
		t.Error("Fork must not spawn sub-agents")
	}
	if p.MaxTurns != 200 {
		t.Errorf("Fork MaxTurns = %d, want 200", p.MaxTurns)
	}
}

func TestBuiltin_WorkerProfile(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)
	p := r.MustGet(AgentTypeWorker)

	if p.ToolFilter.Mode != ToolFilterDenylist {
		t.Errorf("Worker ToolFilter.Mode = %q, want %q", p.ToolFilter.Mode, ToolFilterDenylist)
	}
	if p.CanSpawnSubAgents {
		t.Error("Worker should not be able to spawn sub-agents")
	}
}

func TestBuiltin_ExploreProfile(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)
	p := r.MustGet(AgentTypeExplore)

	if p.ToolFilter.Mode != ToolFilterAllowlist {
		t.Errorf("Explore ToolFilter.Mode = %q, want %q", p.ToolFilter.Mode, ToolFilterAllowlist)
	}

	// Explore should have Read, Glob, Grep, Bash.
	allowed := make(map[string]bool)
	for _, tool := range p.ToolFilter.Tools {
		allowed[tool] = true
	}
	for _, need := range []string{"Read", "Glob", "Grep", "Bash"} {
		if !allowed[need] {
			t.Errorf("Explore missing tool %q", need)
		}
	}
	if p.Model != "haiku" {
		t.Errorf("Explore Model = %q, want %q", p.Model, "haiku")
	}
	if p.MaxTurns != 20 {
		t.Errorf("Explore MaxTurns = %d, want 20", p.MaxTurns)
	}
}

func TestBuiltin_NoneCanSpawnSubAgents(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	RegisterBuiltins(r)

	for _, p := range r.All() {
		if p.CanSpawnSubAgents {
			t.Errorf("built-in type %q should not be able to spawn sub-agents", p.Type)
		}
	}
}
