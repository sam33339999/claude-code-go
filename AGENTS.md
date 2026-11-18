# Claude Code Go — AI Agent Instructions

## 語言偏好（使用者指定）

- 與使用者對話、撰寫說明與文件時，**使用繁體中文＋用語**（除非使用者另有指定）。
- 程式碼、識別符、GoDoc 註解依 repo 現有慣例（GoDoc 以英文為主）；commit message 沿用現有風格（中文可）。

## 專案現狀：維護模式（2026-08-29 起）——先讀這段，避免被舊文件誤導

- Module：`github.com/tunsuy/claude-code-go`（**不是** anthropics）；Go 1.26（go.mod `go 1.26.0`）；MIT。
- 本專案是 Claude Code（TS）的 Go 重寫，2026-04 由多 Agent 團隊建構，現為**單人維護模式**：gap 分析驅動、feat 分支 + PR。
- **建構期治理文件已封存，不構成現行流程約束**：`docs/project/agents/`、`docs/project/harness/`、
  `docs/project/harness/tasks/task-registry.yaml`（已封存，勿再寫入）。不要自動扮演 PM Agent 或執行舊 SOP；
  使用者明確要求多 Agent 協調時才參考那些角色定義。
- **功能狀態的權威來源**：程式碼本身 + `git log` + `test/parity/cases.md`（parity 差距台賬）。
  `docs/ROADMAP.md` 與舊 QA 報告曾有「已註冊＝已實現」的虛報（2026-11-18 巡檢證實，修正中）——狀態衝突時以程式碼與台賬為準。
- stub 全量回填**已完成**（26→0；debt 基線現為 TODO(dep)=11、樁=0）。近期里程碑：Code Mode v1–v3、
  EV-01 MCP 事件驅動喚醒、Bubble Tea v2 遷移。任務台賬見 `docs/project/discussions/2026-11-18-stub-backfill-plan.md`，
  進度見 `docs/project/logs/` 最新巡檢日誌。

## 架構：内核已搬回本倉（2026-11-18 反轉 PR #31 提取）

agtkeel 外部倉停止維護，人類決策將内核**整併回本倉**（含其 OpenAI 相容增強：
normalizeBaseURL、ReasoningContent）。原 agtkeel 包恢復原始路徑：
- `internal/{api,compact,config,engine,hooks,mcp,msgqueue,permissions,session}`
- `internal/tools`（框架：Tool 介面 + Registry + BaseTool，與各 category 子包同目錄）
- `pkg/types`（零第三方依賴）、`pkg/utils/{fs,ids}`

本倉產品側不變：
- `cmd/claude`（Cobra 入口）、`internal/bootstrap`（DI 連線點 `AppContainer`、啟動接線）
- `internal/tui` + `internal/commands`（Bubble Tea MVU、slash 命令）。
  **TUI 用 charm.land/v2 路徑**：`charm.land/bubbletea/v2`、`charm.land/lipgloss/v2`、`charm.land/bubbles/v2`——
  不是 `github.com/charmbracelet/*` v1（go.mod 頂部註解是舊的，勿信；v1 路徑編譯不會過）。
- `internal/tools/<category>`（具體工具實現）
- `internal/coordinator、oauth、plugin、state、memdir、agentctx、agenttype、transcript、codemode（CM-01..03 JS/TS 沙箱引擎，moejs v0.1.1 釘版）`；`pkg/utils、pkg/testutil`

依賴鐵律（CI 評審會查）：
- 下層禁止 import 上層；**TUI/commands 禁止 import bootstrap**（接線是 bootstrap 的職責，依賴用建構子注入）。
- `internal/tools/` 各子包**禁止互相 import**（框架根包除外，子包依賴根包契約合法）。
- `pkg/types` 零依賴，所有層可依賴。
- **不再有外部内核依賴**：go.mod 無 agtkeel；内核改動直接在本倉做（同倉 PR）。

## 命令

```bash
make build        # → bin/claude
make test         # go test -race ./...（需 CGO + C 編譯器；缺 gcc 時：apt-get install gcc）
make vet          # go vet ./...
make lint         # golangci-lint（需另裝）
make all          # vet + test + build
make docs         # cmd/docgen 重生成各包 CONTEXT.md + docs/generated/INDEX.md
make docs-check   # CI 閘門：驗證生成文件與程式碼同步
make debt-check   # CI 閘門：欠賬紅線（見下）
make parity       # 先 build 再跑 test/parity；未設 CCG_PARITY_ORACLE 時全部 SKIP
```

- 單一包：`go test -race ./internal/tools/mcp/`；單一測試：`go test -race -run TestName ./internal/tui/`
- 已知偶發 flake：`TestAdapterStopAgent`（coordinator）在全倉併發 -race 下時序競態；單獨重跑該包確認，勿逕行歸咎新改動。
- TUI 測試斷言用「詞」而非完整字串（ANSI 碼會插在 token 之間）；測 `Update()` 與 view 函式，無需真實終端。

## CI 紅線（會擋 PR）

1. **欠賬棘輪** `scripts/debt-check.sh`：`TODO(dep)` 與用戶可見 `not yet implemented` 樁計數相對
   `scripts/debt-baseline.txt` **只降不升**。消滅樁/TODO 後跑 `scripts/debt-check.sh --update`，
   新基線**與實現同一個 commit**。新增樁必須在 PR 說明給回填 issue 編號。
2. **生成文件同步**：各包 `CONTEXT.md` 與 `docs/generated/` 由 `make docs` 生成，**勿手改**；
   唯一可手寫的是結尾 `## Design Notes` 段（重生成時保留）——設計決策寫這裡。改導出符號後必跑 `make docs`。
   **順序陷阱**：Notes 要在 `make docs` **之後**附加（CONTEXT.md 還不存在時先寫會被生成覆蓋）；
   docs-check 會擋「有程式碼變更但無 Design Notes」的包。
3. **gofmt**：CI 對未格式化檔案失敗；送審前 `gofmt -l internal/ cmd/` 應為 0。
4. lint / vet / 全倉 -race / docs-check / debt-check 全綠。

## Cache-first 評審閘門（2026-11-18 正式採納）

依據 `docs/PROMPT_CACHE_AND_COST.md`（Reasonix 參考規範，路徑映射見該文件與巡檢日誌）：
- 觸及 **prompt 構造、工具 schema、工具註冊/增刪/改名、MCP 註冊、壓縮、請求序列化**的變更，
  PR 說明必須回答三問：**是否移動前綴？是否增大窗口？是否新增呼叫？**
- 移動前綴＝成本決策，須附論證＋聚焦守護測試；動態狀態優先掛 turn 尾部，不改前綴。
- **會話內工具集合與 schema 字節釘住**：MCP 重連差異下個會話生效，禁止會話中途重新註冊工具。
- 壓縮是唯一被認可的前綴重置。

## 編碼規範（摘要；細節見 CLAUDE.md）

- 檔案 ≤800 行；函式 ≤80 行；測試檔 ≤1600 行；測試函式 ≤160 行；巢狀 ≤4 層；行寬 ≤120。
- 所有 error 必處理，`fmt.Errorf("context: %w", err)` 包裝，盡早 return；一般錯誤禁 panic。
- 匯入三組排序：標準庫 / 第三方 / 內部（`github.com/tunsuy/...`），空行分隔。
- 導出符號必有 GoDoc；包註解每包一份；送審前刪註解掉的程式碼。
- 共享狀態用 mutex 保護並註明 guarded 欄位；I/O 函式首參 `context.Context`。
- TUI 嚴守 MVU：狀態在 `AppModel`（值類型），變更走 `Update() → tea.Cmd → tea.Msg`，副作用用 `tea.Cmd`。
- 新工具：`internal/tools/<category>/` 嵌入 `tools.BaseTool`（internal/tools 框架），同包補測試。
  註冊位置分兩種：無運行時依賴→`internal/bootstrap/tools.go`（RegisterBuiltinTools）；
  需要 checker/engine/channel 等依賴→`internal/bootstrap/wire.go` 接線（CodeMode、AskUserQuestion 前例），
  **兩條建構路徑（生產＋test）都要接**。
- 介面變更按 `docs/project/doc-sync-policy.md` 同步 `docs/project/design/<layer>.md`（版本號 + 變更記錄）。

## 分支與 PR

- 分支命名：`feat/` `fix/` `docs/` `refactor/` `test/` + 描述；**禁止直接 push main**。
- **現行整合流程**（已驗證的慣例）：`feat/impl` 是整合分支（本地＝origin 保持同步）。
  功能在 worktree 開 `feat/<task>` 分支→門控全綠→commit→回主 checkout
  `git merge --no-ff` 進 feat/impl→合併後重跑全量門控→`git push origin feat/impl`→移除 worktree。
  巡檢日誌等純 docs 可直接 commit 在 feat/impl。
- **git add -A 前看一眼**：`.claude/projects/`、`.claude/codemode/` 是本地運行資料（已 gitignore，勿入库）。
- 改包前先讀該包 `CONTEXT.md`（導出簽名＋變更影響鏈）；再讀對應 `docs/project/design/<layer>.md`。
- parity 差距關閉流程：更新 `test/parity/cases.md` 狀態 → 用例從 skip/樁名單移入斷言名單 → 若消樁同步 debt 基線。

## 文件地圖（哪個問題讀哪份）

| 問題 | 來源 |
|------|------|
| 慣例細節、設計模式、bootstrap 階段 | `CLAUDE.md`（Project Layout 已隨内核搬回更新；本文架構段為準） |
| 貢獻流程、PR 模板 | `CONTRIBUTING.md`（架構表為換庫前舊版，僅流程部分有效） |
| 某包導出與影響鏈 | 該包 `CONTEXT.md`（生成） |
| 功能真實狀態 / 與原版差距 | `test/parity/cases.md` + 程式碼 |
| 當前任務與進度 | `docs/project/discussions/2026-11-18-stub-backfill-plan.md`、`docs/project/logs/`（巡檢日誌） |
| Code Mode 設計/局限/moejs API 陷阱 | `docs/project/design/codemode.md`＋`internal/codemode/CONTEXT.md` |
| 歷史教訓（為何有這些紅線） | `docs/project/discussions/2026-08-29-process-retrospective.md` |
