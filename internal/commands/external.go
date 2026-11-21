package commands

import (
	"fmt"
	"strings"
)

// ExternalAgentCommand describes an external ACP agent exposed as /<name>.
type ExternalAgentCommand struct {
	Name        string
	Description string
}

// ExternalAgentSpawner starts a background delegation to the named external
// agent and returns the agent ID.
type ExternalAgentSpawner func(name, task string) (string, error)

// RegisterExternalAgents registers one /<name> command per external agent.
// "/grok <task>" hands the task to that agent's long-lived session; progress
// streams in the agent panel and the result returns to the conversation.
// Names already taken by another command are skipped.
func RegisterExternalAgents(r *Registry, agents []ExternalAgentCommand, spawn ExternalAgentSpawner) {
	if spawn == nil {
		return
	}
	for _, a := range agents {
		if a.Name == "" || r.Lookup(a.Name) != nil {
			continue
		}
		name := a.Name
		r.Register(&Command{
			Name:        name,
			Description: fmt.Sprintf("Delegate a task to the external %s agent (one long-lived session): /%s <task>", name, name),
			Execute: func(_ CommandContext, args string) Result {
				task := strings.TrimSpace(args)
				if task == "" {
					return Result{Text: fmt.Sprintf("Usage: /%s <task>", name), Display: DisplayError}
				}
				id, err := spawn(name, task)
				if err != nil {
					return Result{Text: fmt.Sprintf("/%s: %v", name, err), Display: DisplayError}
				}
				return Result{
					Text:    fmt.Sprintf("Delegated to %s (%s). Progress shows in the agent panel; the result returns to this conversation when it finishes.", name, id),
					Display: DisplayMessage,
				}
			},
		})
	}
}
