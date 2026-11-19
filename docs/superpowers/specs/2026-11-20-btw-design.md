# `/btw` 平行旁支問答 設計

- 日期：2026-11-20
- 狀態：草案，待負責人核准。
- 範圍：provider-tiers 設計 §10 的第 3 件。
- 依賴：`docs/superpowers/specs/2026-11-20-model-switch-design.md` 的 `engine.MainTarget`（`--tier` 與「跟主腦同一個目標」都用它），所以在 `/model` 之後實作。

## 1. 意圖

主腦正在處理長任務時，使用者常有一個不相干、或只是想確認的小問題（「retry.go 的 backoff 上限是多少？」）。現在只能等主腦這一輪結束，或打斷它。`/btw <問題>` 讓這個問題**平行**回答：主腦繼續跑、不被打擾；答案以旁注顯示，使用者看完可以選擇交給主腦，或直接忽略。

## 2. 目標 / 非目標 / 反目標

**目標**
- `/btw <問題>`，可加 `--tier lite|pro|max`（或其他等級名）。主腦在跑時**立刻**開始，不進 slash 指令佇列；主腦閒置時也能用。
- 繼承主腦目前的脈絡（system prompt＋對話歷史），預設用主腦目前的目標，吃得到主腦的 prompt 快取。
- 只能執行唯讀工具與 TodoWrite。
- 回答以標註「btw」的旁注區塊串流顯示，**不進主腦歷史**。
- 回答完成後按 ctrl+y 把「問題＋回答」交給主腦；不按就什麼都不做。

**非目標**
- 多個 `/btw` 同時進行（同時只允許一個）。
- btw 之間的追問串（每個 `/btw` 都是獨立的一次性問答）。
- 把 btw 寫進 transcript 或在 resume 後顯示。
- 讓 btw 修改檔案、執行 shell、派 sub-agent。

**反目標（就算能跑也算失敗）**
- btw 改變主腦的歷史、前綴或正在進行的這一輪。
- btw 吃掉排給主腦的佇列訊息（見 §5.4，現有 bug）。
- btw 執行了寫入或非唯讀的工具，或因為權限詢問跳出對話框打斷使用者。
- btw 因為改了工具清單而讓自己完全吃不到快取（反而比直接問主腦更貴）。
- 按 Esc 想中斷 btw，結果中斷了主腦（或反過來）。

## 3. 限制

- MVU：btw 的串流以 `tea.Cmd` 逐段讀取 engine 的 `Msg` channel；Update 不阻塞。
- Cache-first：btw 的請求前綴（工具 schema、system、歷史）必須與主腦最新一輪的請求**位元組一致**，問題接在尾端。所以工具清單不能刪，只能在**執行時**擋。
- TUI 不 import bootstrap；`--tier` 的目標解析以注入的函式取得。
- 檔案 ≤800 行、函式 ≤80 行；導出符號 GoDoc；`make docs` 後補 Design Notes。

## 4. 做法比較（負責人選 A）

| 做法 | 內容 | 取捨 |
|------|------|------|
| **A. TUI 直接發 fork 查詢** ✅ | TUI 取主腦最新前綴快照，尾端接上問題，`engine.Query`（NoWriteBack），工具清單照送、執行時只放行唯讀＋TodoWrite；以 `tea.Cmd` 串流到 btw 區塊 | 唯一能做出「畫面上的旁注」又共用快取的做法；TUI 要多管一條串流，engine 多一個欄位 |
| B. 交給 coordinator 當 fork sub-agent | 沿用派工、續跑、進度面板 | 回答出現在任務面板而非畫面旁注；coordinator 模式沒開就不能用；交給主腦要繞路 |
| C. 另建 engine 實例 | 完全隔離 | hooks、權限、工具註冊要重接，改動最大；好處 A 都有——不考慮 |

## 5. 設計

### 5.1 觸發與解析

- `/btw` 在 TUI 的輸入處理中**先於**「查詢中 slash 指令排隊」判斷，直接處理（`internal/tui/keys.go` 的 slash 分支）。
- 語法：`/btw [--tier <名>] <問題>`；問題為空 → 顯示用法。`--tier` 的值經注入的解析函式取得 `engine.MainTarget`（等級名、舊別名、`provider:model`；未綁定或金鑰缺失 → 錯誤訊息，不送出）。
- 已有一個 btw 在進行 → 拒絕：「a /btw is already running (Esc to stop it)」。

### 5.2 脈絡與目標

- **脈絡來源**（審閱後修正）：
  - 主腦**在跑**：用這次查詢 `UseContext.CacheParams` 的快照。engine 在每次送出請求前寫快照，所以快照是「主腦目前這一個請求」的前綴，正好命中它的快取；看不到主腦正在串流中的輸出（可接受）。
  - 主腦**閒置**：不用快照（它少了最後一輪的 assistant 回覆與 tool_result，而且 `/clear`、`/compact`、resume、`/model` 之後會過期）。改用 `queryEngine.GetMessages()`，system prompt 以 `startQueryCmd` 相同的方式組（抽成共用 helper）。前綴仍能命中上一個快取斷點。
  - TUI 只在主腦查詢進行中保留那次查詢的 `UseContext` 指標，查詢結束就丟棄。
- **請求**：`messages = 快照歷史 + [user: 問題 + btw 說明]`。btw 說明是接在問題後的一段固定文字（例如「This is a side question asked while you are working; answer it briefly. You can only use read-only tools.」），只出現在尾端，不動前綴。
- **目標**：
  - 預設：快照的 client 與 model（= 主腦這一輪的目標，才吃得到快取）。
  - `--tier`：該等級的 client／model／上下文長度／max_tokens。provider 或模型不同就吃不到主腦快取——使用者明確選的，不另外警示。
- `QuerySource: "btw"`、`NoWriteBack: true`、`MaxTurns: 10`（我的決定：旁支問題不該跑成長任務）。

### 5.3 執行時工具限制

- engine 新增 `QueryParams.ToolFilter func(name string, input tools.Input) (allowed bool, reason string)`：工具清單（schema）照送，執行前呼叫；不允許時不執行，回 `tool_result` 錯誤「<name> is not available in /btw (read-only)」。
- btw 的 filter（審閱後修正）：`IsReadOnly(input) == true` **且不在排除清單**，或名稱是 `TodoWrite`（見 §5.7）。排除清單：`AskUserQuestion`（會跳對話框）、`EnterPlanMode` / `ExitPlanMode`（會改整個會話的權限模式）、`Sleep`，以及任何 coordinator / sub-agent 工具。其餘（Write、Edit、Bash、Agent、SendMessage、…）一律擋。排除清單要有守護測試，新增工具時一併檢查。
- **權限**（審閱後修正）：唯讀工具仍要經過權限判斷（不然 btw 可以不經同意讀專案外的檔案），但以**不互動、不記錄**的方式：permissions 套件新增一個檢查模式（例如 `Checker.CheckNonInteractive`），結果是 ask 時直接當成 deny、**不呼叫 askUser**，而且**不寫入**會話共用的拒絕紀錄（否則 btw 被擋的次數會讓主腦之後改成一直詢問使用者）。`QueryParams.NonInteractivePermissions bool` 讓 orchestration 改用這個模式。reason「needs permission; ask the main conversation instead」。
- **壓縮隔離**：btw 帶自己的 `CompactTracker`，並以 `QueryParams.NoCompaction bool` 關閉 snip／micro／auto 壓縮（壓縮會多一次 LLM 呼叫、改寫前綴、吃不到快取，失敗還會影響熔斷）。請求估算超過目標窗口時直接拒絕：「the conversation is too large for /btw; ask the main conversation instead」。

### 5.4 修正：非主腦查詢不得接收主腦佇列

- 現況：`runQueryLoop` 在工具執行後呼叫 `drainMidTurn`，**沒有**限定主腦；任何 NoWriteBack 查詢（sub-agent、fork、記憶抽取、btw）有工具呼叫時，都可能把使用者排給主腦的訊息搶進自己的對話。
- 修法：只有 `!params.NoWriteBack` 的查詢才 drain（sub-agent 的 follow-up 走自己的 `InboxCh`，不受影響）。補守護測試。

### 5.5 呈現（TUI）

```
┌ btw: retry.go 的 backoff 上限是多少？            (pro · openrouter:qwen/…)
│ 上限是 32 秒（internal/api/retry.go:88），每次重試加倍…
└ ctrl+y send to main · Esc dismiss
```

- btw 區塊是 TUI 本地的訊息類型（比照 system 訊息，**不進 engine 歷史、不進 transcript**），在送出時插在目前畫面的尾端，串流更新內容；主腦後續的輸出照常接在下方。
- 進行中顯示 spinner 與工具名稱（例如「Reading internal/api/retry.go」）；被擋的工具不顯示細節。
- **Esc 的分派**：btw 進行中且主腦閒置 → Esc 中斷 btw。兩者都在跑時，第一次 Esc 中斷 btw、第二次才中斷主腦（我的決定：btw 是使用者剛剛發的，比較可能是想停它）。btw 已完成時 Esc 只是關閉提示列。
- 錯誤（API 錯誤、逾時）顯示在區塊內，不影響主腦。

### 5.6 交給主腦

- btw **完成後**、提示列還在時，ctrl+y 把以下文字交給主腦：
  ```
  [btw] Q: <問題>
  A: <回答>
  ```
- 主腦在跑：以 `msgqueue.ModePrompt`（`PriorityNext`）入列，在下一個工具邊界由 `drainMidTurn` 接到歷史尾端（既有機制，不動前綴）。這則佇列項目要帶 btw 標記：**主腦這一輪如果沒有再碰到工具邊界就結束**，TUI 在處理佇列前把帶標記、尚未被取走的項目移出，改成「待附加的 btw 備註」（同下一條），不自動開新一輪（審閱後修正）。
- 主腦閒置：不自動開一輪查詢；暫存為「待附加的 btw 備註」，附加在使用者下一則訊息的前面一起送出；輸入框上方顯示「1 btw note will be attached」，可按 Esc 取消（我的決定：避免使用者只是想存起來，主腦卻立刻開始回應）。
- 交出後提示列消失，區塊標示「sent to main」。

## 6. Cache-first 三問

- **是否移動前綴？** 主腦不移動（btw 不寫回、不改主腦歷史；交給主腦的文字只接在尾端）。btw 自己的前綴與主腦最新一輪位元組一致（工具清單不刪、system 與歷史取自快照），只有尾端的問題是新的。
- **是否增大窗口？** 主腦：只有使用者按 ctrl+y 時，尾端多一則文字。
- **是否新增呼叫？** 是——每個 `/btw` 是一次（或數次，含工具往返）新的請求，這正是功能本身；預設用主腦目標以命中快取，讀取前綴的成本是快取價。

## 7. 測試重點

- engine：`ToolFilter` 擋下的工具不執行且回錯誤 tool_result、schema 仍包含它；`NoInteractivePermissions` 時 ask → deny、不送出 AskRequest；NoWriteBack 查詢不 drain 主腦佇列（守護 §5.4）；btw 請求的工具 schema 與 system 位元組與主腦相同。
- TUI：查詢進行中 `/btw` 不進佇列而立即開始；第二個 `/btw` 被拒；串流更新區塊；區塊不進 `queryEngine` 歷史；Esc 的分派順序；ctrl+y 在主腦執行中入列 ModePrompt、閒置時暫存並附加在下一則訊息、Esc 取消暫存；錯誤顯示在區塊內；`--tier` 未綁定時錯誤且不送出。
- bootstrap：`--tier` 解析函式回傳正確的 `MainTarget`；不可用目標回錯誤。

## 8. 留給實作者決定

- btw 區塊的排版、spinner、提示文字（英文）。
- `ToolFilter` 的簽名細節、`NoInteractivePermissions` 的命名。
- btw 說明文字的確切內容（只要固定、只出現在尾端）。

## 9. 負責人決定紀錄

- 2026-11-19：主腦繼續跑時平行回答；繼承脈絡；只讀＋TodoWrite；預設主腦模型以吃快取；可用 `--tier`。
- 2026-11-20：回答以旁注顯示、不進主腦歷史，完成後可選擇交給主腦（ctrl+y）；做法 A。

## 10. 我替負責人做的決定（核准時請確認）

1. 同時只允許一個 `/btw`；主腦閒置時也能用。
2. `MaxTurns: 10`。
3. 需要權限詢問的工具呼叫直接拒絕，不跳對話框。
4. 兩者都在跑時，第一次 Esc 中斷 btw、第二次才中斷主腦。
5. 主腦閒置時 ctrl+y 不自動開始查詢，而是附加在下一則訊息前面送出；可取消。
6. btw 不寫進 transcript，resume 後看不到；交給主腦的內容隨主腦歷史保存。
7. 一併修正 §5.4：非主腦查詢不再接收主腦的佇列訊息。
8. （審閱後）唯讀判斷用 `IsReadOnly` 加排除清單（AskUserQuestion、Enter/ExitPlanMode、Sleep、coordinator 工具）。
9. （審閱後）主腦閒置時用 `GetMessages()`，執行中才用快照；快照在查詢結束時丟棄。
10. （審閱後）權限檢查改用不互動、不記錄拒絕的模式；btw 關閉壓縮，脈絡太大就拒絕。
11. （審閱後）ctrl+y 交出、但主腦這一輪沒再碰到工具邊界就結束時，轉為「待附加」，不自動開新一輪。
