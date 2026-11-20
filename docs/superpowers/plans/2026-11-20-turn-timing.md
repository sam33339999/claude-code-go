# 每輪耗時（Turn Timing）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 互動 TUI 每輪結束顯示耗時，並把每輪一筆 JSONL 記錄附加到 `~/.glaude/turn-timings.jsonl`。

**Architecture:** 純 TUI 層。`AppModel` 在查詢開始時記下 `turnStart` 並歸零 `turnStats`，串流期間累加 API 請求／工具／token，
在 `StreamDoneMsg`／`StreamErrorMsg`／`doAbort` 結束；顯示一則系統訊息（只進 `m.messages`），並以 `tea.Cmd` 寫檔。

**Tech Stack:** Go 1.26、`charm.land/bubbletea/v2`、`encoding/json`。

**Spec:** `docs/superpowers/specs/2026-11-20-turn-timing-design.md`

## Global Constraints

- 只改 `internal/tui`；不改 engine、不改 `newAppModel`/`New` 的參數列。
- 耗時訊息不得進 engine 訊息或 transcript（cache-first：不移動前綴、不增大窗口、不新增呼叫）。
- 寫檔不得在 Update loop 內執行；失敗靜默略過（回傳 nil msg），不打擾使用者。
- 不記 prompt 內容。
- 顯示文字：`✻ Worked for <dur>`（done）、`✻ Interrupted after <dur>`（interrupted）；error 不另加顯示。`<dur>` 用既有 `formatDuration`。
- JSONL 欄位名固定：`start`,`end`,`duration_ms`,`outcome`,`session_id`,`cwd`,`model`,`api_requests`,`tool_calls`,`input_tokens`,`output_tokens`；`outcome` ∈ `done|error|interrupted`；時間用 RFC3339Nano。
- 檔案 ≤800 行、函式 ≤80 行、導出符號要 GoDoc；`gofmt -l` 無輸出。

## Review Focus

- `turnLogPath == ""`（測試、拿不到 home）→ 不寫檔、照常顯示。Task 2 測。
- 沒有 `turnStart`（零值）時收到 `StreamDoneMsg`（例如過期事件）→ 不顯示、不寫檔。Task 2 測。
- 連續兩輪：第二輪的統計不得帶上第一輪的數字。Task 2 測。
- `~/.glaude` 目錄不存在 → 自動建立後附加。Task 1 測。
- 中斷後 stale 的 `StreamDoneMsg` 被 `streamCh == nil` 擋掉；不重複記錄。Task 2 測（abort 後再送 Done，只有一筆）。

---

### Task 1: 記錄格式與寫檔

**Files:**
- Create: `internal/tui/turntiming.go`
- Test: `internal/tui/turntiming_test.go`

**Interfaces:**
- Produces:
  - `type turnStats struct { apiRequests, toolCalls, inputTokens, outputTokens int }`
  - `type turnRecord struct { Start, End time.Time; DurationMs int64; Outcome, SessionID, Cwd, Model string; APIRequests, ToolCalls, InputTokens, OutputTokens int }`（json tag 依 Global Constraints；`Start`/`End` 以 `time.Time` 預設 JSON 編碼即為 RFC3339Nano）
  - `func appendTurnRecord(path string, rec turnRecord) error` — `MkdirAll(dir, 0o755)`、`OpenFile(O_APPEND|O_CREATE|O_WRONLY, 0o600)`、寫一行 JSON + `\n`
  - `func writeTurnRecordCmd(path string, rec turnRecord) tea.Cmd` — `path == ""` 回傳 nil；否則回傳呼叫 `appendTurnRecord` 並忽略錯誤、回傳 nil msg 的 Cmd
  - `func defaultTurnLogPath() string` — `os.UserHomeDir()` + `.glaude/turn-timings.jsonl`；錯誤回傳 `""`

- [ ] **Step 1: 寫失敗測試** `TestAppendTurnRecord_CreatesDirAndAppends`：`path := filepath.Join(t.TempDir(), "nested", "turn-timings.jsonl")`，寫兩筆（Outcome `done`、`error`），讀回兩行，各自 `json.Unmarshal` 成 `map[string]any`，斷言第一行 `outcome=="done"`、`duration_ms==83123`、含 `start`、`session_id` 鍵，第二行 `outcome=="error"`。另加 `TestWriteTurnRecordCmd_EmptyPathIsNil`：`writeTurnRecordCmd("", turnRecord{}) == nil`。
- [ ] **Step 2: 執行確認失敗** `go test ./internal/tui/ -run 'TestAppendTurnRecord|TestWriteTurnRecordCmd'` → 編譯失敗（未定義）。
- [ ] **Step 3: 實作** 上述 Produces。
- [ ] **Step 4: 執行確認通過** 同指令 → PASS。
- [ ] **Step 5: Commit** `feat(tui): 每輪耗時記錄格式與 JSONL 寫檔`

### Task 2: 接進 AppModel 的查詢生命週期

**Files:**
- Modify: `internal/tui/model.go`（`AppModel` 新欄位）、`internal/tui/init.go`（`New` 設定 `m.turnLogPath = defaultTurnLogPath()`）、
  `internal/tui/update.go`（`StreamToolUseStartMsg`、`StreamAssistantTurnMsg`、`StreamDoneMsg`、`StreamErrorMsg`、queue 的兩個 mode、`DisplayNone`）、
  `internal/tui/keys.go`（`handleSubmit` 直送、`doAbort`）、`internal/tui/turntiming.go`
- Test: `internal/tui/turntiming_test.go`

**Interfaces:**
- Consumes: Task 1 全部。
- Produces（`turntiming.go`）：
  - `AppModel` 欄位：`turnStart time.Time`、`turnStats turnStats`、`turnLogPath string`、`now func() time.Time`（nil 時用 `time.Now`）
  - `func (m AppModel) clock() time.Time`
  - `func (m AppModel) beginQuery() AppModel` — 設 `isLoading=true`、`showSpinner=true`、`spinner.Reset()`、`turnStart=m.clock()`、`turnStats={}`；取代上述 4 個起點裡重複的三行
  - `func (m AppModel) noteTurnUsage(u *TokenUsage) AppModel` — `u != nil` 時 `apiRequests++` 並累加 Input/Output
  - `func (m AppModel) finishTurn(outcome string) (AppModel, tea.Cmd)` — `turnStart` 為零值回傳 `(m, nil)`；否則組 `turnRecord`（model 取 `m.statusBar.model`、cwd 取 `m.statusBar.cwd`、session 取 `m.sessionID`），`done`/`interrupted` 附加系統訊息（`newSystemMessage`），清空 `turnStart`，回傳 `writeTurnRecordCmd(m.turnLogPath, rec)`
- 接點：`StreamToolUseStartMsg` → `turnStats.toolCalls++`；`StreamAssistantTurnMsg` 與 `StreamDoneMsg` → `noteTurnUsage(msg.Usage)`；
  `StreamDoneMsg` 在 append 最終訊息之後呼叫 `finishTurn("done")`，Cmd 併入既有 `cmds`；`StreamErrorMsg` 在錯誤訊息之後 `finishTurn("error")`，與 persist Cmd 用 `tea.Batch`；
  `doAbort` → `finishTurn("interrupted")`，與 abortCmd 併用 `tea.Batch`。

- [ ] **Step 1: 寫失敗測試**（用 `newTestModel()`、`m.turnLogPath = filepath.Join(t.TempDir(),"t.jsonl")`、假時鐘每次呼叫前進可控；串流訊息前需設 `m.streamCh = make(chan engine.Msg)` 讓 handler 不提早 return；執行回傳的 Cmd 以觸發寫檔）：
  - `TestTurnTiming_DoneShowsAndLogs`：`beginQuery` 於 t0，送 `StreamToolUseStartMsg{ToolName:"Bash"}`、`StreamAssistantTurnMsg{Usage:&TokenUsage{Input:100,Output:20}}`、時鐘 +83s、`StreamDoneMsg{Usage:&TokenUsage{Input:50,Output:10}}`。斷言最後一則訊息文字含詞 `Worked`、`1m23s`；JSONL 一行：`outcome=="done"`、`duration_ms==83000`、`api_requests==2`、`tool_calls==1`、`input_tokens==150`、`output_tokens==30`、`model=="test-model"`。
  - `TestTurnTiming_ErrorLogsWithoutExtraLine`：送 `StreamErrorMsg`；訊息裡沒有 `Worked`；JSONL `outcome=="error"`。
  - `TestTurnTiming_AbortThenStaleDone`：`doAbort` 後顯示含 `Interrupted`；再送 `StreamDoneMsg` → JSONL 仍只有一行 `interrupted`。
  - `TestTurnTiming_SecondTurnStartsFresh`：兩輪各一個 tool，第二筆 `tool_calls==1`。
  - `TestTurnTiming_NoStartNoRecord`：未 `beginQuery` 直接送 `StreamDoneMsg`（設 streamCh）→ 無 `Worked`、檔案不存在。
  - `TestTurnTiming_EmptyPathStillShows`：`turnLogPath=""` → 有 `Worked`、回傳 Cmd 執行不 panic。
- [ ] **Step 2: 執行確認失敗** `go test ./internal/tui/ -run TestTurnTiming` → FAIL。
- [ ] **Step 3: 實作** Produces 與接點；把 4 個起點改用 `beginQuery()`。
- [ ] **Step 4: 執行確認通過** `go test -race ./internal/tui/` → PASS（整包，確保沒弄壞既有測試）。
- [ ] **Step 5: Commit** `feat(tui): 每輪結束顯示耗時並寫入 turn-timings.jsonl`

### Task 3: 文件與全倉檢查

**Files:**
- Modify: `internal/tui/CONTEXT.md`（`make docs` 生成後補 `## Design Notes`）、`docs/project/design/` 中 TUI layer 文件（若有描述查詢生命週期的段落則補一行）

- [ ] **Step 1:** `make docs`，再在 `internal/tui/CONTEXT.md` 的 Design Notes 補：為何在 TUI 層計時、為何用時鐘不用 spinner tick、為何耗時訊息不進 transcript（cache-first）、log 路徑選擇。
- [ ] **Step 2:** `gofmt -l internal/ cmd/` 無輸出；`make all`、`make docs-check`、`make debt-check` 全過（`TestAdapterStopAgent` flake 單獨重跑確認）。
- [ ] **Step 3: Commit** `docs(tui): 每輪耗時的 Design Notes`
