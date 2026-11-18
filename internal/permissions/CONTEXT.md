---
package: permissions
import_path: internal/permissions
layer: services
generated_at: 2026-11-18T05:05:54Z
source_files: [ask.go, checker.go, denial.go, filesystem.go, persist.go]
---

# internal/permissions

> Layer: **Services** · Files: 5 · Interfaces: 1 · Structs: 9 · Functions: 3

## Interfaces

### Checker (3 methods)
> Checker is the top-level permission pipeline interface.

```go
type Checker interface {
    CanUseTool(ctx context.Context, toolName string, input tools.Input, tctx *tools.UseContext) (tools.PermissionResult, error)
    RequestPermission(ctx context.Context, req PermissionRequest, tctx *tools.UseContext) (tools.PermissionResult, error)
    GetDenialCount() int
}
```

## Structs

- **AskRequest** — 8 fields: ID, ToolName, ToolUseID, Message, Input, Suggestions, BlockedPath, ProjectPath
- **AskResponse** — 4 fields: ID, Decision, AlwaysAllow, UserModified
- **CheckerConfig** — 7 fields: PermCtx, Dispatcher, Registry, AskCh, RespCh, Persist, ModeFn
- **DangerousPathResult** — 3 fields: Dangerous, Matched, Reason
- **DenialLimits** — 2 fields: MaxConsecutive, MaxTotal
- **DenialRecord** — 4 fields: ToolName, ToolUseID, Reason, DeniedAt
- **DenialTrackingState** — 4 fields: DenialCount, LastDeniedAt, RecentDenials
- **PermissionRequest** — 4 fields: ToolName, ToolUseID, Input, ToolResult
- **PersistConfig** — 1 fields: ProjectDir

## Functions

- `CheckDangerousPath(path string) DangerousPathResult`
- `IsDangerousPath(path string) bool`
- `NewChecker(cfg CheckerConfig) Checker`

## Change Impact

**Exported type references (files that use types from this package):**
- `AskRequest` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `AskResponse` → `internal/bootstrap/wire.go`, `internal/tui/init.go`, `internal/tui/model.go`
- `Checker` → `internal/bootstrap/wire.go`, `internal/engine/engine.go`, `internal/engine/orchestration.go`
- `CheckerConfig` → `internal/bootstrap/wire.go`
- `PermissionRequest` → `internal/engine/orchestration.go`
- `PersistConfig` → `internal/bootstrap/wire.go`

## Dependencies

**Imports:** `internal/config`, `internal/hooks`, `internal/tools`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/engine`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 内核搬回（反转 PR #31）**：本包原于 2026-09-23 逐字提取至外部仓 github.com/tunsuy/agtkeel；因该仓停止维护，经人类决策整併回本仓原始路径。来源：agtkeel main 3a77085＋后续提交，零逻辑改动（仅 import 路径恢复＋gofmt）。内核改动自此直接在本仓进行。
- **2026-11-18 BF-09/BF-21 ModeFn**：CheckerConfig.ModeFn 可选 live mode 来源（覆盖 PermCtx.Mode 快照）；currentMode() 于 bypass 与 mode-switch 两处消费；nil 时行为不变。锁序 checker.mu→store.mu 无死锁。Checker 接口零变更。
