package bootstrap

import (
	agtmcp "github.com/tunsuy/glaude/internal/mcp"
	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/internal/tools/agent"
	"github.com/tunsuy/glaude/internal/tools/fileops"
	"github.com/tunsuy/glaude/internal/tools/mcp"
	"github.com/tunsuy/glaude/internal/tools/memory"
	"github.com/tunsuy/glaude/internal/tools/misc"
	"github.com/tunsuy/glaude/internal/tools/shell"
	"github.com/tunsuy/glaude/internal/tools/tasks"
	"github.com/tunsuy/glaude/internal/tools/web"
)

// RegisterBuiltinTools registers all built-in tool singletons into reg.
// Order follows the canonical tool ordering in the TS implementation.
//
// pool is the MCP connection pool injected into the MCP resource tools
// (ListMcpResources / ReadMcpResource). It may be nil, in which case those
// tools report that no MCP pool is configured. Per-server MCP proxy tools are
// registered separately via mcp.RegisterServerTools once connections exist
// (see connectConfiguredMCPServers) — a one-shot, session-pinned registration.
func RegisterBuiltinTools(reg *tools.Registry, pool *agtmcp.Pool) {
	// ── File operations ──────────────────────────────────────────────────────
	reg.Register(fileops.FileReadTool)
	reg.Register(fileops.FileWriteTool)
	reg.Register(fileops.FileEditTool)
	reg.Register(fileops.NotebookEditTool)
	reg.Register(fileops.GrepTool)
	reg.Register(fileops.GlobTool)

	// ── Shell ────────────────────────────────────────────────────────────────
	reg.Register(shell.BashTool)

	// ── Web ──────────────────────────────────────────────────────────────────
	reg.Register(web.WebSearchTool)
	reg.Register(web.WebFetchTool)

	// ── Task management ──────────────────────────────────────────────────────
	reg.Register(tasks.TaskCreateTool)
	reg.Register(tasks.TaskGetTool)
	reg.Register(tasks.TaskListTool)
	reg.Register(tasks.TaskUpdateTool)
	reg.Register(tasks.TaskStopTool)
	reg.Register(tasks.TaskOutputTool)

	// ── Agent ────────────────────────────────────────────────────────────────
	reg.Register(agent.AgentTool)
	reg.Register(agent.SendMessageTool)
	reg.Register(agent.GetAgentStatusTool)

	// ── Interaction / plan mode ──────────────────────────────────────────────
	// TodoWrite, AskUserQuestion, EnterPlanMode, ExitPlanMode, EnterWorktree
	// and ExitWorktree are registered by wire.go after their dependencies
	// exist (app state store / HIL channels); the rest are stateless singletons.

	// ── MCP meta-tools ───────────────────────────────────────────────────────
	reg.Register(mcp.NewListMcpResourcesTool(pool))
	reg.Register(mcp.NewReadMcpResourceTool(pool))

	// ── Misc ─────────────────────────────────────────────────────────────────
	// Skill is registered by wire.go (needs home + live WorkingDir for its
	// search directories).
	reg.Register(misc.BriefTool)
	reg.Register(misc.NewToolSearchTool(reg))
	reg.Register(misc.SleepTool)
	reg.Register(misc.SyntheticOutputTool)

	// ── Memory ────────────────────────────────────────────────────────────────
	reg.Register(memory.MemoryReadTool)
	reg.Register(memory.MemoryWriteTool)
	reg.Register(memory.MemoryDeleteTool)
}
