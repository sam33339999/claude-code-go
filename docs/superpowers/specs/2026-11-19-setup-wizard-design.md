# `/setup` 首次設定引導 設計

- 日期：2026-11-19
- 狀態：已核准並實作（2026-11-19，分支 `feat/setup-wizard`）。使用說明見 `docs/setup.md`。
- 分支：`feat/setup-wizard`（從 `feat/provider-tiers` 分出，依賴其 `internal/modeltier` 與 `main:<模型>` 等級綁定；PR #4 合併後改以 `dev` 為基底）。
- 相關：`docs/superpowers/specs/2026-11-19-provider-tiers-design.md`（等級與 config.toml）、`docs/config-toml.md`。

## 1. 意圖

第一次使用 glaude、主腦還沒有任何可用模型時，使用者不該需要手寫 `settings.json` 和 `config.toml`。`/setup` 在 TUI 裡一步一步引導：輸入端點與金鑰 → 驗證可用 → 從模型清單挑模型 → 分配主腦與等級 → 寫檔 → 請使用者重新啟動。

**全程是程式邏輯，不經過 AI**（負責人要求）：驗證和列模型都只呼叫 `GET /models`，不呼叫 LLM、不花 token。

## 2. 目標 / 非目標 / 反目標

**目標**
- `/slash` 指令 `/setup`，任何時候都能執行；已經有設定時帶入現值。
- 支援兩種線格式：Anthropic Messages（`direct`）與 OpenAI 相容（`openai`）。
- 驗證端點與金鑰可用，失敗時顯示原因並可重試。
- 模型選單：打字即時過濾、空白勾選多個、enter 進下一步。
- 從勾選的模型分配主腦（必填）與 lite / pro / max（選填）。
- 寫入 `~/.glaude/settings.json`（主腦）與 `~/.glaude/config.toml`（等級）。
- 啟動時找不到任何可用認證，就提示「輸入 /setup 開始」。

**非目標**
- OAuth 登入、Bedrock / Vertex（後兩者目前是佔位實作）。
- 會話中途立即生效（負責人決定：寫完請使用者重新啟動）。
- 在 `/setup` 裡註冊第二個 provider（多 provider 仍手動編輯 `config.toml`，見 `docs/config.example.toml`）。
- 專案層或本地層設定的寫入：只寫全域 `~/.glaude/`。

**反目標（就算能跑也算失敗）**
- 覆寫或弄丟 `settings.json` / `config.toml` 裡與本次無關的欄位或段落。
- 金鑰出現在畫面（輸入時未遮蔽）、log、錯誤訊息或 `config.toml` 裡。
- 驗證或列模型時呼叫 LLM（花 token）。
- 寫完之後，因為專案層設定或環境變數蓋過，主腦實際上沒用到新設定，而使用者完全不知道。
- 網路請求卡住 TUI（`Update()` 裡做 I/O）。

## 3. 限制

- 依賴方向：TUI/commands 禁止 import bootstrap；`internal/config` 只用標準庫；`pkg/types` 零第三方依賴。
- MVU：所有網路請求與寫檔都走 `tea.Cmd`。
- 檔案 ≤800 行、函式 ≤80 行；導出符號要有 GoDoc；`fmt.Errorf("...: %w", err)`；I/O 函式第一個參數是 `context.Context`。
- 欠帳棘輪不升；改到導出符號的包要 `make docs` 再補 Design Notes；介面變更同步 `docs/project/design/<layer>.md`。
- Cache-first：`/setup` 是 slash 指令，不是工具，不進 schema；不改 prompt。

## 4. 做法比較（負責人選 A）

| 做法 | 內容 | 取捨 |
|------|------|------|
| **A. TUI 專用設定對話框** ✅ | `internal/tui` 新增多步驟對話框（state machine），網路與寫檔走 `tea.Cmd`，依賴由 bootstrap 以介面注入 | 唯一能在 `/setup` 裡做出搜尋＋多選選單的做法，和 `/resume` 選單同一套模式；TUI 程式碼最多 |
| B. CLI 子命令 `glaude setup` | 進 TUI 前用一般終端機逐行提問 | TUI 狀態最單純，但不是負責人要的 `/setup`，搜尋＋多選要自己寫 raw-mode 選單 |
| C. 重用 AskUserQuestion 對話框 | 一題一題問 | 改動最小，但不支援多選和即時過濾，做不出要求的選單——不考慮 |

## 5. 設計

### 5.1 流程（對話框步驟）

```
[1 格式] ──▶ [2 端點與金鑰] ──▶ [3 驗證中…] ──┬─ 成功 ──▶ [4 選模型] ──▶ [5 分配角色] ──▶ [6 確認] ──▶ [7 寫入中…] ──▶ [8 完成]
                  ▲                          │                                    (只勾 1 個時跳過 5)
                  └──────── 失敗：顯示原因 ◀──┘
                                             └─ 端點沒有 /models（404）──▶ [4' 手打模型 ID]
```

1. **格式**：二選一，`Anthropic` / `OpenAI 相容`。已有設定時預選目前的 provider。
2. **端點與金鑰**：兩個輸入欄位。
   - `Base URL`：預設值依格式帶入（`https://api.anthropic.com` / `https://api.openai.com`），可改。已有設定時帶入現值。
   - `API Key`：輸入時以 `•` 遮蔽。已有金鑰時欄位顯示「(已設定，留空沿用)」，留空就沿用現有金鑰。
3. **驗證**：背景呼叫 `api.ListModels`（§5.2）。畫面顯示「驗證中…」，可按 esc 取消。
   - 成功 → 進入 4。
   - 失敗 → 回到 2，上方顯示錯誤類別與訊息（連不上 / 401 認證失敗 / 403 / 逾時 / 其他 HTTP 狀態），欄位內容保留。
   - 404 或回傳格式無法解析 → 進入 4'：說明「這個端點不支援列出模型」，提供一個輸入欄位手打模型 ID（可輸入多個，以逗號分隔），之後照常進 5。
4. **選模型**（多選選單）：
   ```
   搜尋: qwen█                     (共 214 個，符合 3 個)
     [x] qwen/qwen3.8-27b          262k
   > [ ] qwen/qwen3-coder          131k
     [x] qwen/qwen-turbo
   ↑↓ 移動  空白 勾選  enter 下一步  esc 上一步
   ```
   - 打字即時過濾（不分大小寫、子字串比對）；退格刪字。
   - 空白勾選 / 取消游標所在的模型；**勾選狀態在過濾條件改變時保留**。
   - 有回傳上下文長度時顯示在右側（例如 OpenRouter 的 `context_length`）。
   - enter：至少勾一個才能前進；一個都沒勾時，把游標所在的模型當成勾選。
   - 清單依模型 ID 排序；過濾後游標回到第一筆。
5. **分配角色**（勾選 ≥2 個時）：
   ```
   已勾選 3 個模型，分配角色：
   > 主腦  qwen/qwen3.8-27b   ◀ ▶
     lite  qwen/qwen-turbo     ◀ ▶
     pro   (不綁定)            ◀ ▶
     max   (不綁定)            ◀ ▶
   ↑↓ 選角色  ◀▶ 換模型  enter 下一步  esc 上一步
   ```
   - 主腦必填，預設是勾選的第一個（若現有主腦模型在勾選清單中，預設它）。
   - lite / pro / max：**帶入現值**——目前綁在 `main:<模型>`、而且該模型有被勾選的，預設選它；其餘預設「不綁定」。可在勾選的模型與「不綁定」之間切換。
   - **「不綁定」＝解除**：原本是 `main:<模型>` 綁定、這次選「不綁定」的等級，寫入時會 Unbind；旁邊標示「原本 main:X → 將解除」。
   - **綁在其他 provider 的等級完全不動**：在角色頁顯示為「保留 zai:glm-5.3-flash」，不能切換（多 provider 仍用 `/tier` 或手動編輯）。
   - 同一個模型可以分給多個角色。
   - 負責人決定（2026-11-19）：見 §5.4。
6. **確認**：列出將寫入的內容（金鑰只顯示末 4 碼）和兩個檔案路徑，以及 §5.5 偵測到的覆蓋警告；enter 寫入、esc 上一步。
7. **寫入**：背景執行 §5.3、§5.4。任一失敗就顯示錯誤並停在確認頁（可重試）；`settings.json` 寫成功但 `config.toml` 失敗時，明確說明哪一個已寫入。
8. **完成**：「設定已寫入，請重新啟動 glaude 讓新設定生效。」附上 §5.5 的警告（若有）。任意鍵關閉。

全程 esc 回上一步；在第 1 步按 esc 關閉對話框，不寫任何東西。

### 5.2 列模型：`api.ListModels`

- 新增獨立函式，**不加進 `api.Client` 介面**（避免改動所有測試用的假 client）：
  ```go
  // ModelListing is one entry from a provider's model list.
  type ModelListing struct {
      ID            string
      ContextWindow int // 0 = provider did not report it
  }
  func ListModels(ctx context.Context, cfg ClientConfig, httpClient *http.Client) ([]ModelListing, error)
  ```
- `direct`：`GET {base}/v1/models`，標頭 `x-api-key`、`anthropic-version: 2023-06-01`；依 `has_more` / `last_id` 以 `after_id` 分頁，上限 10 頁。
- `openai`：`GET {base}/v1/models`，標頭 `Authorization: Bearer`；讀 `data[].id`，`context_length` 有值就帶入 `ContextWindow`（OpenRouter）。
- `base` 經 `normalizeBaseURL`（去掉結尾 `/v1`），與既有 client 一致。
- 逾時 15 秒（由 ctx 控制）。
- 錯誤分類回傳 `*ListModelsError{Kind, StatusCode, Message}`，`Kind` ∈ `unreachable` / `unauthorized`（401）/ `forbidden`（403）/ `not_supported`（404，或 200 但無法解析）/ `timeout` / `http`。訊息**不得包含金鑰**。
- 其他 provider（bedrock / vertex / foundry）回 `not_supported`。

### 5.3 寫入 `settings.json`

- 新增 `internal/config` 函式（只用標準庫）：
  ```go
  // MainModelSettings are the fields /setup writes to the user settings.json.
  type MainModelSettings struct { Provider, BaseURL, Model, APIKey string }
  func WriteUserMainModel(homeDir string, s MainModelSettings) error
  ```
- 以既有的 `LoadOrderedJSON` / `WriteOrderedJSONAtomic` 做 read-modify-write：只設定 `provider`、`baseUrl`、`model`、`apiKey` 四個鍵（`APIKey` 為空字串時**不動**原有的 `apiKey`），其他鍵與順序保留。
- 檔案不存在就建立（目錄 `0755`）。寫入後檔案權限設為 **`0600`**（內含金鑰）。
- 原檔 JSON 無法解析時回錯誤、**不寫檔**。

### 5.4 寫入 `config.toml`

- 依角色頁的結果處理 lite / pro / max（負責人決定：帶入現值、「不綁定」＝解除）：
  - 分配了模型 → `Bind(tier, "main", model)`。
  - 選「不綁定」、而原本是 `main:<模型>` → `Unbind(tier)`。
  - 選「不綁定」、原本就未綁定 → 不動。
  - 綁在其他 provider → 不動。
- 自訂等級（lite / pro / max 以外）一律不動。
- 寫入用 bootstrap **另外從磁碟載入**的 `modeltier.Store`，**不動執行中那一個**：等級和主腦一樣等重新啟動才生效，避免會話中途出現「舊主腦 client＋新模型 ID」的組合，`/tier` 也不會顯示尚未生效的值（我的決定）。檔案無法解析時 Bind 會拒絕，照實回報。
- 沒有任何等級要變動、也沒有 `[main]` 要寫時，不碰 `config.toml`。
- 有上下文長度資訊的模型，連同該等級寫入 `context_window`。這需要 `Store` 新增一個帶限制值的綁定方法（例如 `BindWithLimits(tier, provider, model string, contextWindow int)`），名稱與簽名由實作者決定。
- **`[main] context_window`**：主腦模型和原本不同時，清掉舊值（屬於舊模型）；新主腦模型有上下文長度資訊、而且不在內建表時，寫入新值。主腦模型沒變就不動。`Store` 需要新增設定／清除 `[main] context_window` 的方法（同樣 read-modify-write），名稱由實作者決定。

### 5.5 覆蓋偵測（反目標：寫了卻沒生效）

寫入前後都檢查，結果顯示在確認頁與完成頁：
- 專案層 `.glaude/settings.json` 或本地層 `.glaude.local/settings.json`（`LayeredSettings.Project/Local`）設了 `provider` / `baseUrl` / `model` / `apiKey` 任一個 → 警告「專案層設定會蓋過全域設定：<欄位>」。
- 環境變數 `CLAUDE_PROVIDER`、`API_PROVIDER`、`ANTHROPIC_API_KEY`、`OPENAI_API_KEY`、`ANTHROPIC_BASE_URL`、`OPENAI_BASE_URL`、`OPENAI_API_BASE`、`ANTHROPIC_MODEL`、`CLAUDE_MODEL`、`OPENAI_MODEL`（2026-11-19 `config.applyEnvOverrides` 實際讀取的清單；實作時以程式碼為準）有值 → 警告「環境變數 X 會蓋過 settings.json」。
- managed-settings（Policy）鎖定了這些欄位 → 警告。
- 有 OAuth token 時（主腦目前優先用 OAuth）→ 提示「目前使用 OAuth 登入，API key 只會在未登入時使用」。

### 5.6 啟動提示

- bootstrap 在解析主腦認證時，如果 OAuth、settings 的 `apiKey`、`ANTHROPIC_API_KEY`、`OPENAI_API_KEY` 都沒有，就在 `AppContainer` 設一個旗標（例如 `NeedsSetup bool`）。
- TUI 啟動時旗標為真 → 插入一則 TUI 本地 system 訊息：「尚未設定模型。輸入 /setup 開始設定。」（不進 engine 歷史，不影響前綴）
- 不自動開啟對話框（使用者可能只想看歷史或用其他指令）。

### 5.7 接線

- `commands`：新增 `/setup`，回傳 `Result{OpenDialog: "setup"}`；不在 commands 層做 I/O。
- `tui`：新增 `dialogSetup` 與 `setupWizard` 狀態（獨立檔案，例如 `setupwizard.go` + `setupwizard_view.go`），按鍵處理在 wizard 自己的方法裡。網路與寫檔以 `tea.Cmd` 執行，結果以訊息（例如 `setupValidatedMsg`、`setupWrittenMsg`）回到 `Update()`。
- 依賴以介面注入（TUI 不 import bootstrap）：
  ```go
  // SetupBackend is what the /setup wizard needs from the outside world.
  type SetupBackend interface {
      Current() SetupDefaults                        // 現有 provider/baseURL/model/是否已有金鑰
      ListModels(ctx context.Context, in SetupInput) ([]api.ModelListing, error)
      Apply(ctx context.Context, plan SetupPlan) (SetupReport, error)
      Overrides() []string                          // §5.5 的警告
  }
  ```
  型別放在 `commands` 包（與 `TierManager` 同一處），實作在 bootstrap（新檔，例如 `setup.go`），由 `tui.New` 注入。確切名稱與欄位由實作者決定。
- **「現值」只從全域 `~/.glaude/settings.json` 讀**（`LayeredSettings.User`），不從合併後的設定讀，避免把專案層、環境變數或 OAuth token 的值預填並寫進全域檔（我的決定）。等級現值從另外載入的 `config.toml` 讀。
- **「留空沿用」**：只沿用全域 `settings.json` 的 `apiKey`，而且格式沒換才算數；換了格式就必須輸入新金鑰。全域檔沒有金鑰、但 `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`（依格式）有值時，欄位顯示「(使用環境變數 X，留空沿用)」，驗證時用它，`apiKey` **不寫入**檔案。OAuth access token 一律不拿來驗證。
- 沿用的金鑰由 bootstrap 內部取得，**金鑰不經過 TUI 狀態**。

## 6. 測試重點

- `api.ListModels`：用 `httptest` 測兩種格式的成功、分頁、`context_length`、401/403/404/逾時/非 JSON 分類；錯誤訊息不含金鑰。
- `config.WriteUserMainModel`：保留其他鍵與順序、空金鑰不覆蓋、權限 0600、原檔壞掉時不寫、檔案不存在時建立。
- wizard（只測 `Update()` 與 view，用「詞」做斷言）：每一步的前進/返回；驗證失敗回第 2 步並保留輸入；404 走手打路徑；過濾後勾選保留；只勾 1 個跳過角色頁；一個都沒勾時 enter 用游標所在模型；確認頁金鑰只露末 4 碼；寫入失敗停在確認頁；esc 在第 1 步關閉且不寫檔。
- 覆蓋偵測：專案層、環境變數、Policy、OAuth 各一案。
- 啟動提示：無認證時有提示、有認證時沒有。
- bootstrap：「留空沿用」時使用原金鑰；`Apply` 對分配的等級呼叫 Bind 並帶入上下文長度。

## 7. 留給實作者決定

- 對話框的排版、配色、提示文字（產品既有 UI 是英文，沿用英文）。
- `SetupBackend` / `SetupDefaults` / `SetupInput` / `SetupPlan` / `SetupReport` 的確切欄位。
- 清單很長時的視窗捲動方式與顯示筆數。
- `Store` 帶限制值綁定方法的命名。

## 8. 我替負責人做的決定（核准時請確認）

1. 有回傳上下文長度時，自動寫進等級的 `context_window`；主腦的 `[main] context_window` 只在主腦模型不在內建表時才寫。
2. 啟動時只顯示提示，不自動開啟對話框。
3. 404 或回傳無法解析時，改成手打模型 ID，可輸入多個。
4. 已有金鑰時「留空沿用」，金鑰不經過 TUI 狀態。
5. 只寫全域設定；專案層、環境變數、Policy、OAuth 會蓋過時只警告，不自動修改。
6. UI 文字用英文，跟其他 slash 指令一致。
7. 寫 `config.toml` 用另外載入的 Store，等級變更等重新啟動才生效。
8. 「現值」只從全域 `settings.json` 讀；「留空沿用」只沿用全域檔的金鑰且格式不變；只有環境變數金鑰時驗證用它、不寫入檔案；OAuth token 不拿來驗證。

## 9. 負責人決定紀錄

- 2026-11-19：做法 A；金鑰寫進 `settings.json`（0600）；寫完請重新啟動；模型選單多選（空白勾選、enter 下一步）；勾選後再分配角色；重跑 `/setup` 時角色頁帶入現值、「不綁定」＝解除、其他 provider 的等級不動。
