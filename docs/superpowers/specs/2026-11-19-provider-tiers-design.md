# Provider 註冊表 + 模型等級（lite / pro / max）設計

- 日期：2026-11-19
- 狀態：**第 1 件已實作**（2026-11-19，分支 `feat/provider-tiers`，PR #4，待合併進 `dev`）。第 2、3 件**尚未開始**，見 §10。
- 決策紀錄：§0.2 的待決問題（Q1～Q3）和審閱後追問（Q4～Q7）都已在 2026-11-19 答覆，併入 §0.1 和內文。
- 範圍：三件事裡的第 1 件。第 2 件（`/model` 切換警示）和第 3 件（`/btw`）另外寫設計，見文末「後續工作」。

## 0. 交接摘要（新 session 先讀這段）

### 0.1 已決定（負責人確認過的）
- 等級：`lite` / `pro` / `max`，數量和名稱由設定檔決定。舊別名 `haiku` / `sonnet` / `opus` 對應到這三級。
- 一個等級只選定一個來源，**不做自動備援**。
- 設定檔：`~/.glaude/config.toml`，引入 `github.com/BurntSushi/toml`。
- 用 `/tier` 指令在會話中改綁定、寫回檔案，下一個派出去的 sub-agent 生效。
- 做法 C：`QueryParams.Client` 依每次查詢覆寫 client。
- 原本 `inherit` 的 agent 不異動。
- **上下文長度不該跟主腦掛鉤**：要改成依這次查詢實際使用的模型或綁定來計算。
- **全修**：下面 0.3 的盤點項目全部納入這次範圍（負責人原話「我想要全修」）。
- 後續另外做：`/model` 在對話已有內容時跳警示（會失去快取），按 yes 才切換；`/btw` 在主腦繼續跑時平行回答，繼承脈絡、只讀加 TodoWrite。
- **保留 provider id `main`**（Q1）：等級可以綁到主腦的 client 和認證（OAuth 或 settings 的金鑰），例如 `/tier lite main:claude-haiku-5-5`。見 §5.1、§5.2。
- **摘要走等級**（Q2）：帶 `params.Client` 的查詢觸發壓縮時，用這次查詢的 client 和模型做摘要。見 C4、§6。
- **`context_window` 兩處都能寫**（Q3）：等級綁定上的值優先，其次是 provider 的模型表。完整順序見 C1、§5.1。
- **`[main]` 段**（Q4，審閱後追問）：`config.toml` 加 `[main] context_window`，只管主腦的上下文長度。`CLAUDE_CODE_AUTO_COMPACT_WINDOW` 維持全域最高優先，跟現在相容。
- **`max_tokens`**（Q5）：等級和 provider 模型表都可以選填 `max_tokens`，優先順序跟 `context_window` 一樣。這個值會帶進 `QueryParams`、算進壓縮門檻，摘要的輸出上限取 `min(MaxOutputTokensForSummary, 這個值)`。
- **C3 依 agent 分開**（Q6）：主腦的壓縮狀態維持整個會話累計，每個 sub-agent 各自持有一份。
- **C8 全域改**（Q7）：主腦也改用新的估算方式。主腦因此會比現在早一點觸發壓縮，這是負責人認可的修正，算是反目標「主腦行為不變」的明文例外。

### 0.2 待決問題
- 無。Q1～Q3 是原本的待決問題；Q4～Q7 是審閱者讀完後追問的，2026-11-19 已答覆，併入 §0.1。

### 0.3 修正盤點（全修範圍，2026-11-19 查證）

| # | 問題 | 位置 | 修法方向 |
|---|------|------|----------|
| C1 | 上下文長度表只認得 `claude-3*` / `claude-opus*` / `claude-2*`，其他模型（qwen、glm、gpt、`claude-sonnet-4` 等）一律當 200k | `compact/auto.go` `resolveContextWindow` | 解析順序：`CLAUDE_CODE_AUTO_COMPACT_WINDOW`（全域覆寫，維持現在的最高優先）> 等級綁定的 `context_window` > `[providers.<id>.models."<model>"]` 的 `context_window` > 內建表（補上現行 Claude 5 系列等）> 預設值。主腦不經等級，順序是 env > `[main] context_window` > 內建表 > 預設值。TUI 狀態列的上下文用量（`compact.ContextWindow`）也要改成看得到 `[main]` 的值 |
| C2 | 壓縮門檻用 `e.model`（主腦），不是這次查詢的 `activeModel` | `engine/query.go:109,126` | 門檻改看這次查詢的模型和綁定的 `context_window` |
| C3 | 主腦和所有 sub-agent **共用一個 `AutoCompactor`**；`tracking` 沒有 mutex，sub-agent 並行時有 **data race**；熔斷（連續失敗 3 次）也是共用的，sub-agent 失敗會關掉主腦的自動壓縮 | `engine/engine.go:172`、`compact/auto.go` | **依 agent 分開**（Q6）：主腦的狀態維持整個會話累計（行為不變）；每個 sub-agent 各自持有一份，存在 `runStates`，續跑時沿用。共用的部分要加鎖；加 `-race` 測試 |
| C4 | 摘要請求固定用主腦的 client 和啟動時的模型 | `compact.NewAutoCompactor(cfg.Client, cfg.Model, …)` | 帶 `params.Client` 的查詢用這次查詢的 client 和模型摘要（Q2 已決定）；沒帶的維持主腦的 client，摘要模型照 C5 在呼叫當下解析 |
| C5 | `/model` 換主腦後，`SetModel` 只改 `e.model`，compactor 的摘要模型沒跟著換；`SetModel` 也沒加鎖 | `engine/engine.go:207` | 摘要模型改成在呼叫當下解析，或讓 `SetModel` 同步更新；並加鎖 |
| C6 | `FallbackModel` 重試時會用這次查詢的 client 搭配主腦的模型名，打到錯的端點 | `engine/query.go:~180` | 經查證，`FallbackModel` 只在 headless 模式設定（`bootstrap/run.go:65`），sub-agent 從來不帶。所以只要加一道守護：帶 `Client` 的查詢忽略 `FallbackModel`，再補一個測試 |
| C7 | 摘要時把**整段對話**送給摘要模型；如果對話已經超過摘要模型的上下文上限，摘要本身就會失敗 | `compact/auto.go` `Compact` | 至少有守護：超過摘要模型的上限時先 snip 或截斷再送；要不要做到分段摘要由實作者評估。摘要的輸出上限取 `min(MaxOutputTokensForSummary, 綁定的 max_tokens)` |
| C8 | token 估算只看訊息 JSON ÷ 4，不含 system prompt 和工具 schema，門檻會偏樂觀 | `compact/auto.go` `estimateTokens` | 門檻計算加上 system prompt 和 schema 的估算；如果 API 回傳了 usage，優先用最近一次的 `input_tokens`。**主腦也一起改**（Q7），所以主腦觸發壓縮的時間會提早 |
| C9 | **已證實**（2026-11-19，`compact/snip_prefix_test.go`）：訊息數超過 12 之後，每一輪都有一則越過保護區的訊息在前綴中段被改成 `[snipped]`，快取每輪失效 | `compact/snip.go` | 改成依 token 門檻觸發：估算值到達自動壓縮門檻的 80% 才 snip，一次剪完保護區之前的所有內容（等同一次壓縮）|
| C12 | **新發現**：snip 的 boundary 訊息每輪都追加一次（6 輪累積 6 則），而且是緊接在 tool_result 後的連續 user 訊息 | `compact/snip.go`、`engine/query.go:102` | 移除 boundary 訊息，說明文字併入 placeholder |
| C10 | `bedrock` / `vertex` 是佔位實作（沒有簽章或認證） | `api/factory.go` | 不在這次範圍；`config.toml` 的 `format` 只接受 `anthropic` / `openai` |

主功能（provider 註冊表、等級、`/tier`、`QueryParams.Client`）照 §5 做。C1～C9、C12 跟著一起修，C10 只寫文件。另外補一項：

- **C11**：sub-agent 請求的 `max_tokens` 寫死用主腦的值（`engine/query.go` `buildRequestWithModel` 用 `e.maxTokens`），小模型可能因此回 400。修法：`QueryParams` 帶綁定的 `max_tokens`，有值就用它（Q5）。

## 1. 意圖與動機

使用者手上有多個模型來源，而且同一個模型常常有好幾個供應商：

- qwen：QwenCloud 官方、OpenRouter
- glm-5.3-flash：z.ai 官方、OpenRouter
- qwen3.8-27b：好幾個不同的服務商

目標是讓多 agent 能**動態綁定、動態呼叫**這些模型：

1. 先把 provider（端點 + 金鑰 + 線格式）**註冊**進設定檔。
2. 定義**等級代稱**（`lite` / `pro` / `max`）。代稱本身不是模型名，而是像指標一樣指向「某個 provider 上的某個模型」。
3. agent 只寫等級代稱。例如派 explore 讀程式碼時用 `lite`，不再寫死 `haiku`。要換模型只改綁定，agent 的定義不用動。

## 2. 目標 / 非目標 / 反目標

**目標**
- 在 `~/.glaude/config.toml` 註冊多個 provider（線格式：Anthropic Messages、OpenAI Chat Completions）。
- 等級表：`lite` / `pro` / `max` 各自綁定一個 `provider + 模型`。
- sub-agent 依等級把請求送到對應的 provider client。
- 用 `/tier` 指令在會話中查看或改綁定，**寫回檔案**，下一個派出去的 sub-agent 立即生效。
- 舊別名相容：`haiku` → `lite`、`sonnet` → `pro`、`opus` → `max`。

**非目標**
- **自動備援／負載平衡**：一個等級只選定一個來源，失敗就失敗，不自動換下一個（負責人明確決定）。
- **主腦換 provider**：主腦仍照 `settings.json` 的 `provider` / `model` 決定。主腦中途換模型的警示是第 2 件事，不在這裡做。
- Bedrock / Vertex 支援：兩者目前是佔位實作，沒有簽章或認證，不能拿來當等級的 provider。
- 專案層、本地層的 `config.toml`：只讀全域這一份。

**反目標（就算「能跑」也算失敗）**
- 主腦或原本 `inherit` 的 agent 行為改變了，包括模型、provider、prompt 前綴。**唯一例外**是 C8：主腦觸發壓縮的時間會提早（Q7，負責人認可）。
- 等級沒有綁定時，舊有行為壞掉。例如 openai provider 下的 explore 應該照舊繼承主腦，而不是 404。
- 漏帶設定，導致請求**默默**走到錯的 provider（這就是不選做法 A 的原因）。
- 明文金鑰寫進 `config.toml`。

## 3. 限制

- **依賴方向**：TUI/commands 禁止 import bootstrap；`internal/tools/*` 子包之間禁止互相 import；`pkg/types` 零第三方依賴。
- **Cache-first 審查**（`docs/PROMPT_CACHE_AND_COST.md`）：會話內的工具集合和 schema 位元組要固定；主腦的前綴不能動。
- 新依賴：`github.com/BurntSushi/toml`（負責人已同意引入）。只能在新的 `internal/modeltier` 包裡使用，**不要**放進 `internal/config`（基礎層維持只用標準庫的慣例）。
- 欠帳棘輪：不可新增 `TODO(dep)` 或 `not yet implemented` 樁。
- 改到導出符號的包要跑 `make docs`，並在 `CONTEXT.md` 補 Design Notes。

## 4. 做法比較（負責人選 C）

| 做法 | 內容 | 取捨 |
|------|------|------|
| A. API 層路由器 | 實作一個 `api.Client` router，依請求上的路由欄位分派 | engine 幾乎不用改，但壓縮、stop hooks、memory 等各處自己組的請求都要記得帶欄位，漏帶就會默默走錯 provider |
| B. 每次派工建新 engine | 依等級為每次派工另建 engine | 隔離最徹底，但 hooks、budget、compactor、中斷控制都要重新接線，兩條建構路徑都得改，改動最大 |
| **C. 每次查詢覆寫 client** ✅ | `QueryParams.Client` 有值就用它，否則用 engine 預設的 client | 改動集中、看得清楚哪次查詢走哪裡；主腦和 inherit 的 agent 不受影響。唯一副作用見 §6 |

選 C 的原因：現在主腦和所有 sub-agent 共用同一個 engine（`buildRunAgentFn` 的 `eng`），engine 裡只有一個 client，而且只有 `engine/query.go` 的 `streamResponse` 這一處（`e.client.Stream`）真正發出請求。在這一處加一個「優先用 params 帶進來的 client」，就是最小又最明確的改法。

## 5. 設計

### 5.1 設定檔 `~/.glaude/config.toml`

```toml
# Provider 註冊：id 由使用者自訂
[providers.qwencloud]
format      = "openai"          # "openai" | "anthropic"
base_url    = "https://dashscope.aliyuncs.com/compatible-mode/v1"
api_key_env = "DASHSCOPE_API_KEY"

[providers.openrouter]
format      = "openai"
base_url    = "https://openrouter.ai/api/v1"
api_key_env = "OPENROUTER_API_KEY"

[providers.zai]
format      = "openai"
base_url    = "https://api.z.ai/api/paas/v4"
api_key_env = "ZAI_API_KEY"

# 等級綁定：provider id + 模型 ID（模型 ID 原樣傳給該端點）
[tiers.lite]
provider   = "zai"
model      = "glm-5.3-flash"
max_tokens = 8192                    # 選填；優先於 provider 模型表

[tiers.pro]
provider = "openrouter"
model    = "qwen/qwen3.8-27b"

# 選填：provider 層的模型表，目前只有 context_window 一個欄位
[providers.openrouter.models."qwen/qwen3.8-27b"]
context_window = 262144
max_tokens     = 16384

# 選填：只管主腦（主腦的 provider/model 仍由 settings.json 決定）
[main]
context_window = 131072

[tiers.max]
provider       = "main"              # 保留 id：沿用主腦的 client 與認證
model          = "claude-opus-5-5"
context_window = 200000              # 選填；優先於 provider 模型表

# 沒寫的等級 = 未綁定
```

- `format` 決定線格式，對應 `api.ProviderDirect`（anthropic）或 `api.ProviderOpenAI`（openai）。`base_url` 照常經過 `normalizeBaseURL` 去掉結尾的 `/v1`。
- **金鑰只透過 `api_key_env` 指定環境變數名稱**，不接受明文 `api_key` 欄位。環境變數不存在時，啟動照常進行，但 `/tier` 和派工時要回報清楚的錯誤。
- 等級名稱與數量由檔案決定，`[tiers.<任意名>]` 都合法。預設文件和說明只用 `lite` / `pro` / `max`。
- **`main` 是保留的 provider id**：不用（也不能）在 `[providers.main]` 註冊，它永遠指向主腦的 client，包括 OAuth 或 settings 的金鑰。`config.toml` 裡出現 `[providers.main]` 時，啟動印警告並忽略這一段（我的決定：主腦的 client 沒有可以從檔案設定的欄位；`main` 模型的 `context_window` 請寫在等級綁定上）。
- `context_window` 和 `max_tokens`（都是選填的正整數）可以寫在兩處：`[tiers.<名>]`，以及 `[providers.<id>.models."<模型 ID>"]`。兩處都有值時，等級上的值優先。完整解析順序見 §0.3 C1。
- `[main]` 只接受 `context_window`，只影響主腦的壓縮門檻和狀態列顯示，不改主腦的請求內容。我的決定：主腦的 `max_tokens` 仍然只看現有設定，不從這裡讀，避免同一個值有兩個來源。等級綁到 `main:<模型>` 時，用的是等級自己的值，不套 `[main]`。
- 檔案不存在 → 視為沒有 provider、沒有等級，所有行為和現在完全一樣。
- 檔案格式錯誤 → 啟動時印出警告、當作空設定，**不要**讓整個程式起不來。

### 5.2 新包 `internal/modeltier`

職責：讀寫 `config.toml`，並把「等級名稱或舊別名」解析成 `(providerID, modelID)`。這個包**不** import `internal/api`，client 由 bootstrap 建立。

- 型別：`Config{Providers map[string]Provider; Tiers map[string]Binding}`，以及一個並發安全的 `Store`（`sync.RWMutex`，負責讀、改綁定、存檔）。
- `Resolve(name string) (Binding, bool)`：
  - 若 `name` 是等級名，回傳綁定（未綁定時回 false）。
  - 若是舊別名 `haiku` / `sonnet` / `opus`，先轉成 `lite` / `pro` / `max` 再查。
  - 若是明確的 `providerID:modelID`（以第一個 `:` 切開，而且 `providerID` 已註冊或是 `main`），直接回傳。這讓 Agent 工具的 `model` 參數和 `CLAUDE_CODE_SUBAGENT_MODEL` 也能指定特定來源。
- `Binding` 帶 `Provider`、`Model`，以及解析好的 `ContextWindow`、`MaxTokens`（等級值 > provider 模型表 > 0；0 表示 `ContextWindow` 交給 compact 的內建表、`MaxTokens` 用 engine 預設值）。`Config` 另外提供 `MainContextWindow()`。
- `Bind(tier, providerID, modelID) error`：檢查 provider 存在（或是 `main`）後改記憶體裡的表，接著**原子寫檔**（寫入暫存檔再 rename）。
- 寫檔用 BurntSushi 的 encoder 重新輸出，**檔案裡的註解會遺失**。這是已知取捨，寫在 `/tier` 的說明裡。

### 5.3 Engine：`QueryParams.Client`

- `engine.QueryParams` 新增 `Client api.Client` 欄位：沒有值就用 `e.client`。
- `streamResponse` 改成用「這次查詢的 client」。所有會發出串流請求的路徑都要經過同一個 helper，例如 `clientFor(params)`。
- fork agent（`buildForkQuery`）**永遠不帶** `Client`，維持和主腦同一 provider，才能吃到快取。

### 5.4 Bootstrap 接線

- 啟動時：`modeltier.Load(~/.glaude/config.toml)`，再為每個 provider 用 `api.NewClient(ClientConfig{Provider: format對應, BaseURL, APIKey: os.Getenv(api_key_env)})` 建立 client。client 不會連網，啟動時全部建好，放進 `map[providerID]api.Client`。`main` 這個鍵直接放主腦的 client，不另外建立。
- `buildSubAgentQuery` 的模型解析順序**不變**：`CLAUDE_CODE_SUBAGENT_MODEL` > Agent 工具的 `model` > profile 的 `model` > 繼承。差別在於，拿到候選值之後**先**問 `modeltier.Resolve`：
  - **命中** → `params.Model = binding.Model`、`params.Client = clients[binding.Provider]`，並把 `binding.ContextWindow`、`binding.MaxTokens` 帶進 params（欄位名由實作者決定），分別給 C2 的門檻計算和 C11 的請求使用。
  - **未命中**（未綁定，或不是等級名）→ 完全照現在的 `agenttype.ResolveModelForProvider` 走，所以未綁定時 openai 下的 `haiku` 仍然退回繼承主腦。
  - **未綁定的等級名要先反向映射**（審閱時發現的問題，我的決定）：`resolveModelAliasForProvider` 只認得 `haiku` / `sonnet` / `opus`，
    `"lite"` 如果原樣傳下去，會被當成模型 ID 送出，在 openai 下回 404。所以處理規則如下：
    - 未綁定的 `lite` / `pro` / `max` 先轉回 `haiku` / `sonnet` / `opus`，再交給舊邏輯。explore 在兩種 provider 下的行為都和現在**完全一樣**，`wire_fork_test.go` 的既有斷言照樣成立。
    - 使用者自訂的等級名（例如 `foo`）只要曾出現在 `[tiers.*]` 但現在未綁定，或是 `/tier foo none` 解除過，一律**繼承主腦**（`""`）。不在等級表裡的字串視為模型 ID，照舊原樣傳下去。
    - `CLAUDE_CODE_SUBAGENT_MODEL` 也先經過 `Resolve`。若值是未綁定的等級名，套用上面兩條規則，不要在 openai 分支原樣放行。
  - 候選值是空字串或 `inherit` → 不查等級，照舊繼承。
- **續跑（resume-on-message）**：`runStates` 裡保存的 params 已經帶著當初的 `Client` 和 `Model`，續跑時沿用**派工當下**的綁定，不受之後 `/tier` 改動影響（為了快取連續性）。
- 正式（`BuildContainer`）與測試（`BuildContainerWithClient`）兩條建構路徑都要接。測試路徑要能注入假的 provider client map。

### 5.5 內建 profile 與 Agent 工具

- explore profile 的 `Model` 從 `"haiku"` 改成 `"lite"`。舊別名仍然有效，這是為了讓文件語意一致。
- 其他 profile（worker / plan / verify / guide 是空值，fork 是 `inherit`）**不動**。
- `agenttype.AgentProfile.Model` 的 GoDoc 更新可用的值：`""`、`inherit`、等級名、舊別名、完整模型 ID、`providerID:modelID`。
- Agent 工具 `model` 參數的 description（`internal/tools/agent/agent.go:111`）改成提到 `lite` / `pro` / `max`。這會改動工具 schema 的位元組，見 §7 的 Cache-first 說明。

### 5.6 `/tier` 指令

- `/tier`：列出所有等級的綁定（等級 → provider:model，未綁定的標「(未綁定，繼承主腦)」），以及已註冊的 provider 和它的金鑰環境變數是否存在。
- `/tier <等級> <providerID>:<modelID>`：改綁定並寫檔，成功時回報「下一個派出的 sub-agent 生效」。
- `/tier <等級> none`：解除綁定。
- 錯誤情況（provider 未註冊、格式錯誤）用 `DisplayError` 顯示，不寫檔。
- 接線方式：commands 包不能 import bootstrap，所以 `CommandContext` 加一個由 bootstrap 注入的介面（例如 `TierManager`，提供 `List()` 和 `Bind()`）。指令執行完不需要 TUI 再做其他狀態變更，等級表本身就是唯一的狀態來源，派工時即時讀取。
- 新增 provider 必須手動編輯 `config.toml` 並**重新啟動**。`/tier` 只在已註冊的 provider 之間綁定。

## 6. sub-agent 壓縮的修正前行為與修法（⚠️ 負責人要求特別記錄）

> 狀態：已實作（2026-11-19，分支 `feat/provider-tiers`）。Design Notes 見 `internal/engine/CONTEXT.md`、`internal/compact/CONTEXT.md`。

**修正前**：sub-agent 觸發自動壓縮時，用的是主腦的 client 和主腦啟動時的模型，不是該等級綁定的模型。原因是 `compact.NewAutoCompactor(cfg.Client, cfg.Model, …)` 在 engine 建立時就綁死了，而且整個 engine 只有一個 compactor（C3）。結果是費用和延遲算在主腦的模型上，例如主腦是 max 級，lite 級的 explore 一旦觸發壓縮，那次就按 max 計費。

**修法**（Q2：走等級）：
1. 摘要請求的 client 和模型改成依這次查詢決定：帶 `params.Client` 的用它和 `params.Model`；沒帶的用主腦的 client，模型在呼叫當下解析（C5）。
2. 摘要前先套用 C7 的守護：用摘要模型的上下文上限（同一套 C1 解析）判斷，超過就先 snip 或截斷再送。
3. 壓縮狀態（熔斷計數等）每個查詢各自持有（C3），sub-agent 壓縮失敗不影響主腦。
4. 守護測試：帶 `params.Client` 的查詢觸發壓縮時，摘要請求打到**該 client**、模型是等級模型；沒帶的打到主腦 client。
5. debug 模式下，sub-agent 觸發壓縮時記一行 log，標明用了哪個 provider 和模型。
6. `internal/engine/CONTEXT.md` 和 `internal/compact/CONTEXT.md` 的 Design Notes 寫明「修正前行為與修法」。

## 7. Cache-first 三問

- **是否移動前綴？** 主腦不移動，因為主腦不帶 `Client`，模型和 provider 不變。fork 不帶 `Client`。唯一的一次性變動是 Agent 工具 `model` 參數的 description 文字（§5.5）：從生效那個版本起，新會話的工具 schema 位元組會變一次，會話中途不會變。
- **是否增大窗口？** 否。
- **補充（C8）**：主腦的壓縮門檻變得比較保守，所以會比現在早一點壓縮，也就早一點重置前綴。壓縮本來就是唯一被認可的前綴重置，這次只是讓它更接近真正的上限，不會新增其他重置前綴的路徑。
- **是否新增呼叫？** 否。等級只改變 sub-agent 請求送到哪裡，不增加請求次數。`/tier` 是 slash 指令，不是工具，不進 schema。

## 8. 測試重點

- `modeltier`：解析（含格式錯誤、檔案不存在）、`Resolve`（等級名、舊別名、`provider:model`、未綁定）、`Bind` 原子寫檔、並發讀寫（`-race`）。
- engine：沒有 `Client` 時用預設 client；有 `Client` 時用它；§6 的壓縮守護測試；兩個 sub-agent 並行壓縮的 `-race` 測試（C3）。
- compact：C1 解析順序（env > 等級 > provider 模型表 > 內建表 > 預設值）；C9 snip 前綴穩定性測試（寫計畫前先跑）。
- modeltier：`main` 保留 id（`Resolve`、`Bind`、`[providers.main]` 出現時警告並忽略）；`context_window` / `max_tokens` 兩處的優先順序；`[main]` 段。
- engine：主腦熔斷維持整個會話累計，sub-agent 失敗不影響主腦（C3）；C6 守護；C11 帶綁定的 `max_tokens`；C8 門檻包含 system prompt、schema，以及 usage 優先。
- bootstrap：等級命中時 params 帶對的 client 和模型；未命中時行為和現在一模一樣（沿用既有的 openai `haiku` 繼承測試）；續跑沿用派工當下的綁定；fork 永遠不帶 `Client`。
- commands：`/tier` 列表、綁定、解除綁定、錯誤路徑。

## 9. 留給實作者決定

- `modeltier` 內部的檔案細節、錯誤訊息文字、`/tier` 的列表排版。
- `TierManager` 介面的確切方法簽名。
- 撰寫 `config.toml` 的範例文件放在哪裡（建議 `docs/` 下一頁，並由 AGENTS.md 的 API Provider 段連過去）。

## 10. 後續工作與進度（不在本文件範圍）

| 件 | 內容 | 狀態 |
|----|------|------|
| 1 | provider 等級＋壓縮全修（本文件） | ✅ 已實作，PR #4 待合併 |
| 2 | `/model` 切換時警示快取失效 | ⏳ 未開始，需另寫設計 |
| 3 | `/btw` 平行回答旁支問題 | ⏳ 未開始，需另寫設計 |
| — | 第 1 件之後再處理的 Minor（12 項） | ⏳ 列在 PR #4 說明的「已知、之後再處理的 Minor」 |


- **第 2 件**：`/model` 在對話已有內容時，跳出確認框說明會讓快取失效，按 yes 才切換。
- **第 3 件**：`/btw`。主腦繼續跑時平行回答，繼承對話脈絡，只能讀檔、讀對話，可以用 TodoWrite，預設用主腦模型（才吃得到快取），可以用 `--tier` 指定等級。
