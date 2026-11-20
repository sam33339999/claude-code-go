# 每輪耗時（Turn Timing）設計

日期：2026-11-20　規模：小改動（短版設計）

## 目的

量出「送出一個 prompt → 這一輪完全結束（含所有工具呼叫、多次 API 往返）」的實際時間：
每輪結束時在畫面上顯示，並附加寫進 log 檔，之後可以拿來統計、比較模型/tier 的速度。

## 目標 / 非目標

- 目標：互動 TUI 每輪結束顯示耗時；每輪一筆 JSONL 記錄。
- 非目標：headless（`-p`）模式、sub-agent／coordinator 個別任務計時、統計報表指令。
- 反目標：不能影響 prompt cache（耗時行不得進 engine 訊息或 transcript）；寫檔不得卡住 Update loop。

## 做法（Approaches）

| 做法 | 取捨 | 結論 |
|---|---|---|
| A. TUI 層計時 | 只動 `internal/tui`；不碰 prompt；headless 量不到 | **採用**（使用者授權由我決定） |
| B. engine 層計時，經 `TurnComplete` 帶出 | headless 也有；要改 engine 介面與設計文件 | 不採用，代價大於需求 |
| C. 交給 hooks 掛 script | 零程式碼；沒有內建顯示 | 不符合「每輪顯示」 |

## 設計

**計時起點**：`AppModel` 新增 `turnStart time.Time`，在每個「開始查詢」處（`handleSubmit` 直送、
queue 的 `ModePrompt`／`ModeExternalPrompt`、slash command 的 `DisplayNone`）設為 `time.Now()`。
排隊中的 prompt 從**真正開始跑**算起，不含排隊等待（我的判斷：要量的是 agent 做事的時間）。
實作上收斂成一個 helper（例如 `beginQuery`），把現有重複的 `isLoading/showSpinner/spinner.Reset` 一起包進去。

**累計**：一輪內每次 API 回應（`StreamAssistantTurnMsg`／`StreamDoneMsg` 的 usage）累加 input/output tokens 與
API 請求次數；`StreamToolUse` 類訊息累加工具呼叫次數。

**終點**：三種結束都記錄，`outcome` 分別是 `done`、`error`、`interrupted`
（`StreamDoneMsg`、`StreamErrorMsg`、`doAbort`）。用實際時鐘 `time.Since(turnStart)` 計算，
不用 spinner 的 tick 累加值（tick 會漂移）。

**畫面**：`done` 時附加一則系統訊息 `✻ Worked for 1m23s`；`interrupted` 顯示 `✻ Interrupted after 12s`；
`error` 已有錯誤訊息，不另加。用現有的 `formatDuration`。這則訊息只在 `m.messages`（顯示用），
不進 engine 訊息、不進 transcript → **不移動 prompt 前綴、不增大窗口、不新增 API 呼叫**。

**Log**：`~/.glaude/turn-timings.jsonl`，全域一個檔、每輪一行，用 `cwd` 欄位區分專案
（我的判斷：跨專案統計方便，也不會在 repo 裡多出檔案）。欄位：

```json
{"start":"2026-11-20T10:00:00.123+08:00","end":"…","duration_ms":83123,
 "outcome":"done","session_id":"…","cwd":"/path","model":"claude-…",
 "api_requests":4,"tool_calls":7,"input_tokens":12034,"output_tokens":1820}
```

不記 prompt 內容（隱私；session_id 可對回 transcript）。寫檔以 `tea.Cmd` 在 Update loop 外執行（同 `persistTranscriptCmd`），
`O_APPEND|O_CREATE`；失敗只寫 debug log、不打擾使用者。寫檔邏輯放 `internal/tui` 內的小檔（如 `turntiming.go`），
路徑由建構時注入（homeDir），測試用暫存目錄。

## 留給實作者

- helper 的確切名稱與放置位置；tool_calls 對應的訊息型別。
- 測試：Update() 驅動 start→done/error/abort，斷言顯示的詞與 JSONL 欄位；用可注入的 `now func() time.Time` 讓耗時可測。
- 收尾：`make docs` 後補 `internal/tui` 的 Design Notes；`gofmt`、`make all`、`docs-check`、`debt-check`。
