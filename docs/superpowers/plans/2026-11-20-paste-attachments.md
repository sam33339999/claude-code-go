# Paste Collapse & Clipboard Images Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 大段貼上文字收合成 `[Pasted text #N …]` 標記、Ctrl+V 貼剪貼簿圖片成 `[Image #N]`，送出時展開給模型。

**Architecture:** `internal/tui` 新增 session 存活的 `pasteStore`（標記 ↔ 內容），輸入框只放標記文字；刪除鍵在 `handleIdleKey` 攔截做整顆刪除；`startQueryCmd`／`/btw`／`!`／`#`／slash 在送出時依來源展開。新 Infra 套件 `internal/clipboard` 以可注入的 command runner 讀 macOS／Linux 剪貼簿並正規化圖片。

**Tech Stack:** Go 1.26、`charm.land/bubbletea/v2`、`charm.land/bubbles/v2/textarea`、新增 `golang.org/x/image`（`draw`、`webp`、`tiff`、`bmp`，純 Go，無 CGO）。

**Spec:** `docs/superpowers/specs/2026-11-20-paste-attachments-design.md`

## Global Constraints

- 平台：macOS + Linux（Wayland／X11）；其他平台讀圖回 `ErrUnsupported`，文字照常貼。
- 收合門檻：`runes > 800` **或** `行數 > 10`（行數 = `strings.Count(s, "\n") + 1`，先把 `\r\n`、`\r` 正規化為 `\n`）。
- 標記格式：多行 `[Pasted text #N +L lines]`（L = 換行數）；單行 `[Pasted text #N C chars]`（C = rune 數）；圖片 `[Image #N]`。文字與圖片共用編號，從 1 起單調遞增，`/clear` 不重置。
- 有效標記 = 字串**完全等於**暫存區中該編號項目的 marker。
- 圖片：長邊 > 1568 px 縮到 1568；編碼後上限 3,750,000 bytes；超過改 JPEG 品質 85；仍超過拒絕。只送 PNG／JPEG／GIF／WebP。
- Session 圖片上限 20 張（歷史中的 image block，含 tool_result 內，加上輸入框中有效的圖片標記）。
- 外部指令逾時 5 秒；剪貼簿讀取一律走 `tea.Cmd`，不可在 `Update` 內同步執行。
- 產品 UI 文案用英文（與既有 `btwPendingHint`、`bashPendingHint` 一致）；文案見各任務。
- 紅線：檔案 ≤ 800 行（`keys.go` 已 695、`update.go` 已 716，新邏輯放新檔）、函式 ≤ 80 行、導出符號要有 GoDoc、`fmt.Errorf("...: %w", err)`、I/O 函式首參 `context.Context`、import 三組排序、`gofmt -l` 無輸出。
- 每段結束：`make docs` 後補 `## Design Notes`、`make all`、`make docs-check`、`make debt-check`、`gofmt -l internal/ cmd/`。

## Review Focus

1. 貼上的內容本身含有標記字樣（例如貼上一段先前的對話紀錄）：展開必須單趟，不可遞迴展開，原文中的字樣保持字面。→ Task 4 `TestExpand_SinglePass`。
2. 查詢進行中送出含標記的訊息（進佇列），回合結束後才送出：仍要展開並附上圖片。→ Task 4 `TestSubmit_QueuedPromptExpands`。
3. ↑ 叫回含標記的歷史訊息再送出：標記仍有效、可整顆刪、可展開。→ Task 3 `TestDeleteToken_RecalledHistory`。
4. `/btw` 答案（Origin=btw）或 MCP 通知裡出現 `[Image #1]` 字樣：不可展開、不可附圖。→ Task 4 `TestSubmit_NonUserSourcesNotExpanded`。
5. 剪貼簿讀取還沒回來時，使用者繼續打字或已經送出：結果插在當下游標；不 panic、不插到已送出的訊息。→ Task 7 `TestCtrlV_ResultInsertedAtCurrentCursor`。

---

## 段落 1：文字收合（Task 1–4，結束後審查）

### Task 1: pasteStore 與標記

**Files:**
- Create: `internal/tui/paste.go`
- Test: `internal/tui/paste_test.go`

**Interfaces:**
- Produces:
  - `type pasteKind int`，常數 `pasteText`、`pasteImage`
  - `type pastedItem struct { kind pasteKind; marker string; text string; image *types.ImageSource }`
  - `type pasteStore struct { items map[int]*pastedItem; nextID int }`
  - `func newPasteStore() *pasteStore`
  - `func (s *pasteStore) addText(text string) string` — 回傳 marker（呼叫端已確認超過門檻）
  - `func (s *pasteStore) addImage(src *types.ImageSource) string`
  - `func (s *pasteStore) isMarker(tok string) bool`
  - `func shouldCollapse(text string) bool`
  - `func normalizeNewlines(s string) string`
  - `var markerRe = regexp.MustCompile(`\[(?:Pasted text #\d+ (?:\+\d+ lines|\d+ chars)|Image #\d+)\]`)`

- [ ] **Step 1: 寫失敗測試**

```go
func TestShouldCollapse(t *testing.T) {
	cases := []struct{ in string; want bool }{
		{strings.Repeat("a", 800), false},
		{strings.Repeat("a", 801), true},
		{strings.Repeat("中", 801), true},           // rune 計數
		{strings.Repeat("x\n", 9) + "x", false},     // 10 行
		{strings.Repeat("x\n", 10) + "x", true},     // 11 行
		{strings.Repeat("x\r\n", 10) + "x", true},   // CRLF 正規化
	}
	for _, c := range cases {
		if got := shouldCollapse(c.in); got != c.want {
			t.Errorf("shouldCollapse(%d runes) = %v, want %v", len([]rune(c.in)), got, c.want)
		}
	}
}

func TestPasteStore_Markers(t *testing.T) {
	s := newPasteStore()
	if m := s.addText(strings.Repeat("l\n", 312) + "end"); m != "[Pasted text #1 +312 lines]" { t.Errorf("got %q", m) }
	if m := s.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="}); m != "[Image #2]" { t.Errorf("got %q", m) }
	if m := s.addText(strings.Repeat("a", 1234)); m != "[Pasted text #3 1234 chars]" { t.Errorf("got %q", m) }
	for tok, want := range map[string]bool{
		"[Pasted text #1 +312 lines]": true,
		"[Pasted text #1 +311 lines]": false, // 行數不符
		"[Image #2]":                  true,
		"[Image #1]":                  false, // #1 是文字
		"[Image #9]":                  false, // 不存在
	} {
		if got := s.isMarker(tok); got != want { t.Errorf("isMarker(%q) = %v", tok, got) }
	}
}
```

- [ ] **Step 2: 執行確認失敗** — `go test ./internal/tui/ -run 'TestShouldCollapse|TestPasteStore' -count=1`，預期編譯失敗（未定義）。
- [ ] **Step 3: 實作 `paste.go`** — `addText` 先 `normalizeNewlines` 再存，換行數 `> 0` 用 `+L lines`，否則 `C chars`。
- [ ] **Step 4: 執行確認通過** — 同上指令，PASS。
- [ ] **Step 5: Commit** — `git add internal/tui/paste.go internal/tui/paste_test.go && git commit -m "feat(tui): 新增貼上暫存區與收合標記"`

### Task 2: 貼上文字時收合

**Files:**
- Modify: `internal/tui/model.go`（`AppModel` 加 `pastes *pasteStore`；`newAppModel` 初始化 `newPasteStore()`）
- Modify: `internal/tui/update.go:393-403`（`tea.PasteMsg` 分支改呼叫 `insertPaste`）
- Modify: `internal/tui/input.go`（新增 `InsertString`）
- Create: `internal/tui/paste_input.go`
- Test: `internal/tui/paste_input_test.go`

**Interfaces:**
- Consumes: Task 1 的 `shouldCollapse`、`normalizeNewlines`、`(*pasteStore).addText`
- Produces:
  - `func (m InputModel) InsertString(s string) InputModel`
  - `func (m AppModel) insertPaste(text string) (AppModel, tea.Cmd)` — 超過門檻插 marker，否則插 `normalizeNewlines(text)`；結束時重設 `m.history.idx` 並回傳 `m.refreshCompletion()`

- [ ] **Step 1: 寫失敗測試**

```go
func TestPaste_LargeTextCollapses(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	big := strings.Repeat("line\n", 40) + "end"
	updated, _ := m.Update(tea.PasteMsg{Content: big})
	m = updated.(AppModel)
	if got := m.input.Value(); got != "[Pasted text #1 +40 lines]" { t.Fatalf("input = %q", got) }
}

func TestPaste_SmallTextInline(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("see: ")
	updated, _ := m.Update(tea.PasteMsg{Content: "a\r\nb"})
	if got := updated.(AppModel).input.Value(); got != "see: a\nb" { t.Fatalf("input = %q", got) }
}

func TestPaste_InsertsAtCursor(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("ab")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	updated, _ := m.Update(tea.PasteMsg{Content: strings.Repeat("z", 900)})
	if got := updated.(AppModel).input.Value(); got != "a[Pasted text #1 900 chars]b" { t.Fatalf("input = %q", got) }
}
```

- [ ] **Step 2: 執行確認失敗** — `go test ./internal/tui/ -run TestPaste_ -count=1`，預期 FAIL（目前整段插入）。
- [ ] **Step 3: 實作** — setup wizard 與其他 dialog 分支維持原樣；只有 `dialogNone` 走 `insertPaste`。
- [ ] **Step 4: 執行確認通過** — 同上 PASS；`go test -race ./internal/tui/ -count=1` 全綠。
- [ ] **Step 5: Commit** — `git commit -m "feat(tui): 大段貼上文字收合成標記"`（加上上列檔案）

### Task 3: 標記整顆刪除

**Files:**
- Modify: `internal/tui/paste.go`（純函式 `tokenSpan`）
- Modify: `internal/tui/input.go`（`DeleteToken`）
- Modify: `internal/tui/paste_input.go`（`tryDeleteToken`）
- Modify: `internal/tui/keys.go:342`（在 vim normal 區塊**之前**呼叫 `tryDeleteToken`）
- Test: `internal/tui/paste_test.go`、`internal/tui/paste_input_test.go`

**Interfaces:**
- Produces:
  - `type deleteDir int`，常數 `delBackward`、`delForward`、`delWordBackward`
  - `func tokenSpan(line []rune, col int, dir deleteDir, isToken func(string) bool) (start, end int, ok bool)` — rune 索引，半開區間
  - `func (m InputModel) DeleteToken(dir deleteDir, isToken func(string) bool) (InputModel, bool)`
  - `func (m AppModel) tryDeleteToken(key tea.KeyPressMsg) (AppModel, bool)`

規則（`[s,e)` 為 `markerRe` 在該列的一個符合且 `isToken` 為真的區間）：
- `delBackward`：`s < col && col <= e`
- `delForward`：`s <= col && col < e`
- `delWordBackward`：先略過 `col` 左側連續空白得 `p`，`s < p && p <= e`；只刪 `[s,e)`，保留空白
- 按鍵對應：`backspace`→後退；`delete`→前進；`ctrl+w`、`alt+backspace`、`ctrl+backspace`→刪詞；vim normal 模式的 `x`→前進。其他鍵不處理。

`DeleteToken` 做法：`SetCursorColumn(end)` 後送 `end-start` 次 `tea.KeyPressMsg{Code: tea.KeyBackspace}` 給 `m.textarea`（直接呼叫 textarea，繞過 vim 分派），以免跨列重設游標。

- [ ] **Step 1: 寫失敗測試**

```go
func TestTokenSpan(t *testing.T) {
	valid := func(s string) bool { return s == "[Image #1]" }
	line := []rune("see [Image #1] now")   // marker 在 [4,14)
	cases := []struct{ col int; dir deleteDir; ok bool }{
		{14, delBackward, true}, {9, delBackward, true}, {4, delBackward, false},
		{4, delForward, true}, {13, delForward, true}, {14, delForward, false},
		{15, delWordBackward, true}, {18, delWordBackward, false},
	}
	for _, c := range cases {
		s, e, ok := tokenSpan(line, c.col, c.dir, valid)
		if ok != c.ok || (ok && (s != 4 || e != 14)) { t.Errorf("col=%d dir=%d → %d,%d,%v", c.col, c.dir, s, e, ok) }
	}
	if _, _, ok := tokenSpan([]rune("[Image #9]"), 10, delBackward, valid); ok { t.Error("invalid marker must not match") }
}

func TestDeleteToken_Backspace(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	marker := m.pastes.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	m.input = m.input.SetValue("look " + marker)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.input.Value(); got != "look " { t.Fatalf("input = %q", got) }
}

func TestDeleteToken_TypedLookalikeIsPlainText(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	m.input = m.input.SetValue("[Image #9]")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.input.Value(); got != "[Image #9" { t.Fatalf("input = %q", got) }
}

func TestDeleteToken_RecalledHistory(t *testing.T) {
	m, _ := newBTWModel(t, nil)
	updated, _ := m.Update(tea.PasteMsg{Content: strings.Repeat("q\n", 20)})
	m = updated.(AppModel)
	m, cmd := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	runCmds(cmd)
	m.isLoading = false
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.input.Value(); got != "" { t.Fatalf("recalled marker not deleted whole: %q", got) }
}
```

另加一個 vim normal 模式 `x` 的測試（`newAppModel` 的 `vimEnabled` 參數為 true，先按 Esc 進 normal，游標移到標記上，按 `x`，期望標記整顆消失）。

- [ ] **Step 2: 執行確認失敗** — `go test ./internal/tui/ -run 'TestTokenSpan|TestDeleteToken' -count=1`，FAIL。
- [ ] **Step 3: 實作** — 刪除後與 `updateInput` 一樣重設 `m.history.idx` 並 `refreshCompletion()`。
- [ ] **Step 4: 執行確認通過** — 同上 PASS；`go test -race ./internal/tui/ -count=1` 全綠。
- [ ] **Step 5: Commit** — `git commit -m "feat(tui): 貼上標記可整顆刪除"`

### Task 4: 送出時依來源展開

**Files:**
- Modify: `internal/tui/paste.go`（`expand`、`expandTextOnly`）
- Modify: `internal/tui/cmds.go:41,71-76`（簽名加 `fromUser bool`；組 `Content`）
- Modify: `internal/tui/keys.go:489`（`true`）、`update.go:509`（`cmd.Origin == ""`）、`update.go:518`（`false`）、`update.go:691`（`false`）
- Modify: `internal/tui/btw.go:155`（`/btw` 問題展開並附圖）
- Modify: `internal/tui/keys.go`：`!`（`parseBang` 之後）與 `# ` 分支呼叫 `expandTextOnly`
- Modify: `internal/tui/update.go:439` `handleSlashCommand` 開頭呼叫 `expandTextOnly`
- Modify: `internal/tui/paste_input.go`（`pasteNotice` 欄位顯示、清除）、`internal/tui/view.go:49`（hint 列加 notice）
- Test: `internal/tui/paste_test.go`、`internal/tui/paste_submit_test.go`
- Docs: `make docs`；`internal/tui/CONTEXT.md` 的 `## Design Notes` 補一段；`docs/project/design/tui.md` 補「貼上收合」小節

**Interfaces:**
- Consumes: Task 1 的 `markerRe`、`pasteStore`
- Produces:
  - `type expansion struct { Text string; Images []types.ContentBlock }`
  - `func (s *pasteStore) expand(text string) expansion` — 單趟 `ReplaceAllStringFunc`：有效文字標記 → 原文；有效圖片標記 → 字樣保留、image block 依首次出現順序加入（同編號只加一次）；其他字樣原樣
  - `func (s *pasteStore) expandTextOnly(text string) (out string, droppedImages bool)`
  - `func startQueryCmd(m *AppModel, userText string, fromUser bool) tea.Cmd`
  - `AppModel.pasteNotice string`；`func (m *AppModel) setPasteNotice(s string)`；任何按鍵在 `handleIdleKey` 開頭清掉 notice
- 送出時 `Content` 為 `[images…, text]`；無圖時與現在完全相同（單一 text block）。展開在 `takeBTWPending`／`takeBashPending` **之前**。
- 文案：`expandTextOnly` 回報 `droppedImages` 時 notice 為 `Images can only be attached to a normal message or /btw`。

- [ ] **Step 1: 寫失敗測試**

```go
func TestExpand_SinglePass(t *testing.T) {
	s := newPasteStore()
	inner := "quoted [Image #2] and [Pasted text #1 +1 lines]\n" + strings.Repeat("x", 900)
	m1 := s.addText(inner)
	m2 := s.addImage(&types.ImageSource{Type: "base64", MediaType: "image/png", Data: "AA=="})
	exp := s.expand(m1 + " " + m2 + " " + m2)
	if exp.Text != inner+" [Image #2] [Image #2]" { t.Errorf("text = %q", exp.Text) }
	if len(exp.Images) != 1 { t.Errorf("images = %d, want 1 (dedup, inner text not re-expanded)", len(exp.Images)) }
}

func TestSubmit_ExpandsTextAndAttachesImages(t *testing.T) { /* addImage + addText 到 m.pastes；submit("look "+img+" "+txt)；
	斷言 be.queries 最後一個請求最後一則訊息 Content[0].Type == image、Content[1] 文字含原文與 "[Image #1]"；
	m.messages 最後一則顯示文字仍是含標記的原輸入 */ }

func TestSubmit_QueuedPromptExpands(t *testing.T) { /* m.isLoading = true 時 submit 含標記文字 → 佇列 Value 仍含標記；
	m.isLoading = false 後呼叫 processQueueCmd(&m) 並執行 → 請求已展開且附圖 */ }

func TestSubmit_NonUserSourcesNotExpanded(t *testing.T) { /* addImage 產生 [Image #1]；
	(a) Enqueue ModePrompt + Origin=msgqueue.OriginBTW、(b) ModeExternalPrompt，值皆含 "[Image #1]"；
	各自 processQueueCmd → 請求只有一個 text block 且含字面 "[Image #1]" */ }

func TestSubmit_BangExpandsTextDropsImages(t *testing.T) { /* "!" + 文字標記 + 圖片標記 → 執行的指令含原文、
	圖片標記保留字面；View 含 "Images can only be attached" */ }

func TestSubmit_BTWAttachesImages(t *testing.T) { /* "/btw what is " + img → btw 請求最後一則 Content 首個為 image */ }
```

- [ ] **Step 2: 執行確認失敗** — `go test ./internal/tui/ -run 'TestExpand|TestSubmit_' -count=1`，FAIL。
- [ ] **Step 3: 實作** — 依 Interfaces；`startQueryCmd` 所有呼叫點都要改（`grep -n 'startQueryCmd(' internal/tui/*.go` 確認無遺漏，含測試檔）。
- [ ] **Step 4: 執行確認通過** — `go test -race ./internal/tui/ -count=1` 全綠。
- [ ] **Step 5: 文件與檢查** — `make docs` → 補 `internal/tui/CONTEXT.md` Design Notes（session 存活暫存區的理由、只展開使用者輸入的理由）→ 更新 `docs/project/design/tui.md` → `make all && make docs-check && make debt-check && gofmt -l internal/ cmd/`，全部通過、gofmt 無輸出。
- [ ] **Step 6: Commit** — `git commit -m "feat(tui): 送出時展開貼上標記，僅限使用者輸入"`

**段落 1 結束：開 reviewer subagent 審 Task 1–4 的 diff，回到主 session 處理意見。**

---

## 段落 2：剪貼簿圖片（Task 5–7，結束後審查）

### Task 5: `internal/clipboard` 讀取

**Files:**
- Create: `internal/clipboard/clipboard.go`、`internal/clipboard/darwin.go`、`internal/clipboard/linux.go`（**不用 build tag**，依 `Reader.GOOS` 分派，讓兩平台都能在任一機器上測）
- Test: `internal/clipboard/clipboard_test.go`

**Interfaces:**
- Produces:
  - `var ErrNoImage = errors.New("clipboard: no image")`、`var ErrUnsupported = errors.New("clipboard: unsupported platform")`
  - `type MissingToolError struct{ Hint string }`，`Error()` 回傳 `Hint`
  - `type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)`
  - `type Reader struct { Run Runner; LookPath func(string) (string, error); Getenv func(string) string; GOOS string; TempDir string }`
  - `func New() *Reader`（預設：`exec.CommandContext(...).Output`、`exec.LookPath`、`os.Getenv`、`runtime.GOOS`、`os.TempDir()`）
  - `func (r *Reader) ReadImage(ctx context.Context) ([]byte, error)` — 回傳原始位元組（型別判斷在 Task 6）
  - `func (r *Reader) ReadText(ctx context.Context) (string, error)`
- 每次 `Run` 外包 `context.WithTimeout(ctx, 5*time.Second)`。
- darwin：`os.CreateTemp(TempDir, "glaude-clip-*.png")` 取得路徑後關閉；`osascript -e 'set d to (the clipboard as «class PNGf»)' -e 'set f to open for access POSIX file "<path>" with write permission' -e 'set eof f to 0' -e 'write d to f' -e 'close access f'`；osascript 失敗或檔案為空 → `ErrNoImage`；`defer os.Remove`。文字：`pbpaste`。
- linux：`Getenv("WAYLAND_DISPLAY") != ""` 且有 `wl-paste` → `wl-paste --list-types`／`wl-paste --type T`／`wl-paste --no-newline`；否則有 `xclip` → `xclip -selection clipboard -t TARGETS -o`／`-t T -o`／`xclip -selection clipboard -o`；都沒有 → `&MissingToolError{Hint: "Install wl-clipboard or xclip to paste from the clipboard"}`。型別挑選順序 `image/png`、`image/jpeg`、`image/gif`、`image/webp`，再來第一個 `image/` 前綴；沒有 → `ErrNoImage`。
- 其他平台：`ReadImage` → `ErrUnsupported`；`ReadText` → `github.com/atotto/clipboard.ReadAll()`（go.mod 由 indirect 改為 direct）。

- [ ] **Step 1: 寫失敗測試** — 以 fake `Runner` 記錄呼叫並回傳預設輸出：
  - `TestReadImage_DarwinWritesTempFile`：fake 在收到 osascript 時把 `PNG` 魔數寫進腳本中的路徑；斷言回傳位元組相同、暫存檔已刪除。
  - `TestReadImage_DarwinNoImage`：osascript 回錯 → `errors.Is(err, ErrNoImage)`。
  - `TestReadImage_WaylandPicksPNG`：`--list-types` 回 `"text/plain\nimage/jpeg\nimage/png\n"` → 第二次呼叫為 `wl-paste --type image/png`。
  - `TestReadImage_X11Fallback`：無 `WAYLAND_DISPLAY`、`LookPath("xclip")` 成功 → 呼叫 `xclip -selection clipboard -t TARGETS -o`。
  - `TestReadImage_LinuxMissingTools`：`LookPath` 全失敗 → `*MissingToolError` 且 Hint 為上列文案。
  - `TestReadImage_OtherPlatform`：`GOOS="windows"` → `ErrUnsupported`。
  - `TestRun_Timeout`：fake 檢查收到的 ctx 有 deadline 且 ≤ 5s。
- [ ] **Step 2: 執行確認失敗** — `go test ./internal/clipboard/ -count=1`，編譯失敗。
- [ ] **Step 3: 實作**
- [ ] **Step 4: 執行確認通過** — 同上 PASS。
- [ ] **Step 5: Commit** — `git commit -m "feat(clipboard): 讀取 macOS/Linux 剪貼簿圖片與文字"`

### Task 6: 圖片正規化

**Files:**
- Create: `internal/clipboard/image.go`
- Test: `internal/clipboard/image_test.go`
- Modify: `go.mod`／`go.sum`（`go get golang.org/x/image@v0.44.0`）

**Interfaces:**
- Produces:
  - `const MaxImageBytes = 3_750_000`、`const MaxImageEdge = 1568`
  - `var ErrImageTooLarge`、`var ErrImageFormat`
  - `func NormalizeImage(data []byte) (out []byte, mediaType string, err error)`
- 規則：
  1. `image.DecodeConfig` 取尺寸與格式（註冊 png、jpeg、gif、`x/image/webp`、`x/image/tiff`、`x/image/bmp`）；失敗 → `ErrImageFormat`。
  2. 格式為 png／jpeg／gif／webp、長邊 ≤ 1568、`len(data) ≤ MaxImageBytes` → **原樣回傳**（保留 GIF 動畫），media type 用 `http.DetectContentType`。
  3. 否則完整解碼；長邊 > 1568 用 `draw.CatmullRom` 等比縮放；先編 PNG，> 上限改 JPEG 品質 85，仍 > 上限 → `ErrImageTooLarge`。

- [ ] **Step 1: 寫失敗測試**（以 `image.NewRGBA` 在測試內產生圖片）
  - `TestNormalize_SmallPNGUnchanged`：100×50 PNG → 位元組完全相同、`image/png`。
  - `TestNormalize_DownscalesLongEdge`：3000×1000 PNG → 解碼後為 1568×523（四捨五入）。
  - `TestNormalize_TIFFConvertedToPNG`：TIFF 輸入 → `image/png`。
  - `TestNormalize_NoiseFallsBackToJPEG`：1568×1568 隨機雜訊（PNG 會超過 3.75 MB）→ `image/jpeg` 且 ≤ `MaxImageBytes`。
  - `TestNormalize_Garbage`：`[]byte("not an image")` → `errors.Is(err, ErrImageFormat)`。
- [ ] **Step 2: 執行確認失敗** — `go test ./internal/clipboard/ -run TestNormalize -count=1`，FAIL。
- [ ] **Step 3: 實作**
- [ ] **Step 4: 執行確認通過** — 同上 PASS；`CGO_ENABLED=0 go build ./...` 成功。
- [ ] **Step 5: Commit** — `git commit -m "feat(clipboard): 圖片縮圖與格式正規化"`

### Task 7: Ctrl+V 接線

**Files:**
- Create: `internal/tui/clipboard_paste.go`
- Modify: `internal/tui/model.go`（欄位 `clip clipboardReader`，`newAppModel` 預設 `clipboard.New()`）
- Modify: `internal/tui/input.go:NewInput`（`ta.KeyMap.Paste.SetEnabled(false)`）
- Modify: `internal/tui/keys.go`（`handleIdleKey` 中，`switch key.Code` 之前：`key.String() == "ctrl+v"` → `m.startClipboardPaste()`）
- Modify: `internal/tui/update.go`（分派 `clipboardImageMsg`、`clipboardTextMsg`、`clipboardNoticeMsg`）
- Test: `internal/tui/clipboard_paste_test.go`
- Docs: `make docs`；`internal/clipboard/CONTEXT.md`、`internal/tui/CONTEXT.md` 補 Design Notes；`docs/project/design/tui.md` 補「Ctrl+V 圖片」

**Interfaces:**
- Consumes: Task 5 `Reader`（透過介面）、Task 6 `NormalizeImage`、Task 2 `insertPaste`、Task 1 `addImage`、Task 4 `setPasteNotice`
- Produces:
  - `type clipboardReader interface { ReadImage(ctx context.Context) ([]byte, error); ReadText(ctx context.Context) (string, error) }`
  - `func (m AppModel) startClipboardPaste() (AppModel, tea.Cmd)`
  - `func readClipboardCmd(r clipboardReader) tea.Cmd` — 讀圖 → `NormalizeImage` → `clipboardImageMsg{data []byte; mediaType string}`；`ErrNoImage`／`ErrUnsupported` → 讀文字 → `clipboardTextMsg{text string}`；空文字 → notice
  - `func countImages(msgs []types.Message) int` — 最上層與 `tool_result.Content` 內的 image block
  - `const maxSessionImages = 20`
- 處理 `clipboardImageMsg` 時（此時才計數）：`countImages(m.queryEngine.GetMessages())` + 輸入框中有效圖片標記數（去重）≥ 20 → notice，不插入；否則 base64 編碼成 `types.ImageSource{Type: "base64", ...}`，`addImage` 後在當下游標 `InsertString(marker)`。dialog 開啟中收到結果則丟棄。
- 文案：
  - `Clipboard is empty`
  - `Pasting images is not supported on this platform`（`ErrUnsupported` 且文字也為空）
  - `MissingToolError.Hint`（原樣）
  - `This conversation already has 20 images · /compact before pasting more`
  - `Image is too large to send (limit 3.75 MB after downscaling)`
  - `Unsupported image format`
  - 其他錯誤：`Clipboard read failed: <err>`

- [ ] **Step 1: 寫失敗測試**（fake `clipboardReader` 指派到 `m.clip`）
  - `TestCtrlV_ImageInsertsMarker`：fake 回 10×10 PNG → 執行 cmd、送回 msg → 輸入為 `[Image #1]`，`m.pastes.isMarker("[Image #1]")`。
  - `TestCtrlV_TextFallbackCollapses`：fake 圖片回 `ErrNoImage`、文字回 900 字 → 輸入為 `[Pasted text #1 900 chars]`。
  - `TestCtrlV_TextareaBuiltinDisabled`：未攔截時 textarea 不再自行讀剪貼簿（`NewInput().textarea.KeyMap.Paste.Enabled()` 為 false）。
  - `TestCtrlV_SessionImageCap`：fake engine 歷史含 19 張圖片，輸入框已有 1 個有效圖片標記 → 再貼被拒，View 含 `already has 20 images`。
  - `TestCtrlV_ResultInsertedAtCurrentCursor`：送出 ctrl+v 後、送回結果前先 `SetValue("ab")` 並左移一格 → 結果為 `a[Image #1]b`；若中間已送出（輸入為空）→ 結果為 `[Image #1]`，且先前送出的訊息不含標記。
  - `TestCtrlV_MissingToolNotice`：fake 回 `&clipboard.MissingToolError{Hint: "Install wl-clipboard or xclip to paste from the clipboard"}` → View 含該字串，輸入不變。
- [ ] **Step 2: 執行確認失敗** — `go test ./internal/tui/ -run TestCtrlV -count=1`，FAIL。
- [ ] **Step 3: 實作**
- [ ] **Step 4: 執行確認通過** — `go test -race ./internal/tui/ ./internal/clipboard/ -count=1` 全綠；`go test ./internal/engine/ -run TestStripImages -count=1` 通過（確認最上層 user image 已被既有降級涵蓋；若該測試沒有涵蓋最上層 image，補一個 case）。
- [ ] **Step 5: 手動驗證（macOS）** — `make build`，執行 `bin/glaude`：截圖到剪貼簿（Cmd+Ctrl+Shift+4）→ Ctrl+V 出現 `[Image #1]` → 問「圖裡有什麼」得到描述；Backspace 整顆刪；Cmd+V 貼 500 行文字出現收合標記。
- [ ] **Step 6: 文件與檢查** — `make docs` → 補兩個套件的 Design Notes → 更新 `docs/project/design/tui.md` → `make all && make docs-check && make debt-check && gofmt -l internal/ cmd/`。
- [ ] **Step 7: Commit** — `git commit -m "feat(tui): Ctrl+V 貼上剪貼簿圖片"`

**段落 2 結束：開 reviewer subagent 審 Task 5–7；合併前再審整個分支。PR 說明附 spec 第 7 節的 cache-first 三問答案。**
