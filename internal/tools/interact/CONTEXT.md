---
package: interact
import_path: internal/tools/interact
layer: tools
generated_at: 2026-11-18T04:49:22Z
source_files: [doc.go, interact.go, worktree.go]
---

# internal/tools/interact

> Layer: **Tools** · Files: 3 · Interfaces: 0 · Structs: 6 · Functions: 2

## Structs

- **AskUserQuestionInput** — 2 fields: Question, Options
- **EnterPlanModeInput** — 1 fields: Plan
- **EnterWorktreeInput** — 1 fields: Name
- **ExitPlanModeInput** — 1 fields: Approved
- **ExitWorktreeInput** — 2 fields: Action, DiscardChanges
- **TodoWriteInput** — 1 fields: Todos

## Functions

- `NewAskUserQuestionTool(askCh chan<- types.QuestionRequest, respCh <-chan types.QuestionResponse) tools.Tool`
- `NewTodoWriteTool(store *state.AppStateStore) tools.Tool`

## Dependencies

**Imports:** `internal/state`, `internal/tools`, `pkg/types`, `pkg/utils/ids`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。
- **2026-11-18 BF-07 TodoWrite 实现**：单例桩改为 NewTodoWriteTool(store *state.AppStateStore) 构造注入（tools→state 向下依赖合法）；列表存 AppState.Todos，整表替换；oracle 语义：全 completed 清空、固定确认文案（TodoWriteTool.ts）；校验 id/content 非空＋status 枚举，非法不改状态。schema 字节不变（cache gate）。分歧：单主会话列表、无磁盘持久化、无 verification nudge。
- **2026-11-18 BF-08 AskUserQuestion 实现**：单例桩改为 NewAskUserQuestionTool(askCh, respCh) HIL channel 注入（pkg/types.QuestionRequest/Response）；Call 三段 select（送达/等待 × ctx/abort），dismiss 为正常结果非错误；nil channel（headless）诚实报错。权限对话框管线同构，schema 字节不变。
