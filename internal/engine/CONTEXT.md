---
package: engine
import_path: internal/engine
layer: core
generated_at: 2026-11-19T18:19:04Z
source_files: [budget.go, cache_params.go, compaction.go, engine.go, forked_agent.go, msg.go, orchestration.go, query.go, stop_hooks.go]
---

# internal/engine

> Layer: **Core** · Files: 9 · Interfaces: 1 · Structs: 15 · Functions: 9

## Interfaces

### QueryEngine (7 methods)
> QueryEngine is the top-level interface exposed to the TUI/SDK layer.

```go
type QueryEngine interface {
    Query(ctx context.Context, params QueryParams) (<-chan Msg, error)
    Interrupt(ctx context.Context)
    GetMessages() []types.Message
    SetMessages(messages []types.Message)
    SetModel(model string)
    MainTarget() MainTarget
    SetMainTarget(t MainTarget)
}
```

## Structs

- **BudgetTracker** — 4 fields: ContinuationCount, LastDeltaTokens, LastGlobalTurnTokens, StartedAt
- **CacheSafeParams** — 7 fields: SystemPrompt, ContextMessages, ToolUseContext, Model, Client, ContextWindow, MaxTokens
- **Config** — 9 fields: Client, Registry, Model, MaxTokens, PermissionChecker, StopHooks, MsgQueue, MainContextWindow, ...
- **ForkedAgentConfig** — 6 fields: PromptMessages, CacheSafeParams, QuerySource, MaxTurns, AllowedTools, OnMessage
- **MainTarget** — 5 fields: Client, ProviderID, Model, ContextWindow, MaxTokens
- **Msg** — 21 fields: Type, TextDelta, ToolUseID, ToolName, InputDelta, ToolInput, ToolResult, AssistantMsg, ...
- **PermissionRequestMsg** — 6 fields: RequestID, ToolUseID, ToolName, Message, Input, RespFn
- **QueryParams** — 18 fields: Messages, SystemPrompt, UserContext, SystemContext, ToolUseContext, QuerySource, MaxOutputTokensOverride, MaxTurns, ...
- **StopHookContext** — 6 fields: Messages, ToolUseContext, QuerySource, IsBareMode, Engine, CacheParams
- **StopHookRegistry** — 2 fields
- **SystemPrompt** — 1 fields: Parts
- **SystemPromptPart** — 2 fields: Text, CacheControl
- **TaskBudget** — 1 fields: Total
- **TokenBudgetDecision** — 8 fields: Action, NudgeMessage, ContinuationCount, Pct, TurnTokens, Budget, DiminishingReturns, DurationMs
- **ToolResultMsg** — 3 fields: ToolUseID, Content, IsError

## Function Types

- `StopHookFn` — `func(ctx context.Context, hookCtx *StopHookContext)`

## Functions

- `CreateCacheSafeParams(params QueryParams, messages []types.Message) *CacheSafeParams`
- `LoadCacheSafeParams() (*CacheSafeParams, error)`
- `LoadCacheSafeParamsFrom(filePath string) (*CacheSafeParams, error)`
- `New(cfg Config) QueryEngine`
- `NewBudgetTracker() *BudgetTracker`
- `NewStopHookRegistry() *StopHookRegistry`
- `RunForkedAgent(ctx context.Context, eng QueryEngine, cfg ForkedAgentConfig) ([]types.Message, error)`
- `SaveCacheSafeParams(params *CacheSafeParams) error`
- `SaveCacheSafeParamsTo(params *CacheSafeParams, filePath string) error`

## Constants

- `CompletionThreshold`
- `DefaultMaxToolUseConcurrency`
- `DiminishingThreshold`
- `MainProviderID`
- `MaxContinuations`
- `MsgTypeAssistantMessage`
- `MsgTypeCompactEnd`
- `MsgTypeCompactStart`
- `MsgTypeError`
- `MsgTypePermissionRequest`
- `MsgTypePermissionResponse`
- `MsgTypeProgress`
- `MsgTypeRequestStart`
- `MsgTypeStreamRequestStart`
- `MsgTypeStreamText`
- `MsgTypeSystemMessage`
- `MsgTypeThinkingDelta`
- `MsgTypeTombstone`
- `MsgTypeToolResult`
- `MsgTypeToolUseComplete`
- `MsgTypeToolUseInputDelta`
- `MsgTypeToolUseStart`
- `MsgTypeTurnComplete`
- `MsgTypeUserMessage`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `stubQueryEngine` in `internal/bootstrap/wire_fork_test.go`

**Exported type references (files that use types from this package):**
- `CacheSafeParams` → `internal/bootstrap/tiers_test.go` (test), `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test), `internal/memdir/dream.go`, `internal/tui/cmds.go`
- `Config` → `internal/bootstrap/wire.go`
- `ForkedAgentConfig` → `internal/memdir/dream.go`, `internal/memdir/extract.go`, `internal/memdir/session_memory.go`
- `MainTarget` → `internal/bootstrap/modelswitch.go`, `internal/bootstrap/modelswitch_test.go` (test), `internal/bootstrap/tiers_test.go` (test), `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test) + 1 more
- `Msg` → `internal/bootstrap/run.go`, `internal/bootstrap/tiers_test.go` (test), `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test), `internal/bootstrap/wire_result_test.go` (test) + 4 more
- `QueryEngine` → `internal/bootstrap/modelswitch.go`, `internal/bootstrap/wire.go`, `internal/memdir/dream.go`, `internal/tui/cmds.go`, `internal/tui/init.go` + 1 more
- `QueryParams` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test), `internal/tui/cmds.go`, `internal/tui/tui_test.go` (test)
- `StopHookContext` → `internal/bootstrap/wire.go`, `internal/memdir/dream.go`, `internal/memdir/extract.go`, `internal/memdir/extract_test.go` (test), `internal/memdir/session_memory.go`
- `StopHookRegistry` → `internal/bootstrap/wire.go`
- `SystemPrompt` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test), `internal/tui/cmds.go`
- `SystemPromptPart` → `internal/bootstrap/run.go`, `internal/bootstrap/wire.go`, `internal/bootstrap/wire_fork_test.go` (test), `internal/tui/cmds.go`
- `ToolResultMsg` → `internal/tui/tui_test.go` (test)

## Dependencies

**Imports:** `internal/api`, `internal/compact`, `internal/msgqueue`, `internal/permissions`, `internal/tools`, `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/memdir`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 内核搬回（反转 PR #31）**：本包原于 2026-09-23 逐字提取至外部仓 github.com/tunsuy/agtkeel；因该仓停止维护，经人类决策整併回本仓原始路径。来源：agtkeel main 3a77085＋后续提交，零逻辑改动（仅 import 路径恢复＋gofmt）。内核改动自此直接在本仓进行。
- **2026-11-18 cache 斷點佈局＋fork 快照＋inbox（feat/coordinator-fork-cache）**：buildRequestWithModel 確定性三處斷點——system（顯式 part 標記優先；無標記則掛末段，一個斷點快取整個穩定段）、tools 末位（會話內釘住）、messages 滾動斷點（最後 user message 末 block，clone 後標記不污染歷史）。SkipCacheWrite 抑制全部。runQueryLoop 於 turn 邊界（壓縮後、inbox drain 後）快照 CacheSafeParams（+Model）至 UseContext.CacheParams，供 Agent tool spawn fork 時繼承——快照與請求前綴一致，fork 前綴 byte-identical（守護測試 TestForkPrefix_ByteIdentity；訊息比對需排除 cache_control 標記，標記是 write-position metadata、provider 前綴匹配忽略其位置）。QueryParams.InboxCh 於每個 turn 邊界 non-blocking drain，以 <coordinator-message> 包裝追加尾部（前綴不動）。
- **2026-11-18 實機驗證修復：Interrupt slot 不被 background query 劫持（fix/agent-lifetime）**：Query 原無條件覆蓋 e.abortFn——sub-agent/fork 的 Query 跑在同一 engine 上會劫持 Interrupt（Ctrl+C 取消到 background query 而非主對話）。現仅 write-back（foreground）query 擁有 interrupt slot；NoWriteBack query 經自身 ctx 取消（coordinator cancelFn／fork ctx）。守護測試 TestInterrupt_NotHijackedByBackgroundQuery。
- **2026-11-19 streamResponse 接上 WithRetry（feat/api-rate-control）**：e.client.Stream 呼叫包 api.WithRetry（Model/QuerySource from req）——429/5xx/網路錯誤指數退避重试（retry-after 優先），401 fail-fast（無 refresh hook）。此前 WithRetry 僅測試使用、生產路徑 429 直接失敗（使用者 qwen 上限痛點）。守護測試 retry_wiring_test.go×2（429→重试→成功；401→單次失敗）；TestQuery_StreamError_EmitsErrorMsg 改用非重试 400（通用網路錯誤現會重试，TS parity）。
- **2026-11-19 thinking 顯示鏈守護（feat/openai-thinking-display）**：engine 既有 thinking_delta→MsgTypeThinkingDelta 路徑原僅 Anthropic 路徑觸及；OpenAI reasoning_content 現經 api 層轉譯為 thinking 事件後同鏈生效（零 engine 改動）。新增守護測試 thinking_display_test.go（thinking delta 事件＋assistant message thinking block 累積）。
- **2026-11-19 `QueryParams.Client` 與壓縮依查詢目標（provider 等級，做法 C）**：
  - `QueryParams.Client` 有值就用它發串流請求（`clientFor`），否則用 engine 預設 client；主腦與 fork 永遠不帶，前綴與 provider 不變（`TestQuery_MainLoopRequestUnchangedWithoutTiers` 以 golden 位元組把關）。`ContextWindow`、`CompactTracker` 同為每次查詢的設定；max_tokens 沿用既有 `MaxOutputTokensOverride`（C11）。
  - **修正前行為（spec §6）**：sub-agent 觸發自動壓縮時用主腦的 client 與啟動時的模型（費用算在主腦模型上），門檻也用 `e.model`；全 engine 共用一份壓縮狀態。現在壓縮管線（`compaction.go` `runCompaction`）依 `targetFor(params)` 計算門檻並在該 target 的 client/model 上摘要；sub-agent 壓縮在 `--debug` 時記一行 log。
  - C8：門檻估算＝最近一次 API 回報的 prompt tokens（input+cache read+cache creation）＋之後新增的訊息；沒有 usage 或歷史被改寫過時，退回 JSON÷4 估算並加上 system prompt 與工具 schema。**主腦也一起改**，所以主腦會比修正前早一點觸發壓縮——負責人認可的修正，是反目標「主腦行為不變」唯一的例外。
  - C5：`SetModel` 加 `modelMu`；摘要模型在呼叫當下解析，`/model` 後立即生效。C6：帶 `Client` 的查詢忽略 `FallbackModel`（它指的是主腦 provider 上的模型）。`[main] context_window` 經 `Config.MainContextWindow` 只套用在跑主腦模型、未帶 Client 的查詢。
- **2026-11-20 `MainTarget`（/model 切換主腦）**：主腦的 client、ProviderID、model、context window、max_tokens 合成 `MainTarget`，以 `mainMu` 保護；`SetMainTarget` 讓 /model 在會話中換到別的 provider。`runQueryLoop` 開始時取一次快照，整輪不換（切換只影響下一個查詢）。換 provider 時移除歷史中的 thinking 區塊（簽章只對原 provider 有效，切換本來就讓快取失效）；被中斷的查詢稍後寫回時若主腦已換 provider 會再清一次，鎖順序固定為 `e.mu → mainMu`。`CacheSafeParams` 記錄請求的 client、context window、max_tokens，讓 fork 與它的續跑停在原本的 provider。沒有切換時主腦請求位元組不變（golden 測試）。
