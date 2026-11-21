package agent

import (
	"strings"
	"sync"
)

// ExternalAgentInfo describes an external coding agent (reached over the
// Agent Client Protocol) that agent_type can name.
type ExternalAgentInfo struct {
	Name        string
	Description string
}

var (
	externalMu     sync.RWMutex
	externalAgents []ExternalAgentInfo
)

// SetExternalAgents sets the external agents listed in the Agent tool
// description. Bootstrap calls it once at startup; the text stays stable for
// the process lifetime so the tool schema bytes (prompt cache) do not change.
func SetExternalAgents(infos []ExternalAgentInfo) {
	externalMu.Lock()
	defer externalMu.Unlock()
	externalAgents = append([]ExternalAgentInfo(nil), infos...)
}

// externalAgentsSection renders the description block for external agents,
// or "" when none are configured.
func externalAgentsSection() string {
	externalMu.RLock()
	defer externalMu.RUnlock()
	if len(externalAgents) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nExternal agents (another coding agent driven over the Agent Client Protocol, with its own model and tools):\n")
	for _, a := range externalAgents {
		b.WriteString("- \"" + a.Name + "\" — " + a.Description + "\n")
	}
	b.WriteString("Each external agent keeps ONE long-lived session for this glaude process: every delegation to the same agent continues that session, so it remembers earlier tasks and the files it read. Brief it with the task itself; system_prompt, allowed_tools, max_turns and model do not apply to external agents. Its tool calls go through the user's permission rules.")
	return b.String()
}
