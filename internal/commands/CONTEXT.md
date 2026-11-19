---
package: commands
import_path: internal/commands
layer: cli
generated_at: 2026-11-19T13:49:59Z
source_files: [builtins.go, mcp.go, registry.go, resume.go, terminal_setup.go, tier.go]
---

# internal/commands

> Layer: **CLI** · Files: 6 · Interfaces: 2 · Structs: 6 · Functions: 5

## Interfaces

### MCPServerLister (1 methods)
> MCPServerLister is the read-only view of the MCP connection pool required

```go
type MCPServerLister interface {
    GetAll() []*mcp.ServerConnection
}
```

### TierManager (5 methods)
> TierManager is the tier table view /tier needs. *modeltier.Store

```go
type TierManager interface {
    List() []modeltier.Binding
    Providers() []modeltier.ProviderStatus
    Bind(tier string, providerID string, modelID string) error
    Unbind(tier string) error
    MainContextWindow() int
}
```

## Structs

- **Command** — 3 fields: Name, Description, Execute
- **CommandContext** — 10 fields: WorkingDir, Model, SessionID, DarkMode, VimEnabled, Effort, MessageCount, TokensInput, ...
- **Registry** — 2 fields
- **Result** — 14 fields: Text, Display, ShouldExit, NewMessages, NewSessionID, NewTheme, NewModel, NewEffort, ...
- **SessionSummary** — 5 fields: ID, Title, ModTime, MessageCount, Current
- **TerminalInfo** — 3 fields: Terminal, Shell, Term

## Functions

- `DetectTerminal(lookup func(string) string) TerminalInfo`
- `FormatMCPStatus(ctx context.Context, pool MCPServerLister) string`
- `FormatTerminalSetup(info TerminalInfo) string`
- `NewRegistry() *Registry`
- `RegisterBuiltins(r *Registry)`

## Constants

- `DisplayDialog`
- `DisplayError`
- `DisplayMessage`
- `DisplayNone`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `stubRegistry` in `internal/permissions/checker_test.go`

**Exported type references (files that use types from this package):**
- `Command` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `CommandContext` → `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `MCPServerLister` → `internal/tui/cmds.go`, `internal/tui/init.go`, `internal/tui/mcp_resume_test.go` (test), `internal/tui/model.go`
- `Registry` → `internal/tui/completion.go`, `internal/tui/model.go`
- `Result` → `internal/tui/mcp_resume_test.go` (test), `internal/tui/tiers_test.go` (test), `internal/tui/tui_test.go` (test), `internal/tui/update.go`
- `SessionSummary` → `internal/tui/mcp_resume_test.go` (test), `internal/tui/resumepicker.go`
- `TierManager` → `internal/bootstrap/tiers.go`, `internal/tui/init.go`, `internal/tui/model.go`

## Dependencies

**Imports:** `internal/mcp`, `internal/memdir`, `internal/modeltier`, `internal/session`, `pkg/types`, `pkg/utils/fs`

**Imported by:** `internal/bootstrap`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 BF-03（/mcp、/resume 真实实现）**：移除两处用户可见桩。新增 `mcp.go`（`MCPServerLister` 只读池视图接口 + `FormatMCPStatus` 渲染，connected server 经 `ListTools` 附工具数，每 server 2s 超时、失败降级）与 `resume.go`（经 agtkeel `session`/`utils/fs` 扫描 `<cwd>/.glaude/projects/<hash>/*.jsonl`，列表 + 按 ID 恢复，ID 路径穿越校验，损坏条目跳过）。`Result` 新增 `FetchMCPStatus`（/mcp 异步报告标志，仿 TriggerDream 先例）与 `NewSessionID`（/resume 采用恢复会话 ID）；`NewMessages` 契约明确为携带 `[]types.Message`、由 TUI 类型断言消费。/mcp 的 Execute 不做网络 I/O（报告由 TUI 的 tea.Cmd 异步生成）；/resume 的文件 I/O 与既有 /memory 同步模式一致。本包新增对 agtkeel `mcp`/`session`/`types`/`utils/fs` 的向下依赖，未依赖 bootstrap/internal/tools。设计详见 `docs/project/design/tui.md` §4.1。
- **2026-11-18 BF-03-F1 评审修复（CR-BF03-2/3/4）**：`MCPServerLister`/`FormatMCPStatus` GoDoc 钉住「GetAll 返回活指针、读取期间禁止并发池突变」约束（快照根治待跨仓 BF-19；BF-01 并发写者落地前必须关闭，否则升级 P0）；渲染侧每连接值拷贝一次作廉价防御（非同步手段，注释明示）。/resume 的 `session.Resume` 底层错误经 %w 保留于友好文案（用户可见文案不变），`mgr.Close` 失败显式返回错误（与 listSessions 模式一致）。
- **2026-11-18 BF-05 /config＋/terminal-setup 桩回填**：cmdConfig 改返回 Result{OpenDialog:"config"}（TUI 既有 wiring），面板只读（编辑经 /model /theme /vim /effort 与设置文件，交互编辑为后续项）。新增 terminal_setup.go：DetectTerminal(lookup) 纯函数环境检测（12 种终端识别，未知诚实报告），FormatTerminalSetup 非侵入指引（Ctrl+J 多行提示＋cobra completion 指引），不写文件不改终端设置（与原版键绑定安装的差距已在 tui.md v1.3 文档化）。
- **2026-11-19 `/tier`**：`TierManager` 介面（`*modeltier.Store` 直接實作），經 `CommandContext.Tiers` 注入（commands 不得 import bootstrap）。改綁定在 Execute 內同步寫檔（小檔案，比照其他同步指令），下一個派出的 sub-agent 生效。等級表是唯一狀態來源，TUI 不需另外更新狀態。
