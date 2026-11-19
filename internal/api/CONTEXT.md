---
package: api
import_path: internal/api
layer: services
generated_at: 2026-11-19T01:11:07Z
source_files: [accumulate.go, client.go, debug_logger.go, errors.go, factory.go, json.go, openai_client.go, openai_stream.go, openai_types.go, retry.go, stream.go, usage.go]
---

# internal/api

> Layer: **Services** · Files: 12 · Interfaces: 2 · Structs: 27 · Functions: 7

## Interfaces

### Client (2 methods)
> Client is the unified Anthropic API entry point.

```go
type Client interface {
    Stream(ctx context.Context, req *MessageRequest) (StreamReader, error)
    Complete(ctx context.Context, req *MessageRequest) (*MessageResponse, error)
}
```

### StreamReader (1 methods)
> StreamReader wraps SSE event stream reading, following io.Closer semantics.

```go
type StreamReader interface {
    Next() (*StreamEvent, error)
}
```

## Structs

- **APIError** — 4 fields: StatusCode, Message, Headers, Kind
- **APIErrorData** — 2 fields: Type, Message
- **Accumulator** — 3 fields
- **CacheControl** — 1 fields: Type
- **CacheCreationUsage** — 2 fields: Ephemeral1hInputTokens, Ephemeral5mInputTokens
- **CannotRetryError** — 2 fields: Cause, RetryContext
- **ClientConfig** — 13 fields: Provider, APIKey, BaseURL, MaxRetries, TimeoutSeconds, CustomHeaders, AWSRegion, GCPProject, ...
- **ContentBlock** — 11 fields: Type, Text, ID, Name, Input, ToolUseID, Content, IsError, ...
- **ContentBlockDeltaData** — 2 fields: Index, Delta
- **DebugLogger** — 4 fields
- **Delta** — 4 fields: Type, Text, PartialJSON, Thinking
- **ExtraUsage** — 4 fields: IsEnabled, MonthlyLimit, UsedCredits, Utilization
- **FallbackTriggeredError** — 2 fields: OriginalModel, FallbackModel
- **MessageDeltaData** — 2 fields: Delta, Usage
- **MessageParam** — 2 fields: Role, Content
- **MessageRequest** — 9 fields: Model, MaxTokens, Messages, System, Tools, Stream, ThinkingBudget, SystemBlocks, ...
- **MessageResponse** — 7 fields: ID, Type, Role, Content, Model, StopReason, Usage
- **MessageStartData** — 1 fields: Message
- **RateLimit** — 2 fields: Utilization, ResetsAt
- **RetryContext** — 3 fields: Attempt, Model, QuerySource
- **RetryOptions** — 6 fields: MaxRetries, Model, FallbackModel, QuerySource, InitialConsecutive529, Signal
- **ServerToolUse** — 2 fields: WebSearchRequests, WebFetchRequests
- **StreamEvent** — 6 fields: Type, Data, MessageStart, ContentBlockDelta, MessageDelta, Error
- **SystemBlock** — 3 fields: Type, Text, CacheControl
- **ToolSchema** — 4 fields: Name, Description, InputSchema, CacheControl
- **Usage** — 9 fields: InputTokens, CacheCreationInputTokens, CacheReadInputTokens, OutputTokens, ServerToolUse, ServiceTier, CacheCreation, InferenceGeo, ...
- **Utilization** — 6 fields: FiveHour, SevenDay, SevenDayOAuthApps, SevenDayOpus, SevenDaySonnet, ExtraUsage

## Function Types

- `RetryableFunc` — `func(ctx context.Context, attempt int) (T, error)`

## Functions

- `Is529Error(err error) bool`
- `IsOAuthTokenRevokedError(err error) bool`
- `NewClient(cfg ClientConfig, httpClient *http.Client) (Client, error)`
- `NewDebugLogger(debug bool, debugFile string) (*DebugLogger, error)`
- `ParseContextOverflowError(err error) (inputTokens int, contextLimit int, ok bool)`
- `RetryDelay(attempt int, retryAfterHeader string, maxDelayMS int) time.Duration`
- `WithRetry(ctx context.Context, fn any, opts RetryOptions) (T, error)`

## Constants

- `BaseDelayMS`
- `DefaultMaxRetries`
- `ErrKindConnectionError`
- `ErrKindConnectionTimeout`
- `ErrKindContextWindow`
- `ErrKindForbidden`
- `ErrKindInvalidRequest`
- `ErrKindOverloaded`
- `ErrKindRateLimit`
- `ErrKindServerError`
- `ErrKindUnauthorized`
- `ErrKindUnknown`
- `EventContentBlockDelta`
- `EventContentBlockStart`
- `EventContentBlockStop`
- `EventError`
- `EventMessageDelta`
- `EventMessageStart`
- `EventMessageStop`
- `EventPing`
- `HeartbeatInterval`
- `Max529Retries`
- `MaxDelayMS`
- `PersistentMaxBackoff`
- `PersistentResetCap`
- `ProviderBedrock`
- `ProviderDirect`
- `ProviderFoundry`
- `ProviderOpenAI`
- `ProviderVertex`

## Change Impact

**Test Mocks (must add new methods when interfaces change):**
- `mockClient` in `internal/engine/engine_test.go`
- `mockClient` in `test/integration/engine_e2e_test.go`

**Exported type references (files that use types from this package):**
- `APIError` → `internal/engine/engine_test.go` (test), `internal/engine/query.go`
- `Accumulator` → `internal/engine/query.go`
- `Client` → `internal/bootstrap/wire.go`, `internal/compact/auto.go`, `internal/engine/engine.go`, `internal/engine/engine_test.go` (test)
- `ClientConfig` → `internal/bootstrap/wire.go`
- `ContentBlock` → `internal/engine/engine_test.go` (test), `internal/engine/query.go`
- `ContentBlockDeltaData` → `internal/engine/engine_test.go` (test)
- `Delta` → `internal/engine/engine_test.go` (test)
- `MessageDeltaData` → `internal/engine/engine_test.go` (test)
- `MessageParam` → `internal/compact/auto.go`, `internal/engine/query.go`
- `MessageRequest` → `internal/compact/auto.go`, `internal/engine/cache_breakpoints_test.go` (test), `internal/engine/engine_test.go` (test), `internal/engine/forked_agent_test.go` (test), `internal/engine/interrupt_slot_test.go` (test) + 2 more
- `MessageResponse` → `internal/compact/auto.go`, `internal/engine/engine_test.go` (test), `internal/engine/query.go`
- `MessageStartData` → `internal/engine/engine_test.go` (test)
- `StreamEvent` → `internal/engine/engine_test.go` (test)
- `StreamReader` → `internal/engine/cache_breakpoints_test.go` (test), `internal/engine/engine_test.go` (test), `internal/engine/forked_agent_test.go` (test), `internal/engine/interrupt_slot_test.go` (test), `internal/engine/stop_hooks_test.go` (test)
- `SystemBlock` → `internal/engine/query.go`
- `ToolSchema` → `internal/engine/query.go`
- `Usage` → `internal/engine/engine_test.go` (test), `internal/engine/query.go`

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/compact`, `internal/engine`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 内核搬回（反转 PR #31）**：本包原于 2026-09-23 逐字提取至外部仓 github.com/tunsuy/agtkeel；因该仓停止维护，经人类决策整併回本仓原始路径。来源：agtkeel main 3a77085＋后续提交，零逻辑改动（仅 import 路径恢复＋gofmt）。内核改动自此直接在本仓进行。
- **OpenAI 兼容增强（随搬回正式入库）**：normalizeBaseURL 去除 baseURL 尾部 /v1 避免重复版本路径（原 agtkeel 提交 9262ebf）；openai_types.go 新增 ReasoningContent 字段支持 DeepSeek-R1/QwQ 类推理模型（原 .agtkeel-uncommitted.diff WIP，现已落定）。
- **2026-11-18 prompt-cache wire 支援（feat/coordinator-fork-cache）**：此前 Anthropic 路徑**完全未送 cache_control**（TUI 設的 SystemPromptPart.CacheControl 被 engine 壓成純字串忽略）——prompt cache 從未寫入。現 MessageRequest.SystemBlocks 非空時 MarshalJSON 以 block 陣列序列化 system（帶斷點）；ToolSchema/ContentBlock +CacheControl；EphemeralCache 共享標記值。System 仍為 canonical 純文字（既有測試與 compact 路徑不受影響）；OpenAI 相容路徑經 SystemText() 讀純文字（provider 隱式前綴快取，不帶標記）。滾動斷點由 engine 佈局，api 層只負責序列化。
- **2026-11-19 OpenAI 相容路徑 cache usage 映射（feat/openai-cache-usage）**：此前 OpenAI 路徑 CacheReadInputTokens 恆 0——openaiUsage 無 cached_tokens 欄位、StreamOptions.IncludeUsage 從未設定、usage chunk（常於 finish chunk 之後、choices 空）不被發出事件、accumulator 只合併 OutputTokens。現：openaiUsage +PromptTokensDetails（OpenAI 式）＋PromptCacheHitTokens（DeepSeek 式）＋cachedTokens()；streaming 請求設 stream_options.include_usage=true；usage chunk 觸發补充 message_delta（createUsageDeltaEvent，.repeat lastStopReason）携带 final input/output/cache counts；accumulator 非零 input/cache 欄位覆寫 message_start 快照、空 stop reason 視為不變（Anthropic delta 語義保留）。Complete 路徑同步映射。守護測試 openai_cache_usage_test.go×4。注意：OpenAI prompt_tokens 含 cached（與 Anthropic input_tokens 不含 cache read 的語義不同），CacheReadInputTokens 独立報告命中數。
