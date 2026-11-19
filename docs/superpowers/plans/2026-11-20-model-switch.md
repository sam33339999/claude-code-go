# `/model` 選單切換主腦 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **本專案的執行方式**（負責人全域 CLAUDE.md）：主 session 依序實作；Task 3（後端完成）時開 reviewer subagent 審那一段 diff；Task 5、合併前再審整個分支。

**Goal:** `/model` 開啟 default＋lite/pro/max 選單（也接受 `/model <參考>`），把主腦切換到選定目標的 provider client、模型、上下文長度與輸出上限，只在本 session 有效；對話有內容時先警示快取失效。

**Architecture:** engine 以 `MainTarget` 取代單一 `model` 欄位（mutex 保護，每輪查詢開始取快照）。bootstrap 的 `modelSwitcher` 解析參考並呼叫 `SetMainTarget`；派工時把當下主腦目標凍結進 sub-agent params。TUI 新增選單與確認框。

**Tech Stack:** Go 1.26、bubbletea v2、既有 `modeltier` / `tierRouter`。

**Spec:** `docs/superpowers/specs/2026-11-20-model-switch-design.md`

## Global Constraints

- 只在 session 有效：不寫 `settings.json` / `config.toml`。
- 不切換時，主腦請求位元組不變（`TestQuery_MainLoopRequestUnchangedWithoutTiers` 必須維持）。
- 查詢進行中（TUI `isLoading`）拒絕切換；`runQueryLoop` 在開始時取一次 `MainTarget` 快照，整輪不換。
- `main:<模型>` 等級永遠用**啟動時**的主腦 client（router 的 `clients["main"]`）。
- 已派出的 sub-agent（inherit / fork）續跑時凍結在派工當下的目標。
- 切換存解析後的目標快照；之後 `/tier` 改綁定不影響主腦。
- provider 改變時，從主腦歷史移除 thinking 區塊；同 provider 換模型不清洗。
- 不新增 LLM 呼叫；不在切換前預先壓縮。
- TUI/commands 不 import bootstrap；UI 文字英文；TUI 測試用「詞」斷言。
- 檔案 ≤800 行、函式 ≤80 行；導出符號 GoDoc；`make docs` 後補 Design Notes。

## Review Focus

1. **切換後立刻派出 inherit sub-agent**：它要打新主腦的 client 與模型，而不是啟動時的。→ Task 2 `TestBuildRunAgentFn_InheritUsesCurrentMainTarget`。
2. **背景 sub-agent 跨過切換時刻再被 SendMessage 續跑**：仍用派工當下的 client/model。→ Task 2 `TestBuildRunAgentFn_InheritResumeFrozen`。
3. **切換到金鑰缺失或已被 `/tier none` 的等級**：選單標示不可用；打字路徑回錯誤；主腦不變。→ Task 3 `TestModelSwitcher_UnavailableNotApplied`。
4. **切到 openai 格式的 provider 後，explore（`lite` 未綁定）**：別名規則用 openai 邏輯（繼承主腦），不送出 `haiku`。→ Task 2 `TestBuildRunAgentFn_AliasUsesMainLoopProvider`。
5. **快速連按：確認框開著時又輸入 `/model`**：不得疊出兩個對話框或套用錯的目標。→ Task 4 `TestModelPicker_ConfirmBlocksReentry`。

---

### Task 1: engine `MainTarget`

**Files:** Modify `internal/engine/engine.go`, `internal/engine/compaction.go`, `internal/engine/query.go`, `internal/engine/cache_params.go`, `internal/engine/forked_agent.go`（若引用）；測試替身 `internal/bootstrap/wire_fork_test.go`（`stubQueryEngine`）、`internal/tui/tui_test.go`（`fakeEngine`）；Test `internal/engine/maintarget_test.go`

**Interfaces:**
- Produces:
  ```go
  // MainTarget is what the main loop — and every query without its own
  // Client/Model — runs on.
  type MainTarget struct {
      Client        api.Client
      ProviderID    string // "main" for the startup client, else a config.toml provider id
      Model         string
      ContextWindow int // 0 = resolve via the built-in table
      MaxTokens     int // 0 = engine default
  }
  // QueryEngine 介面新增：
  SetMainTarget(t MainTarget)
  MainTarget() MainTarget
  // CacheSafeParams 新增 Client api.Client（fork 用快照當下的 client）
  ```
  `engineImpl` 以 `mainMu sync.RWMutex` 保護 `main MainTarget`（取代 `client`/`model`/`mainContextWindow`/`maxTokens` 的直接讀取；`New` 由 Config 初始化，`ProviderID: "main"`）。`SetModel(m)` 改為「複製目前目標、只換 Model」。

- [ ] **Step 1: 寫失敗測試**

```go
func TestSetMainTarget_MainLoopUsesNewClient(t *testing.T)   // SetMainTarget{Client: b, Model: "glm"} 後 Query 無 Client 的 params → b.Stream 1 次、req.Model=="glm"、原 client 0 次
func TestSetMainTarget_ParamsClientUnaffected(t *testing.T)  // 帶 params.Client=c 的查詢仍打 c
func TestSetMainTarget_SnapshotPerLoop(t *testing.T)         // 第一輪 streamFn 內呼叫 SetMainTarget(新 client)、回 tool_use；第二輪請求仍打舊 client、舊 model
func TestSetMainTarget_ConcurrentNoRace(t *testing.T)        // 並行 SetMainTarget 與 Query 各 50 次，-race 無報告
func TestSetMainTarget_CompactionUsesNewWindow(t *testing.T) // SetMainTarget{ContextWindow: 40000} 後 32k token 歷史觸發壓縮，摘要打新 client
func TestSetMainTarget_StripsThinkingOnProviderChange(t *testing.T) // 歷史含 thinking 區塊；SetMainTarget 換 ProviderID → GetMessages 不含 thinking、text/tool_use 保留；同 ProviderID 只換 Model → thinking 保留
func TestMainTarget_DefaultsFromConfig(t *testing.T)         // New(Config{Client:a, Model:"m", MainContextWindow:131072, MaxTokens:4096}).MainTarget() == {a,"main","m",131072,4096}
func TestCacheSafeParams_CarriesClient(t *testing.T)        // 主腦一輪後 ToolUseContext.CacheParams.Client == 目前主腦 client
```
  既有 `TestSetModel` 改為斷言 `MainTarget().Model`。
- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/engine/` → 編譯失敗。
- [ ] **Step 3: 實作**：`runQueryLoop` 開頭 `mt := e.MainTarget()`，`clientFor`/`activeModelFor`/`targetFor`/`buildRequestWithModel` 改收 `mt`（主腦 maxTokens、mainContextWindow 從 `mt` 取）。`stripThinking(msgs []types.Message) []types.Message`：移除 `ContentTypeThinking` 區塊，移除後為空的 assistant 訊息整則刪掉。`SetMainTarget` 在 `mainMu` 內換目標；ProviderID 不同時在 `mu` 內對 `e.messages` 做 `stripThinking`。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/engine/ ./internal/bootstrap/ ./internal/tui/` → PASS（golden 測試必須仍過）。
- [ ] **Step 5: Commit**：`feat(engine): MainTarget 可在會話中切換主腦 client 與模型`

---

### Task 2: bootstrap 派工凍結與 `main:` 綁定

**Files:** Modify `internal/bootstrap/tiers.go`, `internal/bootstrap/wire.go:~535-680`（`buildRunAgentFn` / `buildSubAgentQuery` / `buildForkQuery`）; Test `internal/bootstrap/tiers_test.go`, `wire_fork_test.go`

**Interfaces:**
- Consumes: Task 1 `MainTarget()`、`CacheSafeParams.Client`。
- Produces: `buildSubAgentQuery(..., router, main engine.MainTarget)`（多一個參數，呼叫端傳 `eng.MainTarget()`）。

- [ ] **Step 1: 寫失敗測試**

```go
func TestBuildRunAgentFn_InheritUsesCurrentMainTarget(t *testing.T) // stub engine MainTarget 回 {Client: z, Model: "glm", ContextWindow: 131072}；worker（inherit）派工 → params.Client==z、Model=="glm"、ContextWindow==131072
func TestBuildRunAgentFn_InheritResumeFrozen(t *testing.T)         // 派工後把 stub 的 MainTarget 改成 {Client: o, Model: "q"}；同 agentID 續跑 → params 仍是 z / glm
func TestResolveSubAgentTarget_MainBindingUsesStartupClient(t *testing.T) // lite=main:claude-haiku → Client == clients["main"]（不是 nil）
func TestBuildForkQuery_UsesSnapshotClient(t *testing.T)            // CacheParams{Model:"m1", Client: x} → params.Client==x、Model=="m1"；無快照 → 當下 MainTarget
func TestBuildRunAgentFn_AliasUsesMainLoopProvider(t *testing.T)    // AppState.MainLoopModel.Provider "openai"、Settings.Provider "direct"、lite 未綁定 → explore 的 Model == ""（繼承），不是 "haiku"
```
  `TestBuildForkQuery_NeverSetsClient` 改名為 `TestBuildForkQuery_ClientIsMainTarget`：fork 的 Client 等於快照或主腦目標，絕不是等級的 client。
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**：inherit（`resolveSubAgentTarget` 回 Model "" 且 Client nil）→ 填入 `main` 的 Client/Model/ContextWindow/MaxTokens。`main:` 綁定 → `Client: r.clients["main"]`（router 為 nil 時維持 nil）。fork → `cache.Client`，nil 時用 `main.Client`。別名解析的 provider：`st.MainLoopModel.Provider`，空值退回 `st.Settings.Provider`。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/bootstrap/`（既有 openai explore 測試必須仍過）。
- [ ] **Step 5: Commit**：`fix(bootstrap): sub-agent 派工時凍結當下主腦目標；main: 等級固定用啟動時的 client`

---

### Task 3: `ModelSwitcher` 與 `/model` 指令

**Files:** Create `internal/commands/modelswitch.go`, `internal/bootstrap/modelswitch.go`（＋各自 `_test.go`）; Modify `internal/commands/builtins.go`（`cmdModel`）, `internal/commands/registry.go`（`Result.ModelRef`）, `internal/bootstrap/wire.go`（`AppContainer.ModelSwitcher`）

**Interfaces:**
- Produces:
  ```go
  // commands
  type ModelOption struct {
      Name, Provider, Model string // Name: "default" | tier | "" for an ad-hoc ref
      ContextWindow, MaxTokens int
      Available bool; Reason string; Current bool
  }
  type ModelSwitcher interface {
      Options() []ModelOption
      Resolve(ref string) (ModelOption, error) // option name, alias, provider:model, or model ID on the current provider
      Apply(opt ModelOption) error            // retargets the main loop; error if !Available
  }
  // Result 新增 ModelRef string（/model <ref>）；/model 無參數 → OpenDialog "model"
  // bootstrap
  func newModelSwitcher(eng engine.QueryEngine, router *tierRouter, store *state.AppStateStore, startup engine.MainTarget) *modelSwitcher
  ```
- [ ] **Step 1: 寫失敗測試**

```go
// commands
func TestModelCommand_NoArgsOpensPicker(t *testing.T) // Result.OpenDialog == "model"
func TestModelCommand_RefPassesThrough(t *testing.T)  // "/model pro" → Result.ModelRef == "pro"、NewModel == ""
// bootstrap
func TestModelSwitcher_Options(t *testing.T)          // default + lite/pro/max；lite 綁 zai（有 key）Available、max 未綁定 Reason "unbound"、pro 綁 openrouter 但 key 缺 Reason 含 "OPENROUTER_API_KEY"；啟動時 Current 為 default
func TestModelSwitcher_ResolvePaths(t *testing.T)     // "default"、"lite"、"haiku"（→lite）、"openrouter:x/y"、"claude-foo"（目前 provider 上的 ID）各自解析正確
func TestModelSwitcher_ApplyRetargets(t *testing.T)   // Apply(lite) → stub engine 收到 SetMainTarget{Client: zai client, ProviderID "zai", Model glm, ContextWindow, MaxTokens}；AppState.MainLoopModel == {glm, "openai"}；Options() 中 lite.Current
func TestModelSwitcher_DefaultRestoresStartup(t *testing.T) // Apply(lite) 再 Apply(default) → SetMainTarget == startup
func TestModelSwitcher_UnavailableNotApplied(t *testing.T)  // Apply(max 未綁定) → error、engine 未收到 SetMainTarget
func TestModelSwitcher_TierRebindDoesNotMoveMain(t *testing.T) // Apply(lite) 後 store.Bind("lite", ...other) → engine 目標不變；Options() 的 lite 不再 Current
```
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**：`Options` 即時讀 `router.store.List()`；不可用原因：未綁定 → "unbound"；`keyErr` → 錯誤字串；沒有 client → "added after startup; restart"。`Current` 以 `MainTarget()` 的 ProviderID＋Model 比對。純模型 ID 參考 → 目前 MainTarget 換 Model（ContextWindow 歸 0、MaxTokens 沿用 startup 預設）。`Apply` 成功後更新 `AppState.MainLoopModel{ModelID, Provider: 線格式}`。`AppContainer.ModelSwitcher` 兩條建構路徑都接。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/commands/ ./internal/bootstrap/`。
- [ ] **Step 5: Commit**：`feat(bootstrap): ModelSwitcher 在 default 與等級之間切換主腦`
- [ ] **Step 6: 段落驗證與審查**：`make vet && go test -race ./internal/... && gofmt -l internal/ cmd/`；reviewer subagent 審 Task 1–3 diff（重點：Global Constraints 每一條、golden 位元組、並發、凍結語意）。

---

### Task 4: TUI 選單、確認框與打字路徑

**Files:** Create `internal/tui/modelpicker.go`, `internal/tui/modelpicker_test.go`; Modify `internal/tui/{model,keys,update,view,init}.go`、`newAppModel`/`New` 呼叫點、`internal/bootstrap/root.go`

**Interfaces:**
- Consumes: Task 3 `ModelSwitcher`、`ModelOption`、`Result.ModelRef`。
- Produces: `tui.New`/`newAppModel` 在 `setup` 後新增 `models commands.ModelSwitcher`；`dialogModelPicker`、`dialogModelConfirm`。

- [ ] **Step 1: 寫失敗測試**

```go
func TestModelPicker_OpensWithOptions(t *testing.T)       // OpenDialog "model" → dialogModelPicker；view 含 default、lite、"unbound"、"current"
func TestModelPicker_SkipsUnavailable(t *testing.T)       // ↓ 跳過不可用列
func TestModelPicker_ConfirmWhenHistory(t *testing.T)     // engine 歷史非空、選 lite → dialogModelConfirm，view 含 "cache" 與 "≈"；n → 未 Apply；y → Apply(lite) 一次、狀態列 model 與 contextWindow 更新、system 訊息含 "session only"
func TestModelPicker_NoConfirmWhenEmpty(t *testing.T)     // 歷史為空 → 直接 Apply
func TestModelPicker_SameTargetNoop(t *testing.T)         // 選目前 Current → 不 Apply、訊息含 "already"
func TestModelPicker_WindowWarning(t *testing.T)          // 歷史估算 > 新目標門檻 → 確認框含 "compacted"
func TestModelPicker_RejectWhileLoading(t *testing.T)     // isLoading 時 /model → 不開選單、訊息含 "Esc"
func TestModelPicker_TypedRef(t *testing.T)               // Result{ModelRef:"pro"} → Resolve("pro")；有歷史時走確認框；Resolve 錯誤 → 錯誤訊息
func TestModelPicker_ConfirmBlocksReentry(t *testing.T)   // 確認框開著時套用 Result{OpenDialog:"model"} → activeDialog 仍為 confirm，Apply 未被呼叫
func TestModelPicker_NilSwitcherFallsBack(t *testing.T)   // switcher nil、Result{ModelRef:"x"} → queryEngine.SetModel("x")（舊行為）
```
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**：選單比照 `resumePicker`（↑/↓、Enter、Esc）；確認框比照 `handleCompactKey`（y/n/Esc）。估算用 `compact.EstimateTokens(m.queryEngine.GetMessages())`；門檻用 `compact.AutoCompactThreshold(compact.Target{Model, ContextWindow, MaxOutputTokens})`。`/model` 指令在查詢中會被排隊（slash 指令既有行為），佇列 drain 時若仍 `isLoading` 則拒絕。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/tui/ ./internal/bootstrap/`。
- [ ] **Step 5: Commit**：`feat(tui): /model 選單與快取失效確認`

---

### Task 5: 文件與合併前驗證

- [ ] **Step 1**：`docs/config-toml.md` 新增「在會話中切換主腦（/model）」一節；provider-tiers 設計 §10 進度表把第 2 件改為完成；model-switch spec 狀態改為已實作。
- [ ] **Step 2**：`make docs`（還原純時間戳變動），補 Design Notes（engine、bootstrap、commands、tui）與 `docs/project/design/{core,cli,tui}.md` 變更記錄。
- [ ] **Step 3**：`make all && make docs-check && make debt-check`；`gofmt -l internal/ cmd/` 無輸出。
- [ ] **Step 4**：Commit `docs: /model 切換說明與 Design Notes`。
- [ ] **Step 5**：reviewer subagent 審 `git diff dev...HEAD`；處理後依 superpowers:finishing-a-development-branch 收尾。
