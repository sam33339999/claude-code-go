# Provider 註冊表 + 模型等級 + 壓縮全修 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **本專案的執行方式**（負責人的全域 CLAUDE.md）：主 session 依序實作（superpowers:executing-plans）；Task 7 結束（後端完成）時開一個 reviewer subagent 審那一段的 diff；Task 9 結束、合併前再審整個分支。

**Goal:** 讓 sub-agent 依等級（lite / pro / max）把請求送到 `~/.glaude/config.toml` 註冊的 provider，並修好 C1～C12 的壓縮問題。

**Architecture:** 新包 `internal/modeltier` 負責讀寫 `config.toml`、解析等級。bootstrap 為每個 provider 建 client，派工時把 `Client`、`Model`、`ContextWindow`、`MaxOutputTokensOverride`、`CompactTracker` 放進 `engine.QueryParams`（做法 C）。compact 改成無狀態的策略，搭配每個 agent 各一份的 `Tracker`，門檻、摘要和 snip 都依這次查詢的 `Target` 計算。

**Tech Stack:** Go 1.26、`github.com/BurntSushi/toml`（新依賴，只在 `internal/modeltier` 使用）、bubbletea v2（`charm.land/*`）。

**Spec:** `docs/superpowers/specs/2026-11-19-provider-tiers-design.md`（§0 為準）

## Global Constraints

- 依賴方向：TUI/commands 禁止 import bootstrap；`internal/tools/*` 子包之間禁止互相 import；`pkg/types` 零第三方依賴；`internal/modeltier` **不** import `internal/api`。
- `github.com/BurntSushi/toml` 只能在 `internal/modeltier` 使用，不要放進 `internal/config`。
- 金鑰只透過 `api_key_env`，不接受明文 `api_key` 欄位。
- `format` 只接受 `"anthropic"`（→ `api.ProviderDirect`）和 `"openai"`（→ `api.ProviderOpenAI`）。
- `main` 是保留 provider id；`[providers.main]` 出現時啟動印警告並忽略。
- `context_window` 解析順序：`CLAUDE_CODE_AUTO_COMPACT_WINDOW` > 等級 > `[providers.<id>.models."<model>"]` > 內建表 > 預設值 200000；主腦：env > `[main] context_window` > 內建表 > 預設值。
- `max_tokens`：等級 > provider 模型表 > engine 預設；摘要輸出上限 `min(MaxOutputTokensForSummary=20000, max_tokens)`。
- fork agent 永遠不帶 `Client`；主腦不帶 `Client`。
- 檔案 ≤800 行、函式 ≤80 行；import 三組排序；導出符號要有 GoDoc；`fmt.Errorf("...: %w", err)`；I/O 函式第一個參數是 `context.Context`（純檔案讀寫的 `Load`/`Bind` 例外，比照 `internal/config` 現況）。
- 欠帳棘輪不升（`TODO(dep)` 11、`not yet implemented` 0）。
- 改到導出符號的包：先 `make docs`，再補 `CONTEXT.md` 的 `## Design Notes`。
- 跑 Go 指令不需要 fnm；`make test` 需要 CGO。

## Review Focus

1. **`api_key_env` 指的環境變數不存在**：派工到該等級時要回報清楚、點名變數的錯誤，不能默默退回主腦。→ Task 7 `TestResolveSubAgentTarget_MissingKeyErrors`。
2. **模型 ID 本身含 `:`**（例如 `openrouter:qwen/qwen3:free`，或沒有註冊前綴的 `qwen3:free`）：只用第一個 `:` 切開，前綴不是已註冊 provider 就當成模型 ID 原樣傳下去。→ Task 1 `TestResolve_ColonInModelID`。
3. **`/tier` 寫檔的同時有 sub-agent 正在派工讀等級表**：不能有 data race，也不能讀到半套的狀態。→ Task 2 `TestStore_ConcurrentBindResolve`（`-race`）。
4. **沒有 `config.toml` 時，主腦的請求位元組跟現在完全一樣**（反目標）。→ Task 5 `TestQuery_MainLoopRequestUnchangedWithoutTiers`。
5. **`config.toml` 格式錯誤，或 `[tiers.x]` 指向未註冊的 provider**：印警告、該等級視為未綁定，程式照常啟動。→ Task 1 `TestLoad_MalformedFile`、`TestLoad_TierWithUnknownProvider`。

---

## File Structure

| 檔案 | 動作 | 職責 |
|------|------|------|
| `internal/modeltier/config.go` | 新增 | TOML 型別、`Load`、驗證與警告 |
| `internal/modeltier/resolve.go` | 新增 | `Resolve`、`LegacyAlias`、值的優先順序 |
| `internal/modeltier/store.go` | 新增 | `Store`（RWMutex）、`Bind`/`Unbind`/`List`/`Providers`、原子寫檔 |
| `internal/modeltier/*_test.go` | 新增 | 單元與並發測試 |
| `internal/compact/window.go` | 新增 | `ResolveContextWindow`、內建表（自 auto.go 移出） |
| `internal/compact/target.go` | 新增 | `Target`、`Tracker` |
| `internal/compact/auto.go` | 修改 | 無狀態化、`ShouldCompact`、`Compact(…, Target, *Tracker)`、`fitForSummary` |
| `internal/compact/snip.go` | 修改 | 依 token 門檻一次剪一批，移除 `BoundaryMessage` |
| `internal/compact/snip_prefix_test.go` | 修改 | C9/C12 測試（已存在，現在是失敗狀態） |
| `internal/engine/engine.go` | 修改 | `QueryParams` 新欄位、`Config` 新欄位、`SetModel` 加鎖 |
| `internal/engine/compaction.go` | 新增 | 從 `query.go` 抽出壓縮管線：`targetFor`、`clientFor`、`estimatePrompt`、`runCompaction` |
| `internal/engine/query.go` | 修改 | 呼叫 `runCompaction`、`clientFor`；C6 守護 |
| `internal/agenttype/agenttype.go` | 修改 | 導出 `ResolveAliasForProvider`；`Model` 的 GoDoc |
| `internal/agenttype/builtins.go`（explore 定義所在檔） | 修改 | explore `Model` 改成 `"lite"` |
| `internal/tools/agent/agent.go:111` | 修改 | `model` 參數的 description |
| `internal/bootstrap/tiers.go` | 新增 | `tierRouter`、`loadTierRouter`、`resolveSubAgentTarget` |
| `internal/bootstrap/wire.go` | 修改 | 兩條建構路徑接線、`buildSubAgentQuery` 帶 router、`AppContainer.Tiers` |
| `internal/commands/tier.go` | 新增 | `TierManager` 介面、`/tier` 指令 |
| `internal/commands/registry.go` | 修改 | `CommandContext.Tiers` |
| `internal/tui/{init,model,update}.go`、`internal/bootstrap/root.go` | 修改 | 注入 `TierManager`、狀態列視窗用 `[main]` |
| `docs/config-toml.md` | 新增 | `config.toml` 使用說明 |

---

### Task 0: 分支與基線

**Files:** 無程式碼變更

- [ ] **Step 1: 確認分支**：`git branch --show-current` → `feat/provider-tiers`。
- [ ] **Step 2: 記錄基線**：`make vet && go test -race ./internal/compact/ ./internal/engine/ ./internal/bootstrap/ ./internal/agenttype/ ./internal/commands/ ./internal/tui/`。除了 `TestSnip_PrefixStableAcrossTurns`、`TestSnip_BoundaryNotRepeated`（C9/C12 的證據，預期失敗）以外，全部 PASS。
- [ ] **Step 3: Commit 設計文件與計畫**

```bash
git add docs/superpowers/specs/2026-11-19-provider-tiers-design.md docs/superpowers/plans/2026-11-19-provider-tiers.md
git commit -m "docs(spec): provider 等級與壓縮全修設計與實作計畫"
```

（`AGENTS.md`、`.claude/CLAUDE.md`、`GLAUDE.md` 的未提交修改不在這次範圍，依負責人指示另行處理。）

---

### Task 1: `modeltier` 設定載入與解析

**Files:**
- Create: `internal/modeltier/config.go`, `internal/modeltier/resolve.go`
- Test: `internal/modeltier/config_test.go`, `internal/modeltier/resolve_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces:
  ```go
  const MainProviderID = "main"
  type ModelInfo struct { ContextWindow int `toml:"context_window,omitempty"`; MaxTokens int `toml:"max_tokens,omitempty"` }
  type Provider struct { Format string `toml:"format"`; BaseURL string `toml:"base_url"`; APIKeyEnv string `toml:"api_key_env"`; Models map[string]ModelInfo `toml:"models,omitempty"` }
  type TierEntry struct { Provider string `toml:"provider,omitempty"`; Model string `toml:"model,omitempty"`; ContextWindow int `toml:"context_window,omitempty"`; MaxTokens int `toml:"max_tokens,omitempty"` }
  type MainSection struct { ContextWindow int `toml:"context_window,omitempty"` }
  type Config struct { Providers map[string]Provider `toml:"providers"`; Tiers map[string]TierEntry `toml:"tiers"`; Main MainSection `toml:"main"` }
  type Binding struct { Tier, Provider, Model string; ContextWindow, MaxTokens int } // 0 = 未指定
  type ResolveKind int // NotTier | Unbound | Bound
  func DefaultPath(homeDir string) string                       // <home>/.glaude/config.toml
  func Load(path string) (*Store, []string)                     // 不回 error：失敗一律變成 warnings + 空設定
  func (s *Store) Resolve(name string) (Binding, ResolveKind)
  func LegacyAlias(tier string) (string, bool)                  // lite→haiku, pro→sonnet, max→opus
  func (s *Store) MainContextWindow() int
  ```
  （`Store` 的結構在本 task 先定義成 `struct{ mu sync.RWMutex; path string; cfg Config }`，寫入方法在 Task 2。）

- [ ] **Step 1: 加入依賴**：`go get github.com/BurntSushi/toml@latest`，預期 go.mod 多一行 require。

- [ ] **Step 2: 寫失敗測試**（`config_test.go`）

```go
func TestLoad_MissingFile(t *testing.T)           // Load(不存在路徑) → warnings 為空；Resolve("lite") kind==Unbound（lite/pro/max 永遠是已知等級）；Resolve("qwen-turbo")==NotTier；MainContextWindow()==0
func TestLoad_MalformedFile(t *testing.T)         // 內容 "[[[" → len(warnings)==1 且含檔名；Resolve("lite") kind==Unbound
func TestLoad_RejectsBadFormat(t *testing.T)      // providers.x.format="bedrock" → 警告含 "x"；provider x 不存在
func TestLoad_ReservedMainIgnored(t *testing.T)   // [providers.main] → 警告含 "main"；Resolve("main:foo") 仍回 Bound{Provider:"main",Model:"foo"}
func TestLoad_TierWithUnknownProvider(t *testing.T) // [tiers.lite] provider="nope" → 警告；Resolve("lite") kind==Unbound
func TestLoad_PlaintextAPIKeyRejected(t *testing.T) // providers.x.api_key="sk-..." → 警告含 "api_key_env"；provider x 不存在（未知欄位用 toml.MetaData.Undecoded() 偵測）
func TestLoad_MainSection(t *testing.T)           // [main] context_window=131072 → MainContextWindow()==131072
```

- [ ] **Step 3: 寫失敗測試**（`resolve_test.go`），用 spec §5.1 的範例 TOML 當 fixture：

```go
func TestResolve_TierBound(t *testing.T)        // "lite" → Bound{Tier:"lite",Provider:"zai",Model:"glm-5.3-flash",MaxTokens:8192}
func TestResolve_LegacyAlias(t *testing.T)      // "haiku" 與 "lite" 結果相同；"HAIKU" 也相同（不分大小寫）
func TestResolve_ValuePrecedence(t *testing.T)  // pro 綁 openrouter:"qwen/qwen3.8-27b"，模型表 262144/16384 → ContextWindow 262144、MaxTokens 16384；等級上另寫 context_window=100000 → 100000
func TestResolve_MainTierUsesOwnValues(t *testing.T) // max → Provider "main", ContextWindow 200000（不套 [main]）
func TestResolve_UnboundKnownTier(t *testing.T) // [tiers.foo] 空表 → kind Unbound；"max" 未出現 → kind Unbound（lite/pro/max 永遠是已知等級）
func TestResolve_ColonInModelID(t *testing.T)   // "openrouter:qwen/qwen3:free" → Bound{Provider:"openrouter",Model:"qwen/qwen3:free"}；"qwen3:free" → NotTier
func TestResolve_EmptyAndInherit(t *testing.T)  // "" 與 "inherit" → NotTier
func TestLegacyAlias(t *testing.T)              // lite→haiku, pro→sonnet, max→opus, foo→("",false)
```

- [ ] **Step 4: 執行，確認失敗**：`go test ./internal/modeltier/` → 編譯失敗（型別未定義）。

- [ ] **Step 5: 實作 `config.go` 與 `resolve.go`**
  - `Load`：`os.ReadFile`；`os.IsNotExist` → 空 Store、無警告。`toml.Decode` 失敗 → 一則警告 `"config.toml: <path>: <err>; ignoring file"`、空設定。通過後依序驗證並刪掉不合格的項目，每一項給一則警告：`providers.main`、format 不合法、有 `api_key` 欄位（Undecoded key）、tier 指向未註冊的 provider（保留 key、清空 Provider/Model，所以會是 Unbound）、`context_window`/`max_tokens` ≤0。
  - `Resolve` 演算法：
    ```
    n := strings.ToLower(strings.TrimSpace(name)); 空或 "inherit" → NotTier
    if legacy→tier 映射命中: n = tier
    if entry, ok := cfg.Tiers[n] 或 n ∈ {lite,pro,max}:
        entry 沒有 Provider/Model → Unbound
        → Bound（值依 等級 > providers[p].Models[model] > 0）
    if i := strings.Index(name, ":"); i > 0 且 (name[:i] 已註冊 或 == MainProviderID):
        → Bound{Tier:"", Provider: name[:i], Model: name[i+1:]}（值依 provider 模型表）
    → NotTier
    ```
    `provider:model` 的模型 ID 保留原始大小寫（只有等級名小寫化）。

- [ ] **Step 6: 執行，確認通過**：`go test -race ./internal/modeltier/` → PASS。

- [ ] **Step 7: Commit**：`git add go.mod go.sum internal/modeltier && git commit -m "feat(modeltier): config.toml 載入與等級解析"`

---

### Task 2: `modeltier.Store` 改綁定與寫檔

**Files:**
- Create: `internal/modeltier/store.go`
- Test: `internal/modeltier/store_test.go`

**Interfaces:**
- Consumes: Task 1 的 `Store`、`Config`、`Binding`。
- Produces:
  ```go
  type ProviderStatus struct { ID, Format, BaseURL, APIKeyEnv string; KeyPresent bool }
  func (s *Store) List() []Binding                 // 依等級名排序；lite/pro/max 一定出現（未綁定時 Provider==""）
  func (s *Store) Providers() []ProviderStatus     // 依 ID 排序；第一筆固定是 {ID:"main", Format:"(主腦)", KeyPresent:true}；KeyPresent 在呼叫當下 os.Getenv 判斷
  func (s *Store) Provider(id string) (Provider, bool)
  func (s *Store) Bind(tier, providerID, modelID string) error
  func (s *Store) Unbind(tier string) error
  ```

- [ ] **Step 1: 寫失敗測試**

```go
func TestStore_BindWritesAtomically(t *testing.T) // Bind("lite","openrouter","x/y") → 重新 Load 同一路徑，Resolve("lite") == Bound{Provider:"openrouter",Model:"x/y"}；目錄裡沒有殘留的暫存檔
func TestStore_BindCreatesFile(t *testing.T)      // 路徑的目錄存在但檔案不存在 → Bind("lite","main","claude-haiku-5-5") 成功並建檔
func TestStore_BindUnknownProvider(t *testing.T)  // Bind("lite","nope","m") → error 含 "nope"；檔案內容不變
func TestStore_BindClearsTierOverrides(t *testing.T) // lite 原本有 max_tokens=8192 → Bind 到新模型後 Resolve("lite").MaxTokens 改由 provider 模型表決定（無則 0）
func TestStore_Unbind(t *testing.T)               // Unbind("foo") 後重新 Load → Resolve("foo") kind==Unbound（保留空的 [tiers.foo]）
func TestStore_BindRejectsBadInput(t *testing.T)  // tier "" 或含 ":"、model "" → error
func TestStore_ConcurrentBindResolve(t *testing.T) // 20 個 goroutine 交錯 Bind 和 Resolve 共 200 次，-race 無報告；最後 Resolve 回傳某一次 Bind 的完整值（Provider 和 Model 成對）
func TestStore_ProvidersKeyPresence(t *testing.T) // t.Setenv("ZAI_API_KEY","k") → zai KeyPresent==true；openrouter 未設 → false
```

- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/modeltier/ -run TestStore` → 編譯失敗。
- [ ] **Step 3: 實作**：寫入時持有 `mu.Lock()`，複製一份 `cfg` 套用變更，`toml.NewEncoder` 編碼到 `os.CreateTemp(filepath.Dir(path), ".config.toml.*")` → `Sync` → `Close` → `os.Rename`，**成功後**才換掉記憶體裡的 `cfg`。Bind 會清掉該等級的 `ContextWindow`/`MaxTokens`（我的決定：舊值屬於舊模型，留著可能讓新模型超限回 400）。檔案權限 `0o644`，目錄不存在時先 `os.MkdirAll(dir, 0o755)`。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/modeltier/` → PASS。
- [ ] **Step 5: Commit**：`git add internal/modeltier && git commit -m "feat(modeltier): Store 改綁定與原子寫檔"`

---

### Task 3: compact — 視窗解析、Target、Tracker、自動壓縮（C1、C3、C4、C5、C7、C8 的 compact 部分）

**Files:**
- Create: `internal/compact/window.go`, `internal/compact/target.go`
- Modify: `internal/compact/auto.go`, `internal/compact/compact.go`（套件說明）
- Test: `internal/compact/window_test.go`, `internal/compact/auto_test.go`（新）；`compact_test.go:253-330` 改用新 API

**Interfaces:**
- Produces:
  ```go
  // window.go
  func ResolveContextWindow(model string, configured int) int // env > configured(>0) > 內建表 > 200000
  func ContextWindow(model string) int                        // = ResolveContextWindow(model, 0)，TUI 既有呼叫點保留
  // target.go
  type Target struct { Client api.Client; Model string; ContextWindow int; MaxOutputTokens int }
  func (t Target) Window() int                                // ResolveContextWindow(t.Model, t.ContextWindow)
  type Tracker struct { /* mu sync.Mutex; state AutoCompactTrackingState */ }
  func NewTracker() *Tracker
  func (t *Tracker) Snapshot() AutoCompactTrackingState
  // auto.go
  func NewAutoCompactor() *AutoCompactor                      // 無欄位；保留型別讓 engine 持有
  func AutoCompactThreshold(t Target) int                     // Window − min(MaxOutputTokens, 20000) − 13000
  func (a *AutoCompactor) ShouldCompact(estTokens int, t Target, tr *Tracker) bool
  func (a *AutoCompactor) Compact(ctx context.Context, messages []types.Message, params CompactionParams, t Target, tr *Tracker) (*CompactionResult, error)
  func EstimateTokens(messages []types.Message) int           // 原 estimateTokens 導出
  ```
  `CompactionExtra.SnipTokensFreed` 保留欄位但不再相減（訊息已經是 snip 後的，再扣一次會重複扣除），GoDoc 寫明這一點。

- [ ] **Step 1: 寫失敗測試**（`window_test.go`）

```go
func TestResolveContextWindow_Precedence(t *testing.T)
// t.Setenv("CLAUDE_CODE_AUTO_COMPACT_WINDOW","50000") → ResolveContextWindow("x", 131072)==50000
// env 未設：("qwen", 131072)==131072；("claude-sonnet-5-5",0)==200000；("claude-opus-5-5",0)==200000；
// ("claude-haiku-4-5",0)==200000；("claude-2.1",0)==100000；("glm-5.3-flash",0)==200000（預設）
```

- [ ] **Step 2: 寫失敗測試**（`auto_test.go`），用一個記錄請求的假 client（`Complete` 存下 `req` 並回傳 text "SUMMARY"）：

```go
func TestShouldCompact_UsesTargetWindow(t *testing.T)   // Target{Model:"qwen",ContextWindow:32000,MaxOutputTokens:8192} 門檻 = 32000−8192−13000 = 10808；est 10808 → true；10807 → false
func TestShouldCompact_PerTrackerBreaker(t *testing.T)  // trA 連續 3 次 Compact 失敗後 ShouldCompact(…,trA) == false；同時 trB 仍為 true
func TestCompact_UsesTargetClientAndModel(t *testing.T) // Target{Client:fakeTier, Model:"glm"} → fakeTier 收到 1 個請求、Model=="glm"；另一個 fakeMain 收到 0 個
func TestCompact_SummaryMaxTokensCapped(t *testing.T)   // MaxOutputTokens 8192 → req.MaxTokens==8192；MaxOutputTokens 0 或 64000 → 20000
func TestCompact_FitsSummaryInput(t *testing.T)         // Target{ContextWindow:60000, MaxOutputTokens:8192}、訊息估算約 80000 → 送出的訊息 EstimateTokens ≤ 60000−8192−13000；第一則是不含 tool_result 的 user 訊息；最後一則與原訊息最後一則相同
func TestCompact_SuccessResetsTracker(t *testing.T)     // 失敗 2 次再成功 → Snapshot().ConsecutiveFailures==0、Compacted==true
func TestTracker_ConcurrentUse(t *testing.T)            // 10 個 goroutine 各自對同一個 Tracker 呼叫 Compact（假 client 隨機失敗）+ ShouldCompact，-race 無報告
```

- [ ] **Step 3: 執行，確認失敗**：`go test ./internal/compact/` → 編譯失敗。
- [ ] **Step 4: 實作**
  - 內建表：`claude-2*` → 100000；其他 `claude-*` → 200000；其餘回預設值。
  - `Compact` 的摘要請求：`Model: t.Model`、`MaxTokens: summaryMaxOut(t)`、`client: t.Client`（nil → 回 error `"compact: no client"`，並計一次失敗）。
  - `fitForSummary(messages []types.Message, budgetTokens int) []types.Message`：`EstimateTokens(messages) ≤ budget` 就原樣回傳；否則從頭往後丟訊息，直到估算值 ≤ budget，**而且**第一則是不含 `tool_result` 區塊的 user 訊息；如果全部丟完都找不到這樣的切點，就只保留最後一則 user 訊息。budget = `t.Window() − summaryMaxOut(t) − AutoCompactBufferTokens − EstimateTokens(system prompt)`，下限 1。
  - Tracker 所有讀寫都在 `mu` 內進行；`Compact` 只在開頭讀、結尾寫，**呼叫 LLM 時不持有鎖**。
- [ ] **Step 5: 修改 `compact_test.go` 舊的 AutoCompactor 測試**，改用新 API，斷言意義不變（低於門檻 false、熔斷後 false）。
- [ ] **Step 6: 執行，確認通過**：`go test -race ./internal/compact/ -run 'Resolve|Should|Compact_|Tracker|AutoCompactor|Estimate'` → PASS（snip 的兩個測試仍失敗，Task 4 處理）。
- [ ] **Step 7: Commit**：`git commit -m "fix(compact): 依查詢 Target 計算門檻與摘要，壓縮狀態改為每 agent 一份"`

---

### Task 4: compact — snip 依門檻一次剪一批（C9、C12）

**Files:**
- Modify: `internal/compact/snip.go`, `internal/compact/snip_prefix_test.go`, `internal/compact/compact_test.go:60-160`

**Interfaces:**
- Consumes: Task 3 的 `Target`、`AutoCompactThreshold`。
- Produces:
  ```go
  const SnipTriggerFraction = 0.8 // 我的決定：估算值到達自動壓縮門檻的 80% 才 snip
  type SnipResult struct { Messages []types.Message; TokensFreed int; Snipped bool } // 移除 BoundaryMessage
  func SnipCompactIfNeeded(messages []types.Message, estTokens int, t Target) SnipResult
  ```

**為什麼這樣設計**：已經證實（`snip_prefix_test.go`）：現行 snip 每一輪都會改寫前綴中段的一則訊息（turn 0→1 改第 4 則、1→2 改第 7 則……），而且每一輪都會再追加一次 boundary 訊息，6 輪累積 6 則，又是緊接在 tool_result 之後的連續 user 訊息。快取讀取只要 10% 的價格，snip 每一次都會造成一次快取重寫（125%），所以提早 snip 通常是賠錢的。改成只在接近門檻時剪一次，而且一次剪完保護區之前的所有內容：在 Cache-first 規範下，這等同一次壓縮。

- [ ] **Step 1: 改寫測試**（`snip_prefix_test.go`，`simulateSnipTurns` 改用新簽名，並多一個 `estTokens` 參數）

```go
func TestSnip_PrefixStableBelowTrigger(t *testing.T)
// est 低於 0.8×門檻、跑 6 輪 → 每一輪的請求都是下一輪的位元組前綴（沿用現有的比對迴圈）
func TestSnip_SingleBatchAtTrigger(t *testing.T)
// 第 3 輪起 est ≥ 0.8×門檻 → 第 3 輪 Snipped==true，保護區（最後 10 則）之前所有 tool_result 都是 placeholder；
// 第 3 輪之後 est 回到門檻以下 → 第 4、5 輪的請求互為前綴（只有一次改寫）
func TestSnip_NoBoundaryMessage(t *testing.T)
// 任何一輪的歷史裡都不含 "Earlier tool call inputs and outputs have been removed"；訊息數只增加新加入的那一對
func TestSnip_PlaceholderText(t *testing.T)
// 被剪掉的 tool_result 內容 == "[snipped: earlier tool output removed to reduce context length]"
func TestSnip_IdempotentOnSnipped(t *testing.T)
// 對已經 snip 過的歷史再呼叫一次（est 仍 ≥ 觸發值）→ TokensFreed==0、Snipped==false、訊息位元組不變
```
  刪掉 `TestSnip_PrefixStableAcrossTurns`、`TestSnip_BoundaryNotRepeated`，由上面的測試取代；`compact_test.go` 的三個 `SnipCompactIfNeeded` 測試改用新簽名（est 給 ≥ 觸發值），移除 `BoundaryMessage` 的斷言。

- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/compact/ -run Snip` → 編譯失敗。
- [ ] **Step 3: 實作**：觸發條件是 `needsSnip(messages)`（保留 >12 則的條件）**且** `estTokens ≥ int(SnipTriggerFraction × AutoCompactThreshold(t))`。被剪過的區塊用 placeholder 文字辨識，跳過不計入 `TokensFreed`。`Snipped = TokensFreed > 0`。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/compact/` → 全部 PASS。
- [ ] **Step 5: Commit**：`git commit -m "fix(compact): snip 改為門檻觸發、一次剪一批，移除每輪追加的 boundary 訊息"`

---

### Task 5: engine — `QueryParams.Client` 與壓縮管線接線（C2、C3、C5、C6、C8、C11）

**Files:**
- Create: `internal/engine/compaction.go`
- Modify: `internal/engine/engine.go`, `internal/engine/query.go:96-135,174-195,452-530`
- Test: `internal/engine/tiers_test.go`（新）

**Interfaces:**
- Consumes: Task 3、4 的 `compact.Target`、`Tracker`、`ShouldCompact`、`Compact`、`SnipCompactIfNeeded`、`EstimateTokens`。
- Produces（bootstrap 會用到）：
  ```go
  // QueryParams 新欄位
  Client         api.Client       // nil → engine 預設 client
  ContextWindow  int              // 0 → 依模型解析（主腦模型時用 Config.MainContextWindow）
  CompactTracker *compact.Tracker // nil → engine 的主腦 tracker
  // （max_tokens 沿用既有的 MaxOutputTokensOverride）
  // Config 新欄位
  MainContextWindow int                         // [main] context_window；0 = 未設
  DebugLogf         func(format string, args ...any) // nil = 不記錄
  ```
  內部：`func (e *engineImpl) currentModel() string`、`clientFor(p QueryParams) api.Client`、`targetFor(p QueryParams) compact.Target`、`promptOverheadTokens(p QueryParams) int`、`runCompaction(ctx, p QueryParams, messages []types.Message, usage promptUsage) ([]types.Message, bool)`。

- [ ] **Step 1: 寫失敗測試**（`tiers_test.go`，沿用 `engine_test.go` 的 `mockClient`／`staticStreamReader`）

```go
func TestQuery_UsesParamsClient(t *testing.T)            // params.Client=tierMock → tierMock.Stream 被呼叫 1 次，engine 預設 client 0 次
func TestQuery_DefaultClientWithoutParamsClient(t *testing.T) // 反過來
func TestQuery_MainLoopRequestUnchangedWithoutTiers(t *testing.T)
// 用 buildRequestWithModel 對同一組 params/messages 產生請求，與本 task 修改前存下的 golden JSON（testdata/main_request.golden.json，Step 2 產生）位元組完全相同
func TestQuery_TierCompactionHitsTierClient(t *testing.T)
// params.Client=tierMock、Model="glm"、ContextWindow=20000、訊息估算 > 門檻 → tierMock.Complete 收到 Model=="glm" 的摘要請求；預設 client 的 Complete 0 次
func TestQuery_SubAgentBreakerIsolated(t *testing.T)
// sub-agent tracker 熔斷（摘要連續失敗 3 次）後，主腦（CompactTracker nil）的 ShouldCompact 仍為 true
func TestQuery_ParallelSubAgentsNoRace(t *testing.T)     // 4 個 Query 並行，各帶自己的 Tracker 並觸發壓縮，-race 無報告
func TestQuery_MaxOutputOverrideFromBinding(t *testing.T) // params.MaxOutputTokensOverride=8192 → 串流請求 MaxTokens==8192
func TestQuery_FallbackIgnoredWithParamsClient(t *testing.T)
// params.Client=tierMock、FallbackModel="x"、第一次 Stream 回 ErrKindContextWindow → 不重試，回傳錯誤；Stream 共 1 次
func TestSetModel_ConcurrentSafe(t *testing.T)           // SetModel 與 Query 並行，-race 無報告；之後主腦摘要請求的 Model == 新模型（C5）
func TestThreshold_IncludesPromptOverhead(t *testing.T)
// 訊息本身低於門檻，但 system prompt + 工具 schema 估算加上去會超過 → 觸發壓縮
func TestThreshold_PrefersLastUsage(t *testing.T)
// 第一輪假串流回 usage.InputTokens 大於門檻 → 第二輪開頭觸發壓縮（即使 JSON÷4 估算值低於門檻）
```

- [ ] **Step 2: 先產生 golden**（修改任何程式碼之前）：在測試裡加 `-update` flag 寫入 `testdata/main_request.golden.json`，執行 `go test ./internal/engine/ -run MainLoopRequestUnchanged -update` 一次，把 golden 檔加入 git。
- [ ] **Step 3: 執行，確認失敗**：`go test ./internal/engine/` → 編譯失敗（新欄位未定義）。
- [ ] **Step 4: 實作**
  - `engine.go`：`model` 改由 `modelMu sync.RWMutex` 保護；`SetModel` 取寫鎖；`New` 建 `mainTracker: compact.NewTracker()`、`autoCompactor: compact.NewAutoCompactor()`。
  - `targetFor`：`Client=clientFor(p)`；`Model = p.Model`，空字串時用 `currentModel()`；`ContextWindow = p.ContextWindow`，若為 0、`p.Client == nil`、且 Model == `currentModel()`，就用 `e.mainContextWindow`；`MaxOutputTokens = p.MaxOutputTokensOverride`，0 時用 `e.maxTokens`。
  - 估算（C8）：
    ```
    if usage.promptTokens > 0 && usage.msgCount <= len(messages):
        est = usage.promptTokens + EstimateTokens(messages[usage.msgCount:])
    else:
        est = EstimateTokens(messages) + promptOverheadTokens(p)
    ```
    `promptUsage{promptTokens = InputTokens+CacheRead+CacheCreation; msgCount = 送出請求時的 len(messages)}`，每輪串流結束後更新；`runCompaction` 改寫過訊息（snip 或 auto）之後歸零。`promptOverheadTokens` = system prompt 文字 ÷4 + 工具 schema JSON ÷4。把 `buildRequestWithModel` 裡組工具 schema 的迴圈抽成 `e.toolSchemas(p)`，兩邊共用。**`buildRequestWithModel` 輸出的位元組不能變**（golden 測試把關）。
  - `runCompaction` 順序：估算 → snip（`SnipCompactIfNeeded(messages, est, tgt)`，`Snipped` 就重新估算）→ micro（不變）→ `ShouldCompact` → `Compact`。sub-agent（`p.Client != nil`）壓縮時，若 `DebugLogf != nil`，記一行 `"compaction for tiered sub-agent: model=%s window=%d"`。
  - `streamResponse` 多帶一個 `client api.Client` 參數，用 `clientFor(params)` 傳入。
  - C6：`params.FallbackModel != "" && params.Client == nil` 才走 fallback。
  - `query.go` 修改後要 ≤800 行；壓縮管線整段移到 `compaction.go`。
- [ ] **Step 5: 執行，確認通過**：`go test -race ./internal/engine/` → PASS（`TestAdapterStopAgent` 是 coordinator 包的已知 flake，與此無關）。
- [ ] **Step 6: Commit**：`git commit -m "feat(engine): QueryParams.Client 依查詢覆寫 client，壓縮改依查詢 Target"`

---

### Task 6: agenttype 別名函式與 Agent 工具說明

**Files:**
- Modify: `internal/agenttype/agenttype.go`、`internal/tools/agent/agent.go:111`

**Interfaces:**
- Produces: `func ResolveAliasForProvider(provider, parentModel, model string) string`（包裝既有的 `resolveModelAliasForProvider`，provider 等於 `"openai"` 時視為 openai）。

- [ ] **Step 1: 寫失敗測試**：`TestResolveAliasForProvider`：`("openai","qwen4-max","haiku")==""`、`("direct","claude-haiku-4-5","haiku")=="claude-haiku-4-5"`、`("direct","x","qwen-turbo")=="qwen-turbo"`。
- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/agenttype/` → FAIL。
- [ ] **Step 3: 實作**：導出函式；`AgentProfile.Model` 的 GoDoc 列出可用值：`""`、`inherit`、等級名（lite/pro/max 或 config.toml 自訂）、舊別名 haiku/sonnet/opus、完整模型 ID、`providerID:modelID`。Agent 工具 description 改成：`"Optional model override: a tier (lite, pro, max), a full model ID, or providerID:modelID from ~/.glaude/config.toml. Legacy aliases haiku/sonnet/opus map to lite/pro/max. Ignored for agent_type \"fork\" — forks always inherit the parent model."`
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/agenttype/ ./internal/tools/agent/` → PASS。（explore 改成 `"lite"` 放在 Task 7，因為未綁定時的反向映射在那裡；先改會讓 openai 下的 explore 送出 `"lite"` 而回 404。）
- [ ] **Step 5: Commit**：`git commit -m "feat(agenttype): 導出 ResolveAliasForProvider，Agent 工具 model 說明改為等級語意"`

---

### Task 7: bootstrap 等級路由與兩條建構路徑接線

**Files:**
- Create: `internal/bootstrap/tiers.go`, `internal/bootstrap/tiers_test.go`
- Modify: explore profile 的定義檔（`grep -rn '"haiku"' internal/agenttype` 找到的那一處）與 `internal/agenttype/builtins_test.go:116`（期望值改成 `"lite"`）, `internal/bootstrap/wire.go:50-60（ContainerOptions）,64-100（AppContainer）,119-275,287-410,497-667`, `internal/bootstrap/wire_fork_test.go`

**Interfaces:**
- Consumes: Task 1/2 `modeltier.Load/Resolve/LegacyAlias/Provider`；Task 5 `QueryParams.Client/ContextWindow/CompactTracker`；Task 6 `agenttype.ResolveAliasForProvider`。
- Produces:
  ```go
  // ContainerOptions 新欄位：ProviderClientFactory func(id string, p modeltier.Provider, apiKey string) (api.Client, error) // nil → api.NewClient
  // AppContainer 新欄位：Tiers *modeltier.Store（永不為 nil）
  type tierRouter struct { store *modeltier.Store; clients map[string]api.Client; keyErr map[string]error }
  func loadTierRouter(homeDir string, mainClient api.Client, factory func(string, modeltier.Provider, string) (api.Client, error)) (*tierRouter, []string)
  type subAgentTarget struct { Model string; Client api.Client; ContextWindow, MaxTokens int }
  func resolveSubAgentTarget(r *tierRouter, provider, parentModel, toolModel, profileModel string) (subAgentTarget, error)
  // buildSubAgentQuery 簽名：在 agentID 之後加 router *tierRouter，回傳值加 error
  ```

- [ ] **Step 1: 寫失敗測試**（`tiers_test.go`，用假的 factory 建出可辨識的 client）

```go
func TestResolveSubAgentTarget_TierBound(t *testing.T)      // explore 的 "lite" 綁 zai:glm → Model "glm-5.3-flash"、Client == clients["zai"]、MaxTokens 8192
func TestResolveSubAgentTarget_MainProvider(t *testing.T)   // lite 綁 main:claude-haiku-5-5 → Client == nil、Model "claude-haiku-5-5"
func TestResolveSubAgentTarget_UnboundLegacyMapsBack(t *testing.T)
// lite 未綁定：openai provider → Model ""（繼承）；direct provider、parent "claude-haiku-4-5" → "claude-haiku-4-5"
func TestResolveSubAgentTarget_UnboundCustomInherits(t *testing.T) // [tiers.foo] 空 → toolModel "foo" → Model ""、Client nil
func TestResolveSubAgentTarget_NotTierUnchanged(t *testing.T)    // toolModel "qwen-turbo" → 與 agenttype.ResolveModelForProvider 結果相同
func TestResolveSubAgentTarget_EnvUnboundTierOnOpenAI(t *testing.T)
// t.Setenv("CLAUDE_CODE_SUBAGENT_MODEL","lite")、lite 未綁定、openai → Model ""（不能原樣放行 "lite" 或 "haiku"）
func TestResolveSubAgentTarget_EnvBoundTier(t *testing.T)   // env "pro" 已綁 → 用 pro 的綁定，優先於 toolModel
func TestResolveSubAgentTarget_MissingKeyErrors(t *testing.T)
// zai 的 ZAI_API_KEY 未設 → error 訊息含 "ZAI_API_KEY" 與 "lite"；不回傳主腦 client
func TestResolveSubAgentTarget_NilRouter(t *testing.T)      // router nil → 與 ResolveModelForProvider 結果完全相同
func TestBuildRunAgentFn_TierResumeKeepsBinding(t *testing.T)
// 派工時 lite→zai；之後 store.Bind("lite","openrouter",…)；同一個 agentID 續跑 → params.Client 仍是 zai、Model 不變、CompactTracker 是同一個指標
func TestBuildRunAgentFn_EachSubAgentOwnTracker(t *testing.T) // 兩個 agentID → CompactTracker 都不為 nil 且不同
func TestBuildForkQuery_NeverSetsClient(t *testing.T)        // 即使 lite 已綁定，fork 的 params.Client == nil、ContextWindow == 0
```
  `wire_fork_test.go` 既有的 `buildSubAgentQuery(...)` 呼叫補上 `nil` router，並處理多出來的 error 回傳值；`TestBuildSubAgentQuery_OpenAIExploreAliasFallback` 的斷言**不改**，必須照樣通過。

- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/bootstrap/ -run 'SubAgent|RunAgent|Fork'` → 編譯失敗。
- [ ] **Step 3: 實作 `resolveSubAgentTarget`**

```
if r == nil → 把 toolModel/profileModel 中的 lite|pro|max 經 LegacyAlias 轉回舊別名（env 交給 ResolveModelForProvider 照舊處理），再 {Model: agenttype.ResolveModelForProvider(...)}
cand := 第一個非空：env CLAUDE_CODE_SUBAGENT_MODEL、toolModel、profileModel
if cand == "" 或 "inherit" → {Model: ""}
b, kind := r.store.Resolve(cand)
switch kind:
  Bound:
    if b.Provider == "main" → {Model: b.Model, ContextWindow: b.ContextWindow, MaxTokens: b.MaxTokens}
    if err := r.keyErr[b.Provider]; err != nil → error（"tier %q → provider %q: %w"，err 本身點名環境變數）
    → {Model: b.Model, Client: r.clients[b.Provider], ContextWindow, MaxTokens}
  Unbound:
    if alias, ok := modeltier.LegacyAlias(b.Tier); ok → {Model: agenttype.ResolveAliasForProvider(provider, parentModel, alias)}
    → {Model: ""}
  NotTier:
    → {Model: agenttype.ResolveModelForProvider(provider, parentModel, toolModel, profileModel)}
```
  （`Resolve` 遇到 Unbound 時也要填 `Binding.Tier`，讓這裡拿得到正規化後的等級名，例如 "haiku" → "lite"。Task 1 的 `TestResolve_UnboundKnownTier` 補上這條斷言。）

- [ ] **Step 4: 實作 `loadTierRouter` 與接線**
  - 對每個 provider：`os.Getenv(APIKeyEnv)` 為空 → `keyErr[id] = fmt.Errorf("environment variable %s is not set", env)`，不建 client；否則用 factory（nil → `api.NewClient(api.ClientConfig{Provider: 對應值, BaseURL, APIKey}, nil)`）。`clients["main"] = mainClient`。
  - `BuildContainer` 與 `BuildContainerWithClient` 都要：`loadTierRouter(opts.HomeDir, apiClient, opts.ProviderClientFactory)`；警告在 `opts.Debug` 或 `opts.Verbose` 時印到 stderr（依現有的 MCP 警告寫法）。`engine.Config{MainContextWindow: store.MainContextWindow(), DebugLogf: …}`；`buildRunAgentFn(eng, reg, store, onProgress, agentProfiles, router)`；`AppContainer.Tiers = router.store`。
  - `buildSubAgentQuery`：非 fork、非續跑時呼叫 `resolveSubAgentTarget`，並填入 `Model`、`Client`、`ContextWindow`、`MaxOutputTokensOverride`。`buildRunAgentFn`：若 `params.CompactTracker == nil`，就設成 `compact.NewTracker()`（fork 也要）；`buildSubAgentQuery` 回傳的 error 直接當成派工失敗回傳。
  - explore profile 的 `Model` 改成 `"lite"`，`builtins_test.go:116` 的期望值同步修改；`TestBuildSubAgentQuery_OpenAIExploreAliasFallback` 在 router 為 nil 時也要通過，所以 `resolveSubAgentTarget` 的 nil-router 分支要先把 `"lite"`/`"pro"`/`"max"` 經 `LegacyAlias` 轉回舊別名，再交給 `ResolveModelForProvider`（`TestResolveSubAgentTarget_NilRouter` 補一條：nil router、openai、profile `"lite"` → Model `""`）。
  - 新增的接線邏輯都放在 `tiers.go`；`wire.go` 只加呼叫，不要讓它再變長（現在已經 1075 行）。
- [ ] **Step 5: 執行，確認通過**：`go test -race ./internal/bootstrap/` → PASS。
- [ ] **Step 6: 後端段落驗證**：`make vet && go test -race ./internal/... && gofmt -l internal/ cmd/` → 全部 PASS、gofmt 無輸出。
- [ ] **Step 7: Commit**：`git commit -m "feat(bootstrap): sub-agent 依等級路由到 provider client"`
- [ ] **Step 8: 段落審查**：開 reviewer subagent 審 `git diff dev...HEAD -- internal/`（Task 1～7），重點：反目標（主腦、inherit、fork 行為）、C1～C12 是否都有測試、並發。依 superpowers:receiving-code-review 處理意見。

---

### Task 8: `/tier` 指令與 TUI 接線

**Files:**
- Create: `internal/commands/tier.go`, `internal/commands/tier_test.go`
- Modify: `internal/commands/registry.go:64-83`, `internal/commands/builtins.go`（`RegisterBuiltins` 加 `cmdTier()`）, `internal/tui/init.go:30-50`, `internal/tui/model.go:221-262`, `internal/tui/update.go:423-431,516-519`, `internal/bootstrap/root.go:228-244`

**Interfaces:**
- Consumes: Task 2 `Store.List/Providers/Bind/Unbind/MainContextWindow`；Task 3 `compact.ResolveContextWindow`。
- Produces:
  ```go
  type TierManager interface {
      List() []modeltier.Binding
      Providers() []modeltier.ProviderStatus
      Bind(tier, providerID, modelID string) error
      Unbind(tier string) error
      MainContextWindow() int
  }
  var _ TierManager = (*modeltier.Store)(nil)
  // CommandContext 新欄位：Tiers TierManager（nil = 功能未啟用）
  // tui.New 在 mcpPool 之後加參數 tiers commands.TierManager
  ```

- [ ] **Step 1: 寫失敗測試**（`tier_test.go`，用假的 TierManager；斷言用「詞」比對，不比對完整字串）

```go
func TestTier_ListShowsBindingsAndProviders(t *testing.T) // 輸出含 "lite"、"zai:glm-5.3-flash"、"max"、"未綁定"、"ZAI_API_KEY"、"main"
func TestTier_Bind(t *testing.T)            // args "lite openrouter:qwen/qwen3:free" → 假 manager 收到 Bind("lite","openrouter","qwen/qwen3:free")；輸出含 "下一個派出的 sub-agent"
func TestTier_Unbind(t *testing.T)          // args "pro none" → Unbind("pro")
func TestTier_BindErrorDisplayed(t *testing.T) // Bind 回 error → Result.Display == DisplayError，Text 含錯誤訊息
func TestTier_BadArgs(t *testing.T)         // "lite" 少一個參數、"lite nocolon" → DisplayError 並附用法；不呼叫 Bind
func TestTier_NilManager(t *testing.T)      // ctx.Tiers == nil → DisplayError "tier 功能未啟用"
func TestTier_NoteCommentsLost(t *testing.T) // 成功綁定的輸出含 "註解" 與 "config.toml"（提醒重新寫檔會遺失註解）
```
  TUI：`TestStatusBar_UsesMainContextWindow`（`tui_test.go`）：manager 的 `MainContextWindow()` 回 131072、模型 "qwen" → `statusBar.contextWindow == 131072`；`/model` 切換後也維持用 `ResolveContextWindow(newModel, mgr.MainContextWindow())`。

- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/commands/ ./internal/tui/ -run 'Tier|MainContextWindow'` → 編譯失敗。
- [ ] **Step 3: 實作**：`/tier` 解析 `args`（`strings.Fields`）：0 個 → 列表；2 個且第二個是 `none` → Unbind；2 個且含 `:` → 以第一個 `:` 切開後 Bind；其他情況 → 用法錯誤。列表排版由實作者決定。Bind 是同步的小檔案寫入，直接在 Execute 裡做（我的決定：比照其他同步指令，不另外走 tea.Cmd）。TUI 在建 `cmdCtx` 時填 `Tiers: m.tiers`；兩個 `compact.ContextWindow(model)` 呼叫點改成 `compact.ResolveContextWindow(model, mainWindow(m.tiers))`（nil → 0）。`root.go` 傳入 `container.Tiers`。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/commands/ ./internal/tui/ ./internal/bootstrap/` → PASS。
- [ ] **Step 5: Commit**：`git commit -m "feat(commands): /tier 指令查看與改綁定等級"`

---

### Task 9: 文件、生成檔與合併前驗證

**Files:**
- Create: `docs/config-toml.md`
- Modify: `AGENTS.md`（API Provider 段加一行連結）、各包 `CONTEXT.md` 的 `## Design Notes`（modeltier、compact、engine、bootstrap、agenttype、commands、tui、tools/agent）、`docs/project/design/<layer>.md`（engine、tools、tui、bootstrap 介面變更，依 `docs/project/doc-sync-policy.md`）、spec §6 標註「已實作」

- [ ] **Step 1: `docs/config-toml.md`**：spec §5.1 的完整範例；欄位表（format、base_url、api_key_env、models.*.context_window/max_tokens、tiers.*、[main]）；`main` 保留 id；兩個值的解析順序（照抄 Global Constraints）；`/tier` 用法與「重新寫檔會遺失註解」；新增 provider 要重新啟動；bedrock/vertex 不支援（C10）。
- [ ] **Step 2: 生成文件**：`make docs`，然後**才**補各包的 Design Notes。compact 要寫：C9/C12 的證據與「snip 是一次壓縮」的理由、`SnipTriggerFraction`、`SnipTokensFreed` 不再相減。engine 要寫：§6「修正前行為與修法」、C3 主腦 tracker 整個會話累計、C8 主腦提早壓縮是負責人認可的例外。bootstrap 要寫：續跑沿用派工當下的綁定、fork 不帶 Client。
- [ ] **Step 3: 合併前驗證**（全部要通過）

```bash
make all            # vet + test(-race) + build
make docs-check
make debt-check     # TODO(dep) 11 / not yet implemented 0，不可上升
gofmt -l internal/ cmd/   # 無輸出
wc -l internal/engine/query.go internal/bootstrap/tiers.go internal/modeltier/*.go internal/compact/*.go  # 新增或修改過的檔案 ≤800 行
```
- [ ] **Step 4: 手動煙霧測試**：在 `~/.glaude/config.toml` 寫一個 `[tiers.lite] provider="main" model="claude-haiku-5-5"`，啟動 `bin/glaude`，執行 `/tier` → 看到綁定；派一個 explore → `--debug` 的請求記錄中模型是 `claude-haiku-5-5`；`/tier lite none` → 再派一次 explore，行為與修改前相同。
- [ ] **Step 5: Commit**：`git commit -m "docs: config.toml 使用說明與各包 Design Notes"`
- [ ] **Step 6: 整個分支審查**：開 reviewer subagent 審 `git diff dev...HEAD`；PR 說明要回答 Cache-first 三問（照 spec §7，加上 C8 與 snip 的補充），再交給 superpowers:finishing-a-development-branch。
