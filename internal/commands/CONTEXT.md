---
package: commands
import_path: internal/commands
layer: cli
generated_at: 2026-11-18T01:17:44Z
source_files: [builtins.go, mcp.go, registry.go, resume.go]
---

# internal/commands

> Layer: **CLI** · Files: 4 · Interfaces: 1 · Structs: 4 · Functions: 3

## Interfaces

### MCPServerLister (1 methods)
> MCPServerLister is the read-only view of the MCP connection pool required

```go
type MCPServerLister interface {
    GetAll() []*mcp.ServerConnection
}
```

## Structs

- **Command** — 3 fields: Name, Description, Execute
- **CommandContext** — 9 fields: WorkingDir, Model, SessionID, DarkMode, VimEnabled, Effort, MessageCount, TokensInput, ...
- **Registry** — 2 fields
- **Result** — 13 fields: Text, Display, ShouldExit, NewMessages, NewSessionID, NewTheme, NewModel, NewEffort, ...

## Functions

- `FormatMCPStatus(ctx context.Context, pool MCPServerLister) string`
- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`

## Constants

- `DisplayDialog`
- `DisplayError`
- `DisplayMessage`
- `DisplayNone`

## Change Impact

**Exported type references (files that use types from this package):**
- `Command` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `CommandContext` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `MCPServerLister` → `internal/tui/cmds.go`, `internal/tui/init.go`, `internal/tui/mcp_resume_test.go` (test), `internal/tui/model.go`
- `Registry` → `internal/tui/model.go`
- `Result` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`

## Dependencies

**Imports:** `internal/memdir`

**Imported by:** `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 BF-03（/mcp、/resume 真实实现）**：移除两处用户可见桩。新增 `mcp.go`（`MCPServerLister` 只读池视图接口 + `FormatMCPStatus` 渲染，connected server 经 `ListTools` 附工具数，每 server 2s 超时、失败降级）与 `resume.go`（经 agtkeel `session`/`utils/fs` 扫描 `<cwd>/.claude/projects/<hash>/*.jsonl`，列表 + 按 ID 恢复，ID 路径穿越校验，损坏条目跳过）。`Result` 新增 `FetchMCPStatus`（/mcp 异步报告标志，仿 TriggerDream 先例）与 `NewSessionID`（/resume 采用恢复会话 ID）；`NewMessages` 契约明确为携带 `[]types.Message`、由 TUI 类型断言消费。/mcp 的 Execute 不做网络 I/O（报告由 TUI 的 tea.Cmd 异步生成）；/resume 的文件 I/O 与既有 /memory 同步模式一致。本包新增对 agtkeel `mcp`/`session`/`types`/`utils/fs` 的向下依赖，未依赖 bootstrap/internal/tools。设计详见 `docs/project/design/tui.md` §4.1。
