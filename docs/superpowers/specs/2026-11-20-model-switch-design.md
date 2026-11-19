# `/model` 選單切換主腦（等級＋快取警示）設計

- 日期：2026-11-20
- 狀態：草案，待負責人核准。
- 範圍：provider-tiers 設計 §10 的第 2 件（原本只寫「切換時警示快取失效」，討論後擴大為「讓 `/model` 真正能切換」）。第 3 件 `/btw` 另寫設計。
- 相關：`docs/superpowers/specs/2026-11-19-provider-tiers-design.md`、`docs/config-toml.md`、`docs/PROMPT_CACHE_AND_COST.md`。

## 1. 意圖與現況

負責人回報「`/model` 根本沒辦法切換不同模型」。查證（2026-11-20）的現況：

- `/model <ID>` 會呼叫 `engine.SetModel`，同一個 provider 上打對完整 ID 確實會換；但沒有清單、不檢查 ID、打錯要等送出才 404。
- 別名不認得：`/model opus` 會把字面 `"opus"` 原樣送給 API。
- 主腦 client 啟動時綁死 `settings.json` 的 provider / baseUrl / 金鑰，**無法切到別的 provider**（打了 qwen 的 ID 也還是送到原端點）。等級可以綁不同 provider，主腦不行。
- 切換時沒有任何快取失效的警示。

目標：`/model` 打開選單，從 `default`（settings.json 的主腦）與 lite / pro / max 等級中選一個，主腦就換成該等級的 **provider client、模型、上下文長度與輸出上限**；對話已有內容時先警示快取會失效。

## 2. 目標 / 非目標 / 反目標

**目標**
- `/model`（不帶參數）開啟選單：`default` 一列加上每個等級一列，顯示 `provider:model` 與上下文長度；未綁定的等級顯示為灰色、不可選。
- 選擇後主腦切換到該目標；所有「跟著主腦」的查詢一起換（見 §5.2）。
- 對話已有內容時，切換前跳確認框說明快取失效，按 yes 才換。
- `/model <參考>` 打字用法保留，並與選單共用同一套解析：等級名、舊別名（haiku/sonnet/opus → lite/pro/max）、`providerID:modelID`、完整模型 ID（在目前主腦的 provider 上）。
- 狀態列顯示新模型與新的上下文長度。

**非目標**
- **寫回 `settings.json`**：負責人決定**只在本 session 有效**，重開回到 `settings.json`（主腦預設由 `/setup` 或手動設定決定）。
- 在 `/model` 裡新增或編輯 provider、等級（那是 `config.toml` / `/tier` / `/setup` 的事）。
- 從 provider 列出所有模型讓人挑（`/setup` 已有；`/model` 只在既有目標間切換）。

**反目標**
- 沒有選擇切換時，主腦的請求位元組改變（golden 測試 `TestQuery_MainLoopRequestUnchangedWithoutTiers` 必須維持）。
- 切換後只有主對話換了，繼承主腦的 sub-agent、記憶抽取等背景查詢卻還在打舊 provider（不一致）。
- 查詢進行中切換，導致同一輪對話前後用了不同的模型或 provider。
- 未警示就讓使用者付出整段前綴重算的成本。
- 金鑰缺失的等級被選中後默默退回舊 client。

## 3. 限制

- 依賴方向：TUI/commands 不得 import bootstrap；切換能力以介面注入。
- MVU：切換本身不做 I/O（client 在啟動時就建好），可以在 Update 內同步完成；確認框是一般的對話框狀態。
- Cache-first：切換主腦**本來就會**讓前綴快取失效（模型或 provider 變了），這是使用者明確選擇的結果；不新增其他前綴變動路徑，不新增 LLM 呼叫。
- 檔案 ≤800 行、函式 ≤80 行；導出符號 GoDoc；改到導出符號的包 `make docs` 後補 Design Notes。

## 4. 做法比較（負責人選 A）

| 做法 | 內容 | 取捨 |
|------|------|------|
| **A. engine 新增「換主腦目標」** ✅ | `SetMainTarget(client, model, contextWindow, maxTokens)`（mutex 保護），取代只換模型名的 `SetModel` | 主腦、繼承主腦的 sub-agent、背景查詢都讀 engine 預設值，一致跟著換；壓縮門檻與摘要自動改用新目標。代價是預設 client 從固定變成可變，要補並發測試 |
| B. TUI 每次在 `QueryParams` 帶 Client/Model | engine 不改 | 只有主對話換了；繼承主腦的 sub-agent、fork 快照、記憶抽取仍走舊 client——不符合「切換主腦」的預期 |

## 5. 設計

### 5.1 解析：`ModelSwitcher`

- 新介面放在 `commands`（與 `TierManager`、`SetupBackend` 同處），bootstrap 實作：
  ```go
  // ModelOption is one /model menu entry.
  type ModelOption struct {
      Name          string // "default" | tier name
      Provider      string // provider id, "main" for the startup client
      Model         string
      ContextWindow int
      Available     bool   // false: unbound tier or missing key
      Reason        string // why not available (e.g. "unbound", "ZAI_API_KEY not set")
      Current       bool   // the main loop is on this target now
  }
  type ModelSwitcher interface {
      Options() []ModelOption
      // Switch resolves ref (option name, legacy alias, providerID:modelID,
      // or a plain model ID on the current provider) and retargets the main
      // loop. It returns the applied option.
      Switch(ref string) (ModelOption, error)
  }
  ```
- `default` = 啟動時的主腦（settings.json 的 client、模型、`[main] context_window`、引擎預設 max_tokens）。
- 解析順序：`default` → 等級名／舊別名（`modeltier.Resolve`）→ `providerID:modelID`（provider 已註冊或 `main`）→ 其餘視為**目前主腦 provider 上的**模型 ID（保留現在 `/model <ID>` 的行為）。
- 等級綁定的 `context_window` / `max_tokens` 依 provider-tiers 的既有解析順序；`main:<模型>` 用主腦的 client。
- 等級的 provider 沒有 client（金鑰缺失、或啟動後才加入）→ `Available=false`，`Switch` 回錯誤點名原因，**不切換**。
- 每次 `Options()` 即時讀執行中的 `modeltier.Store`，所以 `/tier` 改綁定後選單立刻反映。

### 5.2 Engine：`SetMainTarget`

- 新增：
  ```go
  // MainTarget is what the main loop (and every query without its own
  // Client/Model) runs on.
  type MainTarget struct { Client api.Client; Model string; ContextWindow, MaxTokens int }
  SetMainTarget(t MainTarget)   // QueryEngine 介面新增
  ```
  以既有的 `modelMu`（改名為 `mainMu` 亦可）保護 client、model、mainContextWindow、maxTokens 四個值；`clientFor`、`activeModelFor`、`targetFor`、`buildRequestWithModel` 改讀快照。`SetModel(m)` 保留為「只換模型」的便捷方法（內部呼叫 SetMainTarget）。
- 跟著主腦一起換的：主對話、`Client` 為 nil 且 `Model` 為空的 sub-agent（inherit）、fork（快照取 activeModel，provider 跟 engine 預設 client 走）、記憶抽取／session memory 等背景 `Query`。
- 不受影響的：等級路由的 sub-agent（自帶 `Client`）；已派出、正在續跑的 sub-agent（`runStates` 保存派工當下的 params）。
- **一輪查詢內不換**：`runQueryLoop` 在查詢開始時取一次目標快照，整個 loop 都用它（修正前每輪重讀 `e.model`）。
- bootstrap 同步更新 `AppState.MainLoopModel`（`ModelID` 與 `Provider` 的線格式），讓 `buildRunAgentFn` 依新主腦解析別名（例如切到 openai 格式的 zai 後，inherit 規則要用 openai 的別名邏輯）。

### 5.3 TUI

- `/model` 不帶參數 → `Result{OpenDialog: "model"}` → 新的 `dialogModelPicker`（比照 `/resume` 選單：↑/↓、Enter、Esc），列出 `Options()`；不可選的列以灰色顯示原因，游標跳過。
- `/model <參考>` → 直接走 `Switch(ref)`，結果或錯誤以 system 訊息顯示。
- **查詢進行中**（`isLoading`）：拒絕切換並提示「先等這一輪結束或按 Esc 中斷」。
- **快取警示**：引擎歷史非空、且目標與目前不同時，選擇後先開確認框：
  ```
  Switch main model to pro (openrouter:qwen/qwen3.8-27b)?
  The prompt cache is tied to the current model and provider: the next
  request re-sends the whole conversation (≈<估算> tokens) without cache.
  y switch · n cancel
  ```
  估算值用目前歷史的 token 估算（`compact.EstimateTokens`，不含 overhead 也可，標示「≈」）。歷史為空或選的就是目前目標 → 不警示（選目前目標 = no-op，提示「already on …」）。
- 成功後：system 訊息「Main model: pro → openrouter:qwen/qwen3.8-27b (session only)」；狀態列更新 model 與 `contextWindow`。
- `commands` 層不得 import bootstrap；`ModelSwitcher` 由 `tui.New` 注入（nil → 退回現在的 `SetModel` 行為）。

## 6. Cache-first 三問

- **是否移動前綴？** 只有使用者在確認框按 yes 時才會，而且是預期中的（換模型或 provider 必然失效）。不切換時主腦請求位元組不變（golden 測試）。工具 schema 不變（`/model` 是 slash 指令）。
- **是否增大窗口？** 否。（切到更大窗口的模型時，門檻依新目標計算，屬正確行為。）
- **是否新增呼叫？** 否。

## 7. 測試重點

- engine：`SetMainTarget` 後主對話與 inherit sub-agent 都打新 client；帶 `Client` 的 sub-agent 不受影響；一輪查詢中途 `SetMainTarget` 不改變該輪後續請求的 client/model；並發 `SetMainTarget` 與 `Query` 在 `-race` 下無報告；壓縮門檻用新目標的窗口。
- bootstrap：`Options()` 反映 `/tier` 改綁定、未綁定與金鑰缺失顯示為不可用；`Switch` 的四種解析路徑；不可用目標回錯誤且不切換；`default` 換回啟動時的 client；切換後 `AppState.MainLoopModel` 的 ID 與 Provider 正確。
- TUI：選單開啟與按鍵；不可選列被跳過；有歷史時出現確認框、n 取消不切換、y 切換；無歷史時直接切換；查詢中拒絕；狀態列更新；`/model <參考>` 的成功與錯誤訊息；`ModelSwitcher` 為 nil 時維持舊行為。

## 8. 留給實作者決定

- 選單與確認框的排版、文字（英文，與其他 slash 指令一致）。
- `ModelOption` 的確切欄位、`MainTarget` 是否沿用既有欄位名。
- token 估算是否加上 system prompt 與 schema。

## 9. 負責人決定紀錄

- 2026-11-20：`/model` 改為從 default ＋ lite/pro/max 選單切換，跨 provider 也可以；只在 session 有效；做法 A（engine 換主腦目標）。

## 10. 我替負責人做的決定（核准時請確認）

1. 保留 `/model <參考>` 打字用法，並與選單共用解析；純模型 ID 視為目前主腦 provider 上的模型（維持現行行為）。
2. 查詢進行中拒絕切換，不排隊。
3. 一輪查詢開始時取目標快照，整輪不換。
4. 選目前已在用的目標 = no-op，不警示。
5. 等級金鑰缺失時選單顯示不可用，不退回舊 client。
6. 確認框顯示的 token 數是估算值。
