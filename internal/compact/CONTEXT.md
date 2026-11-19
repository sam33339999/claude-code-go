---
package: compact
import_path: internal/compact
layer: services
generated_at: 2026-11-19T06:52:46Z
source_files: [auto.go, compact.go, micro.go, snip.go]
---

# internal/compact

> Layer: **Services** · Files: 4 · Interfaces: 1 · Structs: 11 · Functions: 4

## Interfaces

### Compressor (2 methods)
> Compressor is the unified interface for context compaction.

```go
type Compressor interface {
    NeedsCompaction(messages []types.Message, model string, extra CompactionExtra) bool
    Compact(ctx context.Context, messages []types.Message, params CompactionParams) (*CompactionResult, error)
}
```

## Structs

- **AutoCompactTrackingState** — 4 fields: Compacted, TurnCounter, TurnID, ConsecutiveFailures
- **AutoCompactor** — 4 fields
- **CacheEdit** — 2 fields: ToolUseID, Summary
- **CompactionExtra** — 1 fields: SnipTokensFreed
- **CompactionParams** — 5 fields: SystemPromptParts, UserContext, SystemContext, QuerySource, ForkMessages
- **CompactionResult** — 7 fields: SummaryMessages, Attachments, HookResults, PreCompactTokenCount, PostCompactTokenCount, TruePostCompactTokenCount, CompactionUsage
- **MicroCompactResult** — 2 fields: Messages, CompactionInfo
- **MicroCompactionInfo** — 1 fields: PendingCacheEdits
- **MicroCompactor**
- **SnipResult** — 3 fields: Messages, TokensFreed, BoundaryMessage
- **TokenUsage** — 4 fields: InputTokens, OutputTokens, CacheReadInputTokens, CacheCreationInputTokens

## Functions

- `ContextWindow(model string) int`
- `NewAutoCompactor(client api.Client, model string, maxTokens int) *AutoCompactor`
- `NewMicroCompactor() *MicroCompactor`
- `SnipCompactIfNeeded(messages []types.Message) SnipResult`

## Constants

- `AutoCompactBufferTokens`
- `AutoCompactWarningBufferTokens`
- `MaxConsecutiveFailures`
- `MaxOutputTokensForSummary`

## Change Impact

**Exported type references (files that use types from this package):**
- `AutoCompactor` → `internal/engine/engine.go`
- `CompactionExtra` → `internal/engine/query.go`
- `CompactionParams` → `internal/engine/query.go`
- `MicroCompactor` → `internal/engine/engine.go`

## Dependencies

**Imports:** `internal/api`, `pkg/types`

**Imported by:** `internal/engine`, `internal/tui`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 内核搬回（反转 PR #31）**：本包原于 2026-09-23 逐字提取至外部仓 github.com/tunsuy/agtkeel；因该仓停止维护，经人类决策整併回本仓原始路径。来源：agtkeel main 3a77085＋后续提交，零逻辑改动（仅 import 路径恢复＋gofmt）。内核改动自此直接在本仓进行。
