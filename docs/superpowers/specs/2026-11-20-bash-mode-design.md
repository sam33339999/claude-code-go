# `!` Bash 模式設計（短版）

日期：2026-11-20 ｜ 規模：小改動

## 意圖

在輸入框開頭打 `!`，後面的內容直接當 shell 指令執行，不經過模型（對齊 Claude Code 的 bash mode）。
指令與輸出要進對話脈絡，讓模型在下一句看得到。

## 目標 / 非目標

- 目標：`!cmd` 立即執行並在畫面顯示結果；指令 + 輸出附在使用者下一句一起送給模型。
- 非目標：跑完自動觸發模型回應；互動式指令（vim、需要 TTY 的登入流程）；沙箱。
- 反目標：多打 API 呼叫、移動 prompt cache 前綴、在會話中途改動工具集合。

## 做法（已選 A）

| 做法 | 取捨 | 結果 |
|---|---|---|
| A. 暫存，跟下一句一起送（同 `/btw` 的 `btwPending`） | 只動 TUI、零額外呼叫、turn 尾部注入；未送出前退出會遺失 | **採用** |
| B. 立刻 `SetMessages` 寫進 engine 歷史 | 可存活於 resume；連續 user 訊息需逐 provider 驗證、與進行中查詢有競態 | 不採用 |
| C. 跑完自動觸發模型 | 多一次呼叫、違反「直接指令」語意 | 不採用 |

## 設計

1. **觸發**：`handleSubmit` 裡，`/btw` 之後、`# ` 之前判斷 `strings.HasPrefix(text, "!")`。指令 = 去掉 `!` 後 trim；空字串忽略。照樣進 ↑ 歷史。
2. **執行**：從 `internal/tools/shell` 導出共用函式（例如 `shell.Run(ctx, dir, command, timeout)`），`bashTool.Call` 改呼叫它，避免兩份實作。
   TUI 以 `tea.Cmd` 非同步執行，工作目錄 = session 工作目錄，逾時沿用 `DefaultBashTimeoutMs`（2 分鐘）。
   使用者自己打的指令，**不走權限決策鏈**（與 Claude Code 一致）。
3. **忙碌時**：查詢進行中也立即執行（不排隊，同 `/btw`），結果一樣暫存到下一句。
4. **取消**：指令執行中按 Esc 取消（cancel context），顯示「已取消」，不暫存。
5. **顯示**：訊息列表加一個區塊：`! <cmd>` + stdout/stderr + 非零 exit code。
6. **送進脈絡**：格式
   `<bash-input>cmd</bash-input>\n<bash-stdout>…</bash-stdout>\n<bash-stderr>…</bash-stderr>`，
   輸出超過 30,000 字元時截斷並註明。新增 `bashPending`（多次 `!` 用 `joinNotes` 累積），在 `startQueryCmd` 裡跟 `takeBTWPending` 一起接在使用者文字前面。
   輸入框上方顯示提示（仿 `btwPendingHint`）：「N 筆指令輸出會附在下一句」。`/clear` 時清掉。
7. **輸入提示**：輸入值以 `!` 開頭時，提示符號 / 邊框改色，標示 bash 模式。

## 交給實作者決定

- 區塊與提示的配色（沿用 theme 既有色票）、訊息型別放在 `messages.go` 的哪裡。
- 測試：`handleSubmit` 分派、pending 累積與附加、截斷、取消；TUI 斷言用詞不用整串。

## Cache-first 三問

不移動前綴（注入在 turn 尾部的 user 訊息）；窗口只增加使用者自己帶入的輸出（有截斷上限）；不新增呼叫。
