# 貼上收合與剪貼簿圖片設計

日期：2026-11-20 ｜ 規模：專案（兩個功能、共用一套機制，分兩個 commit）

## 意圖

讓 glaude 的輸入體驗對齊 Claude Code：

1. **貼上大量文字**時，輸入框與對話紀錄只顯示收合標記 `[Pasted text #1 +312 lines]`，模型收到全文。
   現況：`renderUserMessage`（`messagelist.go:286`）會把整段貼上內容逐行渲染到對話紀錄，貼幾千行就洗幾千行、拖慢重繪。
2. **Ctrl+V 貼剪貼簿圖片**，以 `[Image #1]` 標記插入輸入框，送出時變成 image block 給模型。
   現況：沒有剪貼簿圖片整合；終端機的 bracketed paste 只帶文字。

## 目標 / 非目標 / 反目標

- 目標
  - 大段文字（Cmd+V 的 bracketed paste、Ctrl+V 讀到的剪貼簿文字）收合成標記。
  - Ctrl+V 讀剪貼簿圖片，支援 **macOS 與 Linux**（Wayland、X11）。
  - 標記可以整顆刪除（Backspace／Delete／刪詞／vim `x`）。
  - 查詢進行中送出（進佇列）、↑ 叫回歷史紀錄後再送出，標記一樣能還原成內容。
- 非目標
  - Windows 剪貼簿：按 Ctrl+V 只提示「此平台尚不支援貼上圖片」，文字照常貼上。
  - 貼上圖片**檔案路徑**（從 Finder 拖曳）自動轉附件：之後另做。
  - 在 Finder 對檔案按 Cmd+C 後的檔案 URL：不處理。
  - `/resume` 之後對話紀錄仍顯示收合標記：resume 是從 engine 歷史（已展開的全文）重建畫面，這次不改。
  - 圖片縮圖預覽。
- 反目標
  - 移動 prompt cache 前綴、在會話中途變動工具集合、多打 API 呼叫。
  - 使用者自己打的字（例如手打 `[Image #9]`）被誤當成標記而遭到吞掉或展開。
  - 圖片超過 API 上限，導致 400 錯誤被 engine 誤判成「模型不支援視覺」，進而關掉整個 session 的圖片功能。

## 做法（已選 A，並加上標記整顆刪除）

| 做法 | 取捨 | 結果 |
|---|---|---|
| A. 標記是輸入框裡的普通文字，原文和圖片存在旁邊的暫存區，送出時展開 | 不改 textarea，與 Claude Code 及 `takeBashPending` 模式一致；標記被改壞就退化成字面文字 | **採用** |
| B. 輸入框外另開附件列 | 失去「圖對應哪句話」的位置資訊；刪除附件需要另外設計焦點與快捷鍵，會碰到 vim 模式與補全選單 | 不採用 |
| C. 改寫或包裝 textarea，讓標記成為原子 token | 體驗最好，但要 fork bubbles，維護成本高 | 不採用 |

A 的弱點（標記無法整顆刪）用「刪除鍵攔截」補上：這部分不需要接管游標移動。

## 設計

### 1. 暫存區（`pasteStore`，放在 `internal/tui`）

- `AppModel` 新增 `pastes *pasteStore`。內容是 `map[int]pastedItem` 加一個單調遞增的 `nextID`，文字和圖片**共用同一個編號序列**（#1、#2…）。
- `pastedItem{kind: text|image, text string, image *types.ImageSource, lines int}`。
- **存活期 = 整個 session**，送出後不清除。這樣查詢進行中送出的佇列訊息（`msgqueue` 只存文字）、↑ 叫回的歷史紀錄，裡面的標記都還能解析。`/clear` 也不清，編號不重複使用。
  - 記憶體代價：圖片每張約 1～3 MB（縮圖後），一般 session 可以接受。
- 標記格式（集中在一處產生和解析）：
  - 文字，多行：`[Pasted text #N +L lines]`，L = 換行數。
  - 文字，單行：`[Pasted text #N C chars]`。
  - 圖片：`[Image #N]`。
- **只有編號存在於暫存區、而且類型與格式完全相符的標記才算數**，其餘一律視為普通文字（對應反目標第二項）。

### 2. 貼上文字的收合

- 門檻：**超過 800 字元或超過 10 行**就收合（我的決定，參考 Claude Code 的 800 字元門檻），未超過照常插入原文。
- 入口有兩個，都走同一個函式 `m.insertPaste(text string)`：
  - `tea.PasteMsg`（`update.go:393`，Cmd+V 與終端機的 bracketed paste）。setup wizard 與 dialog 的分支維持原樣，不收合。
  - Ctrl+V 讀到的是剪貼簿文字時（見第 3 節）。
- 插入標記使用 textarea 的 `InsertString`，插在游標位置。
- 貼上內容先把 `\r\n` 正規化成 `\n` 再計算行數。

### 3. Ctrl+V 讀剪貼簿

- 在 `handleIdleKey` 中、交給 textarea **之前**攔截 `ctrl+v`。不能讓 textarea 處理，因為 bubbles textarea 內建的 `KeyMap.Paste` 會用 `atotto/clipboard` 讀文字後直接插入，完全繞過收合。改用 `KeyMap.Paste.SetEnabled(false)` 停用，或在上層吃掉按鍵，由實作者決定。
- 回傳 `tea.Cmd`（剪貼簿讀取是 I/O，不可以在 `Update` 裡同步執行），結果訊息：
  - `clipboardImageMsg{data []byte, mediaType string}` → 縮圖（第 5 節）→ 存入暫存區 → 插入 `[Image #N]`。
  - 剪貼簿沒有圖片 → 讀文字 → 走 `insertPaste`。
  - 錯誤或缺少工具 → 在輸入框上方顯示一行短提示（仿 `btwPendingHint` 的位置），幾秒後或下次按鍵時消失；不寫進對話紀錄。
- 讀取期間可以繼續打字；結果回來時插在**當下**游標位置。
- 新套件 `internal/clipboard`（Infra 層，只依賴標準函式庫與外部指令），以 build tag 分平台：
  - `ReadImage(ctx) (data []byte, mediaType string, err error)`，沒有圖片時回傳 `ErrNoImage`。
  - `ReadText(ctx) (string, error)`。
  - **macOS**：用 `osascript` 把 `the clipboard as «class PNGf»` 寫到暫存檔再讀回來，讀完刪除暫存檔。剪貼簿不含圖片時 osascript 會報錯，轉成 `ErrNoImage`。文字用 `pbpaste`。
  - **Linux**：有 `WAYLAND_DISPLAY` 時用 `wl-paste --list-types` 判斷是否有 `image/png` 等型別，再用 `wl-paste --type <type>` 讀取；否則用 `xclip -selection clipboard -t TARGETS -o` 判斷，再用 `-t <type> -o` 讀取。文字用 `wl-paste --no-newline` 或 `xclip -selection clipboard -o`。兩者都沒有時，回傳的錯誤要明確寫出應該安裝什麼（例如「請安裝 wl-clipboard 或 xclip」）。
  - **其他平台**：回傳 `ErrUnsupported`。
  - 每次外部指令設 5 秒逾時。
- 媒體型別依內容判斷（`http.DetectContentType`），不採信剪貼簿宣稱的型別。只接受 PNG、JPEG、GIF、WebP；其他型別（例如 TIFF）先解碼再轉成 PNG，解不了就提示「不支援的圖片格式」。

### 4. 標記整顆刪除

在 `InputModel` 新增 `DeleteToken(dir, isToken func(string) bool) (InputModel, bool)`，或同等的方法；由 `AppModel` 傳入「這段文字是否為暫存區內的有效標記」的判斷。依目前列文字與 `Line()`、`Column()` 找出游標所在或緊鄰的標記：

| 按鍵 | 條件 | 行為 |
|---|---|---|
| Backspace | 游標緊接在有效標記右側，或落在標記內部 | 刪掉整個標記 |
| Delete | 游標緊接在有效標記左側，或落在標記內部 | 刪掉整個標記 |
| Ctrl+W／Alt+Backspace／Ctrl+Backspace | 往回刪詞的範圍會碰到有效標記 | 只刪那個標記 |
| vim `x` | 游標落在有效標記上 | 刪掉整個標記 |
| 其他按鍵（含 vim `dd`） | — | 照常處理 |

- 刪掉標記**不會**從暫存區移除該項目，因為歷史紀錄可能還引用它。被刪的標記只是不再出現在文字裡，送出時自然不會帶上。
- 游標在標記中間時打字：不攔截。標記被改壞後就是普通文字。

### 5. 圖片尺寸

- 剪貼簿圖片長邊大於 **1568 px** 時，等比例縮到 1568 px（Anthropic 也會在伺服器端縮到這個尺寸，所以先縮不損失畫質，又能大幅減少傳輸量與 session 檔大小）。
- 縮圖後的編碼結果仍超過 `3.75 MB`（與 `fileops.maxImageBytes` 相同）時，改用品質 85 的 JPEG 編碼；還是超過就拒絕，並提示「圖片太大」。
- 縮圖演算法：只用標準函式庫自己寫，或加入 `golang.org/x/image/draw`，由實作者決定，但**不可以使用 CGO**。
- **Session 圖片總量上限（使用者已決定）**：
  - 按 Ctrl+V 時，先數 engine 歷史（`queryEngine.GetMessages()`）裡的 image block，最上層與 tool_result 內的都要算，再加上輸入框裡還沒送出、仍有效的圖片標記。
  - 總數已達 **20 張**時，拒絕這次貼上，並提示「這個對話已有 20 張圖片，請先 /compact 再貼」。
  - 原因：圖片會在之後每一輪重送，超過 Anthropic 單次請求的上限後，每一輪都會回 400。在 engine 端剝掉舊圖雖然能讓使用者一直貼，但每次都會改動 prompt cache 前綴，違反 cache-first 紅線。
  - `/compact` 之後歷史縮短，計數自然歸零。

### 6. 送出與展開

- `handleSubmit`、↑ 歷史紀錄、`msgqueue` **一律存放未展開、含標記的文字**。畫面上的 user message（`newUserMessage(text)`）也直接使用含標記的文字，所以對話紀錄自動顯示收合後的樣子，`renderUserMessage` 不用改。
- **只有使用者在輸入框打的文字才會展開**（我的決定，審查後補上）。`startQueryCmd` 加一個參數（或改成傳一個 struct），用來區分「使用者輸入」和其他來源。下面這兩個呼叫點**不展開**：
  - `applyDisplayResult` 的 `DisplayNone`（`update.go:691`）：prompt 型 slash command 產生的文字。
  - `processQueueCmd` 的 `ModeExternalPrompt`（`update.go:518`）：MCP 通知。
  
  使用者的 `ModePrompt` 佇列訊息照常展開。
- `handleSubmit` 在分流之前就處理的特殊輸入：

  | 輸入 | 文字標記 | 圖片標記 |
  |---|---|---|
  | `/btw …` | 展開成原文 | 附成 image block（`/btw` 本來就是問模型） |
  | `! …` | 展開成原文（才是真正要跑的指令） | 保留字面文字，並提示「圖片只能附在一般訊息或 /btw」 |
  | `# …`（quick memory） | 展開成原文 | 同上 |
  | 其他 slash command 的參數 | 展開成原文 | 同上 |

- 一般訊息在 `startQueryCmd` 組 `userMsg` 時展開，而且要在 `takeBTWPending`／`takeBashPending` **之前**，只處理使用者自己的文字。這樣 `!` 指令輸出裡剛好出現的 `[Image #1]` 字樣不會被展開。
  - 文字標記：換成原文。
  - 圖片標記：文字裡**保留** `[Image #N]` 字樣（讓模型知道文中指的是哪張圖），對應的 image block 依標記出現順序放在 text block **前面**。同一個標記出現兩次只附一張圖。
  - 結果：`Content = [image…, text]`；沒有圖片時與現在完全相同（只有一個 text block）。
- 由 `processQueueCmd` 送出的佇列訊息也會經過 `startQueryCmd`，所以會一樣被展開。實作者要確認這條路徑。
- 模型不支援視覺：既有的 engine 降級機制（`tool_images.go` 的 `hasImages`／`stripImages`）已經涵蓋最上層的 image block，不需要另外處理，但要補一個測試確認。
- Session 持久化：engine 歷史存的是展開後的訊息，圖片以 base64 寫進 session jsonl。這是已知、接受的代價。

### 7. Prompt cache（PR 說明要回答的三個問題）

- 是否移動前綴？否。只在當回合的 user message（turn 尾端）加入內容，system prompt 與工具 schema 不變。
- 是否增大窗口？只有使用者貼上時才會增加，增加量就是貼上的內容本身。圖片已經縮到 1568 px，每張約 1,600 tokens。
- 是否新增呼叫？否。

## 交給實作者決定

- `pasteStore` 與標記解析的檔案拆分（注意 `keys.go`、`update.go` 的檔案長度上限是 800 行，`handleIdleKey` 已經偏長）。
- 停用 textarea 內建 Ctrl+V 的具體方式。
- 縮圖要用標準函式庫還是 `x/image`。
- 錯誤提示的確切文字與消失時機。
- 測試切法：`pasteStore` 與標記解析、展開邏輯、`DeleteToken` 可以用純函式測試；`internal/clipboard` 的指令組裝用可注入的 command runner 測試，不呼叫真的剪貼簿。

## 交付順序

1. **Commit 1：文字收合**。包含 `pasteStore`、標記、`PasteMsg` 收合、整顆刪除、送出時展開。不碰任何外部指令。
2. **Commit 2：剪貼簿圖片**。包含 `internal/clipboard`、Ctrl+V 攔截（剪貼簿是文字時接回 commit 1 的收合）、縮圖、image block。
3. 兩個 commit 都要跑 `make docs`、補 `## Design Notes`（新套件 `internal/clipboard` 與 `internal/tui`），並通過 `make all`、`make docs-check`、`make debt-check`、`gofmt -l`。
