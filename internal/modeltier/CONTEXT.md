---
package: modeltier
import_path: internal/modeltier
layer: unknown
generated_at: 2026-11-19T13:49:59Z
source_files: [config.go, resolve.go, store.go]
---

# internal/modeltier

> Layer: **unknown** · Files: 3 · Interfaces: 0 · Structs: 8 · Functions: 3

## Structs

- **Binding** — 5 fields: Tier, Provider, Model, ContextWindow, MaxTokens
- **Config** — 3 fields: Providers, Tiers, Main
- **MainSection** — 1 fields: ContextWindow
- **ModelInfo** — 2 fields: ContextWindow, MaxTokens
- **Provider** — 4 fields: Format, BaseURL, APIKeyEnv, Models
- **ProviderStatus** — 5 fields: ID, Format, BaseURL, APIKeyEnv, KeyPresent
- **Store** — 4 fields
- **TierEntry** — 4 fields: Provider, Model, ContextWindow, MaxTokens

## Functions

- `DefaultPath(homeDir string) string`
- `LegacyAlias(tier string) (string, bool)`
- `Load(path string) (*Store, []string)`

## Constants

- `Bound`
- `FormatAnthropic`
- `FormatOpenAI`
- `MainProviderID`
- `NotTier`
- `Unbound`

## Change Impact

**Exported type references (files that use types from this package):**
- `Binding` → `internal/bootstrap/tiers.go`, `internal/commands/tier.go`, `internal/commands/tier_test.go` (test)
- `Provider` → `internal/bootstrap/tiers.go`, `internal/bootstrap/tiers_test.go` (test), `internal/commands/tier.go`, `internal/commands/tier_test.go` (test)
- `ProviderStatus` → `internal/commands/tier.go`, `internal/commands/tier_test.go` (test)
- `Store` → `internal/bootstrap/tiers.go`, `internal/bootstrap/wire.go`, `internal/commands/tier.go`, `internal/commands/tier_test.go` (test)

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`, `internal/commands`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-19 新包（provider 等級）**：讀寫 `~/.glaude/config.toml`（provider 註冊、`[tiers.*]`、`[main]`），把等級名、舊別名（haiku/sonnet/opus→lite/pro/max）或 `providerID:modelID` 解析成 `Binding`。刻意**不** import `internal/api`：只描述請求該去哪，client 由 bootstrap 建。`BurntSushi/toml` 只在本包使用（`internal/config` 維持只用標準庫）。
- `Load` 永不回 error：檔案不存在＝空設定；格式錯或欄位不合法＝警告＋忽略該部分，啟動不中斷。警告不回顯欄位值（可能是金鑰）。`main` 是保留 provider id，`[providers.main]` 被忽略。明文 `api_key` 會讓整個 provider 被忽略。
- `Bind` 寫入暫存檔→Sync→rename，成功後才換掉記憶體設定；重新編碼會遺失註解（已知取捨，`/tier` 會提示）。Bind 清掉該等級的 `context_window`/`max_tokens`，因為那屬於舊模型。
- lite/pro/max 永遠是已知等級（即使檔案沒寫），未綁定時 `Resolve` 回 `Unbound` 並帶正規化等級名，讓 bootstrap 能退回舊別名規則。
- 段落審查後補修：`Bind`/`Unbind` 改為在鎖內**重新讀檔**、以 `map[string]any` 只替換 `tiers.<name>` 再原子寫回，被驗證忽略的段落、未知鍵和啟動後的手動修改都會保留（只有註解遺失）。啟動時檔案讀取或解析失敗，就拒絕寫檔，避免把只是打錯一個字的檔案整個覆蓋。
