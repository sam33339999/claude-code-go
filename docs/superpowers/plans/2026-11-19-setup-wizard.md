# `/setup` 首次設定引導 實作計畫

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **本專案的執行方式**（負責人全域 CLAUDE.md）：主 session 依序實作（superpowers:executing-plans）；Task 4（後端：api / config / modeltier / commands 介面）結束時開 reviewer subagent 審那一段 diff；Task 8 結束、合併前再審整個分支。

**Goal:** 新增 `/setup`：在 TUI 裡引導使用者輸入端點與金鑰、以 `GET /models` 驗證、從可搜尋的多選清單挑模型、分配主腦與 lite/pro/max，寫入 `~/.glaude/settings.json` 與 `~/.glaude/config.toml`，請使用者重新啟動。

**Architecture:** 新增 `api.ListModels`（獨立函式，不進 `api.Client` 介面）與 `config.WriteUserMainModel`（標準庫 read-modify-write）。`commands` 定義 `SetupBackend` 介面與資料型別，bootstrap 的 `setup.go` 實作它（讀全域設定、解析沿用金鑰、寫檔、覆蓋偵測）。TUI 的 `setupWizard` 是純狀態機，網路與寫檔透過 `tea.Cmd` 呼叫 backend。

**Tech Stack:** Go 1.26、`net/http`、`charm.land/bubbletea/v2`、既有 `internal/config` OrderedMap、`internal/modeltier`。

**Spec:** `docs/superpowers/specs/2026-11-19-setup-wizard-design.md`

## Global Constraints

- 依賴方向：TUI/commands 禁止 import bootstrap；`commands` 不 import `internal/api`（資料型別自己定義）；`internal/config` 只用標準庫。
- MVU：`Update()` 不做 I/O；ListModels / Apply 一律在 `tea.Cmd` 內執行。
- 驗證與列模型只呼叫 `GET {base}/v1/models`，不呼叫 LLM。逾時 15 秒。
- 金鑰不得出現在：畫面（輸入以 `•` 遮蔽、確認頁只露末 4 碼）、錯誤訊息、log、`config.toml`、TUI 狀態（沿用的金鑰只在 bootstrap 內取得）。
- `settings.json` 寫入後權限 `0600`；只設定 `provider`、`baseUrl`、`model`、`apiKey`（空字串不動 `apiKey`）；原檔無法解析就不寫。
- 「現值」只從全域 `~/.glaude/settings.json`（`LayeredSettings.User`）讀。
- `config.toml` 用另外從磁碟載入的 `modeltier.Store` 寫，不動執行中那一個。
- 角色頁：帶入現有 `main:<模型>` 綁定；「不綁定」且原本是 `main:` → Unbind；綁在其他 provider 的等級不動；自訂等級不動。
- 主腦模型改變 → 清掉舊 `[main] context_window`；新主腦模型有上下文長度且不在內建表 → 寫入新值。
- provider 寫入值：Anthropic 格式寫 `"direct"`，OpenAI 相容寫 `"openai"`。
- UI 文字用英文；TUI 測試用「詞」斷言。
- 檔案 ≤800 行、函式 ≤80 行；導出符號 GoDoc；`fmt.Errorf("...: %w", err)`；I/O 函式首參 `context.Context`（`config`/`modeltier` 的純檔案寫入比照現況例外）。
- 改到導出符號的包：先 `make docs`、再補 `## Design Notes`；只保留內容有變的 CONTEXT.md（純時間戳變動的還原）。

## Review Focus

1. **貼上金鑰**：使用者用 bracketed paste 貼上 API key（`tea.PasteMsg`），期望貼進目前聚焦的欄位，而不是被丟掉。→ Task 6 `TestSetupDialog_PasteGoesToFocusedField`。
2. **驗證進行中按 esc 或關閉對話框**：之後才回來的 `setupModelsMsg` 不能把已關閉或已回上一步的對話框又推進。→ Task 6 `TestSetupDialog_StaleModelsMsgIgnored`。
3. **模型清單上千筆**（OpenRouter 約數百～上千）：過濾與捲動要即時，游標永遠在可見範圍內。→ Task 5 `TestSetupWizard_ScrollKeepsCursorVisible`。
4. **錯誤訊息洩漏金鑰**：端點把 Authorization 回顯在錯誤 body 裡時，畫面與錯誤字串不能出現金鑰。→ Task 1 `TestListModels_ErrorRedactsKey`。
5. **`settings.json` 本來是 symlink 或權限較寬**：寫入後權限為 0600，其他欄位保留。→ Task 2 `TestWriteUserMainModel_Permissions0600`（symlink 情況不處理，比照 provider-tiers 已記錄的 minor）。

---

## File Structure

| 檔案 | 動作 | 職責 |
|------|------|------|
| `internal/api/listmodels.go` | 新增 | `ModelListing`、`ListModelsError`、`ListModels` |
| `internal/api/listmodels_test.go` | 新增 | httptest |
| `internal/config/mainmodel.go` | 新增 | `MainModelSettings`、`WriteUserMainModel`、`MainModelEnvVars` |
| `internal/config/loader.go` | 修改 | `applyEnvOverrides` 改讀 `MainModelEnvVars`（清單單一來源） |
| `internal/config/mainmodel_test.go` | 新增 | |
| `internal/modeltier/store.go` | 修改 | 泛化 `commitLocked`；+`SetMainContextWindow`；+`BindWithLimits` |
| `internal/compact/window.go` | 修改 | +`KnownContextWindow(model) (int, bool)` |
| `internal/commands/setup.go` | 新增 | `SetupBackend` 介面與資料型別、`ErrModelListUnsupported`、`/setup` |
| `internal/tui/setupwizard.go` | 新增 | 純狀態機（步驟、輸入、過濾、勾選、角色、plan） |
| `internal/tui/setupwizard_view.go` | 新增 | 各步驟的 view |
| `internal/tui/setup_dialog.go` | 新增 | `dialogSetup` 接線：按鍵、paste、tea.Cmd、訊息 |
| `internal/tui/{model,keys,update,view,init}.go` | 修改 | 注入 backend、分派、啟動提示 |
| `internal/bootstrap/setup.go` | 新增 | `setupBackend` 實作 |
| `internal/bootstrap/{wire,root}.go` | 修改 | `AppContainer.Setup`、傳入 `tui.New` |

---

### Task 1: `api.ListModels`

**Files:** Create `internal/api/listmodels.go`, `internal/api/listmodels_test.go`

**Interfaces:**
- Produces:
  ```go
  type ModelListing struct { ID string; ContextWindow int }
  type ListModelsErrorKind string // "unreachable" | "unauthorized" | "forbidden" | "not_supported" | "timeout" | "http"
  type ListModelsError struct { Kind ListModelsErrorKind; StatusCode int; Message string }
  func (e *ListModelsError) Error() string
  func ListModels(ctx context.Context, cfg ClientConfig, httpClient *http.Client) ([]ModelListing, error) // nil httpClient → http.DefaultClient
  ```

- [ ] **Step 1: 寫失敗測試**（`httptest.Server`）

```go
func TestListModels_Anthropic(t *testing.T)        // 斷言請求：GET /v1/models、x-api-key=k、anthropic-version=2023-06-01；回 {"data":[{"id":"claude-b"},{"id":"claude-a"}],"has_more":false} → IDs 依序排序 ["claude-a","claude-b"]
func TestListModels_AnthropicPaging(t *testing.T)  // 第 1 頁 has_more=true,last_id="x" → 第 2 頁請求帶 after_id=x；合併兩頁；伺服器永遠 has_more=true 時最多 10 次請求
func TestListModels_OpenAI(t *testing.T)           // Authorization: Bearer k；{"data":[{"id":"q/a","context_length":262144},{"id":"q/b"}]} → [{q/a,262144},{q/b,0}]
func TestListModels_BaseURLWithV1(t *testing.T)    // BaseURL = server.URL+"/v1" → 仍打 /v1/models（不是 /v1/v1/models）
func TestListModels_ErrorKinds(t *testing.T)       // 401→unauthorized、403→forbidden、404→not_supported、500→http(StatusCode 500)、200 非 JSON→not_supported、連不上（關閉的 server）→unreachable、ctx 逾時→timeout；errors.As 取得 *ListModelsError
func TestListModels_ErrorRedactsKey(t *testing.T)  // 伺服器 401 body 內含 "Bearer sk-secret-123" 與 "sk-secret-123" → err.Error() 不含 "sk-secret-123"
func TestListModels_UnsupportedProvider(t *testing.T) // Provider bedrock → Kind not_supported，不發請求
```

- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/api/ -run ListModels` → 編譯失敗。
- [ ] **Step 3: 實作**：base 用 `normalizeBaseURL(cfg.BaseURL)`，空值依 provider 用 `defaultBaseURL`/`defaultOpenAIBaseURL`。`Message` 取 body 前 200 bytes，並以 `strings.ReplaceAll(msg, cfg.APIKey, "[redacted]")` 遮蔽（key 非空時）。`context.DeadlineExceeded` → timeout；`net.Error`/dial 失敗 → unreachable。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/api/` → PASS。
- [ ] **Step 5: Commit**：`git add internal/api/listmodels*.go && git commit -m "feat(api): ListModels 列出 provider 模型（驗證端點與金鑰，不呼叫 LLM）"`

---

### Task 2: `config.WriteUserMainModel`

**Files:** Create `internal/config/mainmodel.go`, `internal/config/mainmodel_test.go`; Modify `internal/config/loader.go:403-440`

**Interfaces:**
- Produces:
  ```go
  type MainModelSettings struct { Provider, BaseURL, Model, APIKey string }
  func UserSettingsPath(homeDir string) string                     // <home>/.glaude/settings.json
  func WriteUserMainModel(homeDir string, s MainModelSettings) error
  // MainModelEnvVars lists every env var applyEnvOverrides reads for the
  // main model (provider, key, base URL, model), in precedence order.
  var MainModelEnvVars = []string{"CLAUDE_PROVIDER", "API_PROVIDER", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_BASE_URL", "OPENAI_BASE_URL", "OPENAI_API_BASE", "ANTHROPIC_MODEL", "CLAUDE_MODEL", "OPENAI_MODEL"}
  ```
  （實作前先 `grep -n Getenv internal/config/loader.go`，確認清單與 `applyEnvOverrides` 完全一致；`TestMainModelEnvVars_MatchesLoader` 把關。）

- [ ] **Step 1: 寫失敗測試**

```go
func TestWriteUserMainModel_CreatesFile(t *testing.T)     // home 下無 .glaude → 寫入後 LoadOrderedJSON 得 provider/baseUrl/model/apiKey 四鍵
func TestWriteUserMainModel_PreservesOtherKeys(t *testing.T) // 原檔 {"theme":"dark","permissions":{"allow":["x"]},"model":"old"} → 寫後 theme、permissions 原樣、鍵順序 theme,permissions,model,... 不變、model=新值
func TestWriteUserMainModel_EmptyKeyKeepsExisting(t *testing.T) // 原檔 apiKey="old" → APIKey "" 寫入後仍為 "old"
func TestWriteUserMainModel_Permissions0600(t *testing.T) // 原檔 0644 → 寫後 os.Stat Mode().Perm()==0600
func TestWriteUserMainModel_RefusesInvalidJSON(t *testing.T) // 原檔 "{bad" → 回 error，檔案內容不變
func TestMainModelEnvVars_MatchesLoader(t *testing.T)     // 對 MainModelEnvVars 每個變數 t.Setenv 成特定值，Load 後 Merged 的對應欄位有被覆寫（至少 provider/apiKey/baseUrl/model 各一）；並讀 loader.go 原始碼，斷言其中 Getenv("X") 的集合 == MainModelEnvVars 集合
```

- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/config/ -run 'MainModel'` → 編譯失敗。
- [ ] **Step 3: 實作**：`LoadOrderedJSON`（不存在 → 空 map）→ `Set` 四鍵（APIKey 空不設）→ `WriteOrderedJSONAtomic` → `os.Chmod(path, 0o600)`。若 `WriteOrderedJSONAtomic` 用 temp+rename，改在 rename 前 chmod 也可；確保最終 0600。`applyEnvOverrides` 不改行為，只把變數名稱改為引用同一份常數（或保留原寫法，由測試把關一致）。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/config/` → PASS。
- [ ] **Step 5: Commit**：`git commit -m "feat(config): WriteUserMainModel 寫入全域 settings.json 主腦設定（0600，保留其他欄位）"`

---

### Task 3: modeltier 與 compact 小擴充

**Files:** Modify `internal/modeltier/store.go`, `internal/compact/window.go`; Test `internal/modeltier/store_test.go`, `internal/compact/window_test.go`

**Interfaces:**
- Produces:
  ```go
  func (s *Store) BindWithLimits(tier, providerID, modelID string, contextWindow int) error // contextWindow 0 = 不寫
  func (s *Store) SetMainContextWindow(n int) error  // 0 = 刪除 [main] context_window
  func KnownContextWindow(model string) (int, bool)   // compact：內建表有此模型（claude-*）時 true
  ```
  `commitLocked` 泛化為 `commitLocked(mutate func(raw map[string]any)) error`；`Bind`/`Unbind` 改用它，行為不變（既有測試把關）。

- [ ] **Step 1: 寫失敗測試**

```go
func TestStore_BindWithLimits(t *testing.T)          // BindWithLimits("lite","main","m",32768) → 重新 Load 後 Resolve("lite").ContextWindow==32768；其他段落保留
func TestStore_SetMainContextWindow(t *testing.T)    // Set(131072) → 重新 Load MainContextWindow()==131072；Set(0) → 0 且檔案中沒有 [main] 的 context_window 鍵；其他段落保留
func TestStore_SetMainContextWindowRefusesAfterLoadError(t *testing.T) // 壞檔 → error、檔案不變
func TestKnownContextWindow(t *testing.T)           // ("claude-sonnet-5-5")→(200000,true)；("qwen")→(0,false)
```
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**。`SetMainContextWindow(0)` 刪掉 `main.context_window`；`[main]` 變空表時整段刪除。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/modeltier/ ./internal/compact/` → PASS。
- [ ] **Step 5: Commit**：`git commit -m "feat(modeltier): BindWithLimits 與 SetMainContextWindow"`

---

### Task 4: `commands` 的 `SetupBackend` 與 `/setup`

**Files:** Create `internal/commands/setup.go`, `internal/commands/setup_test.go`; Modify `internal/commands/builtins.go`（`RegisterBuiltins` 加 `cmdSetup()`）

**Interfaces:**
- Produces:
  ```go
  const (SetupFormatAnthropic = "direct"; SetupFormatOpenAI = "openai")
  // KeySource values: "" (none), "settings", or "env:<VAR>".
  type SetupDefaults struct {
      Format, BaseURL, Model string
      KeySource string
      // Tiers maps lite/pro/max to their current binding: "main:<model>",
      // "<provider>:<model>", or "" when unbound.
      Tiers map[string]string
  }
  type SetupInput struct { Format, BaseURL, APIKey string } // APIKey "" = reuse (see spec §5.7)
  type SetupModel struct { ID string; ContextWindow int }
  // SetupPlan is what Apply writes. Tiers covers only lite/pro/max entries
  // the wizard controls: model ID to bind main:<model>, "" to unbind.
  type SetupPlan struct { Input SetupInput; MainModel string; Tiers map[string]string; ContextWindows map[string]int }
  type SetupReport struct { SettingsPath, ConfigPath string; SettingsWritten, ConfigWritten bool; Warnings []string }
  type SetupBackend interface {
      Current() SetupDefaults
      ListModels(ctx context.Context, in SetupInput) ([]SetupModel, error)
      Apply(ctx context.Context, plan SetupPlan) (SetupReport, error)
      Overrides() []string
      NeedsSetup() bool
  }
  var ErrModelListUnsupported = errors.New("endpoint does not support listing models")
  ```
- [ ] **Step 1: 寫失敗測試**：`TestSetupCommand_OpensDialog`：`cmdSetup().Execute(CommandContext{}, "")` → `Result.OpenDialog == "setup"`、`Display == DisplayNone`；`TestSetupCommand_Registered`：`RegisterBuiltins` 後 `Get("setup")` 存在。
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**（Description：`"Set up the main model: endpoint, API key, model and tiers"`）。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/commands/`。
- [ ] **Step 5: Commit**：`git commit -m "feat(commands): /setup 指令與 SetupBackend 介面"`
- [ ] **Step 6: 段落驗證與審查**：`make vet && go test -race ./internal/... && gofmt -l internal/ cmd/`；開 reviewer subagent 審 Task 1–4 的 diff（重點：金鑰遮蔽、settings.json 保留欄位與權限、ListModels 錯誤分類、介面是否足以支撐 Task 5–7）。依 superpowers:receiving-code-review 處理。

---

### Task 5: TUI 設定狀態機 `setupWizard`

**Files:** Create `internal/tui/setupwizard.go`, `internal/tui/setupwizard_view.go`, `internal/tui/setupwizard_test.go`

**Interfaces:**
- Consumes: Task 4 的型別。
- Produces:
  ```go
  type setupStep int // stepFormat, stepEndpoint, stepValidating, stepModels, stepManualModels, stepRoles, stepConfirm, stepWriting, stepDone
  type setupAction int // actNone, actClose, actValidate, actApply
  func newSetupWizard(d commands.SetupDefaults, overrides []string) *setupWizard
  func (w *setupWizard) handleKey(key tea.KeyPressMsg) setupAction
  func (w *setupWizard) handlePaste(text string)
  func (w *setupWizard) input() commands.SetupInput
  func (w *setupWizard) onModels(models []commands.SetupModel, err error) // 依 err 決定回 stepEndpoint / stepManualModels / stepModels
  func (w *setupWizard) plan() commands.SetupPlan
  func (w *setupWizard) onApplied(r commands.SetupReport, err error)
  func (w *setupWizard) view(width, maxHeight int, theme Theme) string
  ```
  `setupWizard` 還有一個 `gen int`：每次發出 actValidate / actApply 時 +1，Task 6 用它丟棄過期訊息。

- [ ] **Step 1: 寫失敗測試**（只測狀態機與 view，用「詞」斷言；按鍵用 `tea.KeyPressMsg{Code: ..., Text: ...}`）

```go
func TestSetupWizard_FormatDefaultsFromCurrent(t *testing.T)   // Defaults Format "openai" → 第 1 步游標在 OpenAI 相容；enter → stepEndpoint，BaseURL 欄位預填 defaults.BaseURL
func TestSetupWizard_BaseURLDefaultPerFormat(t *testing.T)     // 無 defaults、選 Anthropic → BaseURL "https://api.anthropic.com"；選 OpenAI → "https://api.openai.com"
func TestSetupWizard_KeyMaskedAndReuseHint(t *testing.T)       // 輸入 "sk-abc" → view 不含 "sk-abc"、含 "••••••"；KeySource "settings" 且格式未換 → view 含 "leave empty"；換了格式 → 不含
func TestSetupWizard_EnterValidates(t *testing.T)              // stepEndpoint enter → 回 actValidate、step==stepValidating、input() 帶出輸入值（APIKey 空 = reuse）
func TestSetupWizard_EmptyKeyWithoutSourceBlocked(t *testing.T) // KeySource "" 且金鑰空 → enter 回 actNone、view 含 "API key is required"
func TestSetupWizard_ValidationErrorReturnsWithInputs(t *testing.T) // onModels(nil, errors.New("401 unauthorized")) → stepEndpoint、BaseURL 保留、view 含 "unauthorized"
func TestSetupWizard_UnsupportedGoesManual(t *testing.T)       // onModels(nil, fmt.Errorf("x: %w", commands.ErrModelListUnsupported)) → stepManualModels；輸入 "a, b ,c" enter → 勾選 [a b c]、進 stepRoles
func TestSetupWizard_FilterAndCheckPersist(t *testing.T)       // 模型 [q/a q/b x/c]；打 "q" → 可見 2 筆；空白勾 q/a；打 backspace 清空 → 3 筆可見且 q/a 仍勾選
func TestSetupWizard_SpaceIsToggleNotFilter(t *testing.T)      // 過濾字串 "q" 時按空白 → 過濾字串仍為 "q"
func TestSetupWizard_EnterWithNoneCheckedUsesCursor(t *testing.T) // 未勾選、游標在 x/c → enter 後主腦 = x/c（只 1 個 → 直接 stepConfirm）
func TestSetupWizard_SingleCheckedSkipsRoles(t *testing.T)     // 勾 1 個 → enter 直接 stepConfirm
func TestSetupWizard_RolesPrefillCurrent(t *testing.T)         // Defaults.Tiers {lite:"main:q/b", pro:"zai:glm", max:"main:old"}、勾 [q/a q/b] → 主腦 q/a、lite=q/b、pro 顯示 "zai:glm" 且 ◀▶ 不改變它、max=unbound 且 view 含 "main:old" 與 "will be unbound"
func TestSetupWizard_PlanTiers(t *testing.T)                   // 承上 plan(): MainModel q/a；Tiers {lite:"q/b", max:""}；不含 pro；ContextWindows 只含有值的模型
func TestSetupWizard_ConfirmShowsKeySuffixOnly(t *testing.T)   // 金鑰 "sk-abcdefgh" → confirm view 含 "efgh"、不含 "sk-abcd"；含 overrides 的每一則
func TestSetupWizard_EscWalksBackAndClosesAtStart(t *testing.T) // 每一步 esc 回上一步；stepFormat esc → actClose
func TestSetupWizard_ApplyErrorStaysOnConfirm(t *testing.T)    // onApplied(SetupReport{SettingsWritten:true}, err) → stepConfirm、view 含 "settings.json" 與錯誤
func TestSetupWizard_DoneAsksRestart(t *testing.T)             // onApplied(ok) → stepDone、view 含 "restart"；任意鍵 → actClose
func TestSetupWizard_ScrollKeepsCursorVisible(t *testing.T)    // 1000 個模型、maxHeight 20、按 down 500 次 → view 含游標所在模型 ID
```
- [ ] **Step 2: 執行，確認失敗**：`go test ./internal/tui/ -run SetupWizard` → 編譯失敗。
- [ ] **Step 3: 實作**：文字欄位用 rune slice（`key.Text` 追加、backspace 刪）；Tab / ↑↓ 切換欄位。過濾為不分大小寫子字串；勾選以 ID 為鍵的 map，並保留勾選順序（主腦預設＝第一個勾的，或現有主腦模型若在勾選內）。視窗捲動：保持 cursor 在 `[top, top+rows)` 內。兩個檔案各 ≤800 行。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/tui/ -run SetupWizard` → PASS。
- [ ] **Step 5: Commit**：`git commit -m "feat(tui): /setup 設定引導狀態機與畫面"`

---

### Task 6: TUI 接線（對話框、tea.Cmd、paste、啟動提示）

**Files:** Create `internal/tui/setup_dialog.go`, `internal/tui/setup_dialog_test.go`; Modify `internal/tui/model.go`（`dialogSetup`、`setupBackend`、`setupWizard` 欄位）, `internal/tui/keys.go:35-51`, `internal/tui/update.go:381-388,532-555`, `internal/tui/view.go:~197`, `internal/tui/init.go`, 既有 `newAppModel`/`New` 呼叫點（tests、`bootstrap/root.go`）

**Interfaces:**
- Consumes: Task 4 `SetupBackend`；Task 5 wizard。
- Produces: `tui.New`/`newAppModel` 在 `tiers` 之後新增 `setup commands.SetupBackend`；訊息 `setupModelsMsg{gen int; models []commands.SetupModel; err error}`、`setupAppliedMsg{gen int; report commands.SetupReport; err error}`。

- [ ] **Step 1: 寫失敗測試**（假 backend 記錄呼叫）

```go
func TestSetupDialog_OpenFromCommand(t *testing.T)        // applyCommandResult(Result{OpenDialog:"setup"}) → activeDialog==dialogSetup、wizard 由 backend.Current() 建立
func TestSetupDialog_NilBackend(t *testing.T)            // backend nil → 不開對話框、顯示 system 錯誤 "not available"
func TestSetupDialog_ValidateRunsAsCmd(t *testing.T)     // 走到 endpoint 按 enter → Update 回傳非 nil cmd、假 backend 尚未被呼叫；執行 cmd() → backend.ListModels 被呼叫一次並得到 setupModelsMsg
func TestSetupDialog_StaleModelsMsgIgnored(t *testing.T) // 發出 validate 後 esc 回上一步，再送入當時 gen 的 setupModelsMsg → wizard 仍在 stepEndpoint
func TestSetupDialog_PasteGoesToFocusedField(t *testing.T) // 焦點在 API key 欄位、送 tea.PasteMsg{Content:"sk-xyz"} → wizard.input().APIKey=="sk-xyz"；主輸入框內容不變
func TestSetupDialog_ApplyRunsAsCmd(t *testing.T)        // 確認頁 enter → cmd；執行 → backend.Apply 收到 wizard.plan()
func TestSetupDialog_CloseRestoresIdle(t *testing.T)     // 第 1 步 esc → activeDialog==dialogNone、setupWizard==nil
func TestStartup_NeedsSetupNotice(t *testing.T)          // backend.NeedsSetup()==true → 啟動 messages 含 "/setup"；false → 不含
```
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**：`handleSetupKey` 呼叫 `w.handleKey`，依 action 回傳 `listModelsCmd(backend, w.input(), w.gen)` / `applyCmd(backend, w.plan(), w.gen)` / 關閉；ListModels 的 cmd 用 `context.WithTimeout(context.Background(), 15*time.Second)`（spec §5.2），Apply 的 cmd 用 30 秒。`Update` 的 `tea.PasteMsg` 分支：`dialogSetup` 時交給 `w.handlePaste`。`setupModelsMsg`/`setupAppliedMsg`：`m.setupWizard == nil || msg.gen != w.gen` 就丟棄。啟動提示用 `newSystemMessage`，比照 tier 警告插在 `newAppModel` 尾端。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/tui/` → PASS。
- [ ] **Step 5: Commit**：`git commit -m "feat(tui): /setup 對話框接線與未設定時的啟動提示"`

---

### Task 7: bootstrap `setupBackend`

**Files:** Create `internal/bootstrap/setup.go`, `internal/bootstrap/setup_test.go`; Modify `internal/bootstrap/wire.go`（`AppContainer.Setup commands.SetupBackend`，`BuildContainer` 建立並設 `NeedsSetup` = `resolveAPIKey` 回空字串；`BuildContainerWithClient` 也建立，`NeedsSetup` 固定 false）, `internal/bootstrap/root.go`（`tui.New` 傳 `container.Setup`）

**Interfaces:**
- Consumes: Task 1 `api.ListModels`；Task 2 `config.WriteUserMainModel`、`MainModelEnvVars`、`UserSettingsPath`；Task 3 `BindWithLimits`、`SetMainContextWindow`、`compact.KnownContextWindow`；Task 4 介面。
- Produces:
  ```go
  type setupBackend struct {
      homeDir    string
      settings   *config.LayeredSettings
      needsSetup bool
      hasOAuth   bool
      httpClient *http.Client                // nil → default；測試注入
      getenv     func(string) string         // 測試注入
  }
  func newSetupBackend(homeDir string, settings *config.LayeredSettings, needsSetup, hasOAuth bool) *setupBackend
  ```
  `resolveAPIKey` 需要能回報「key 來自 OAuth」：新增小 helper `hasOAuthToken() bool`（同一套 token store 檢查），不改 `resolveAPIKey` 簽名。

- [ ] **Step 1: 寫失敗測試**

```go
func TestSetupBackend_CurrentFromUserLayerOnly(t *testing.T)   // User {provider openai, baseUrl u, model m, apiKey k}、Project {model pm} → Current(): Format openai、BaseURL u、Model m（不是 pm）、KeySource "settings"；Tiers 來自另外載入的 config.toml
func TestSetupBackend_CurrentEnvKeySource(t *testing.T)       // User 無 apiKey、getenv OPENAI_API_KEY="e"、Format openai → KeySource "env:OPENAI_API_KEY"
func TestSetupBackend_ListModelsReusesSettingsKey(t *testing.T) // httptest 檢查 Bearer 為 settings 的 k；SetupInput.APIKey "" 且格式未換
func TestSetupBackend_ListModelsFormatChangedNeedsKey(t *testing.T) // settings 是 direct、輸入 openai、APIKey "" 且無 env → error 含 "API key"
func TestSetupBackend_ListModelsMapsUnsupported(t *testing.T) // 伺服器 404 → errors.Is(err, commands.ErrModelListUnsupported)
func TestSetupBackend_ApplyWritesSettingsAndTiers(t *testing.T)
// 初始 config.toml: lite=main:q/b, pro=zai:glm(zai 已註冊), max=main:old；plan: Main q/a, Tiers{lite:"q/b", max:""}, ContextWindows{q/b:32768}
// → settings.json provider/baseUrl/model/apiKey 正確、權限 0600；config.toml: lite=main:q/b 且 context_window 32768、pro 不變、max 未綁定；回報 SettingsWritten/ConfigWritten true
func TestSetupBackend_ApplyMainContextWindow(t *testing.T)    // 原主腦 old、[main] context_window=1000；新主腦 qwen 有 131072 → [main]=131072；新主腦 claude-x（KnownContextWindow true）→ [main] 被清掉；主腦未變 → 不動
func TestSetupBackend_ApplyEnvKeyNotWritten(t *testing.T)     // 金鑰來自 env、輸入空 → settings.json 不含 apiKey 鍵
func TestSetupBackend_ApplyConfigErrorReportsPartial(t *testing.T) // config.toml 是壞檔、plan 有等級 → error；report.SettingsWritten true、ConfigWritten false
func TestSetupBackend_DoesNotTouchRunningStore(t *testing.T)  // 傳入執行中 Store（另一個實例）Resolve("lite") 在 Apply 後不變
func TestSetupBackend_Overrides(t *testing.T)                 // Project 設 model → 含 "project"；getenv ANTHROPIC_BASE_URL → 含 "ANTHROPIC_BASE_URL"；Policy 設 apiKey → 含 "managed"；hasOAuth → 含 "OAuth"
```
- [ ] **Step 2: 執行，確認失敗**。
- [ ] **Step 3: 實作**：
  - `Current`：`settings.User` 為 nil 時回空預設。Tiers：`modeltier.Load(DefaultPath(home))` 的 `List()` 取 lite/pro/max，格式化為 `provider:model`。
  - 金鑰解析 `keyFor(in SetupInput) (string, error)`：`in.APIKey` 非空 → 用它；否則 `in.Format == userProvider && user.APIKey != ""` → user key；否則依格式讀 `ANTHROPIC_API_KEY`/`OPENAI_API_KEY`；都沒有 → `errors.New("API key is required")`。
  - `ListModels`：`api.ListModels(ctx, api.ClientConfig{Provider: format, BaseURL, APIKey: key}, b.httpClient)`；`*api.ListModelsError` 且 Kind `not_supported` → `fmt.Errorf("%s: %w", msg, commands.ErrModelListUnsupported)`；其他錯誤訊息以 Kind 為首（例如 `"unauthorized: ..."`）。
  - `Apply`：先 `config.WriteUserMainModel`（APIKey：輸入非空才寫；沿用 settings key 時不動；只有 env key 時不寫）。再 `modeltier.Load` 一個新 Store，依 spec §5.4 對每個等級 Bind/BindWithLimits/Unbind；主腦變更時 `SetMainContextWindow`。任一步失敗即回傳已完成的部分。
  - `Overrides`：Project/Local 任一設了四欄位之一 → "project settings override …"；`MainModelEnvVars` 中有值者 → "environment variable X overrides settings.json"；Policy 設了 → "managed settings lock …"；hasOAuth → "signed in with OAuth; the API key is used only when signed out"。
- [ ] **Step 4: 執行，確認通過**：`go test -race ./internal/bootstrap/` → PASS。
- [ ] **Step 5: Commit**：`git commit -m "feat(bootstrap): /setup 後端：驗證、寫入 settings.json 與等級"`

---

### Task 8: 文件與合併前驗證

**Files:** `docs/setup.md`（新增，使用說明）、`docs/config-toml.md`（加一行指向 `/setup`）、各包 CONTEXT.md Design Notes（api、config、modeltier、compact、commands、tui、bootstrap）、`docs/project/design/{services,infra,tui,cli}.md` 變更記錄、spec 狀態

- [ ] **Step 1: `docs/setup.md`**：流程、按鍵、寫入哪些檔案與欄位、0600、重新啟動、覆蓋警告的意思、端點不支援 `/models` 時怎麼辦、重跑時等級的處理規則。
- [ ] **Step 2: `make docs`**，還原純時間戳變動的 CONTEXT.md，再補 Design Notes（記「為什麼」：不進 `api.Client` 介面、現值只讀 User 層、另外載入 Store、金鑰不經 TUI、gen 丟棄過期訊息）。
- [ ] **Step 3: 驗證**：`make all && make docs-check && make debt-check`；`gofmt -l internal/ cmd/` 無輸出；新增檔案 ≤800 行。
- [ ] **Step 4: Commit**：`git commit -m "docs: /setup 使用說明與 Design Notes"`
- [ ] **Step 5: 整個分支審查**：reviewer subagent 審 `git diff feat/provider-tiers...HEAD`（PR #4 合併後改用 `dev...HEAD`）；處理後依 superpowers:finishing-a-development-branch 收尾。
