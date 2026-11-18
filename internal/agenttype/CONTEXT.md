---
package: agenttype
import_path: internal/agenttype
layer: infra
generated_at: 2026-11-18T15:25:59Z
source_files: [agenttype.go, builtins.go, loader.go, prompts.go, registry.go, tasktype.go]
---

# internal/agenttype

> Layer: **Infra** · Files: 6 · Interfaces: 0 · Structs: 3 · Functions: 5

## Structs

- **AgentProfile** — 9 fields: Type, DisplayName, Description, WhenToUse, SystemPrompt, ToolFilter, Model, MaxTurns, ...
- **Registry** — 3 fields
- **ToolFilter** — 2 fields: Mode, Tools

## Functions

- `FormatForkDirective(directive string) string`
- `LoadCustomAgents(dir string) ([]*AgentProfile, error)`
- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`
- `ResolveModel(parentModel string, toolModel string, profileModel string) string`

## Constants

- `AgentTypeCustom`
- `AgentTypeExplore`
- `AgentTypeFork`
- `AgentTypeGuide`
- `AgentTypePlan`
- `AgentTypeVerify`
- `AgentTypeWorker`
- `ForkBoilerplate`
- `TaskTypeDream`
- `TaskTypeInProcessTeammate`
- `TaskTypeLocalAgent`
- `TaskTypeLocalBash`
- `TaskTypeLocalWorkflow`
- `TaskTypeMonitorMCP`
- `TaskTypeRemoteAgent`
- `ToolFilterAllowlist`
- `ToolFilterDenylist`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `stubRegistry` in `internal/permissions/checker_test.go`

**Exported type references (files that use types from this package):**
- `AgentProfile` → `internal/bootstrap/agents_cmd.go`, `internal/bootstrap/wire.go`
- `Registry` → `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test)
- `ToolFilter` → `internal/bootstrap/wire.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/tools/tasks`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 fork agent type（feat/coordinator-fork-cache）**：AgentTypeFork 的 SystemPrompt 與 ToolFilter.Tools **刻意為空**——fork 繼承父的 system prompt 與完整工具列表，使請求前綴（tools schema bytes＋system＋歷史）與父 byte-identical，命中 prompt cache（KV-cache 复用，便宜快取輸入）。禁止再委派由 ForkBoilerplate 在 prompt 層 enforcement（fork 看得見 Agent 工具但被禁止使用；bootstrap 側 UseContext.Coordinator=nil 兜底 fail-closed），而非從 schema 移除工具——移除會改 schema bytes、破壞 cache 前綴。來源：Anthropic 洩漏 claude-code/agents/fork.md（one-shot、背景執行、model 繼承、maxTurns 200）。FormatForkDirective 將 boilerplate＋directive 包成尾部 user message（繼承歷史之後），保證 tool_use/tool_result 序列合法。
