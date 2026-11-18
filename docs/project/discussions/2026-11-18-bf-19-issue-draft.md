# BF-19 GitHub Issue 草稿（待人類建立）

> 用途：直接貼到 https://github.com/sam33339999/claude-code-go/issues/new
> 建立後把 issue 編號回填到本檔、回填計畫台账 §5 BF-19 行、internal/mcp/CONTEXT.md。
> 建議 labels：enhancement、technical-debt、mcp

---

## 標題

BF-19: MCP pool 快照式 GetAll＋jsonRPC reader 生命周期解耦＋真實 transport teardown（CR-BF03-2 / CR-BF01-7 根因修復）

## 內容

### 背景

兩個 code review 發現的根因級技術債（非功能缺口）。目前**安全但不健壯**：
約束靠文檔而非機制維護。

#### 來源 1：CR-BF03-2（BF-03 /mcp 評審）

`Pool.GetAll()`（`internal/mcp/pool.go:152`）回傳新切片，但元素是池內
**共享的可變 `*ServerConnection` 活指標**。`Connect/Reconnect/Disconnect`
（pool.go:72/102）在池鎖內突變這些欄位；讀取側
（`FormatMCPStatus`，`internal/commands/mcp.go:50`，TUI fetch goroutine）
拿不到池鎖 → 一旦存在併發寫者即構成 data race。

#### 來源 2：CR-BF01-7（BF-01/02 MCP 接線評審，內核繼承限制）

1. **reader 綁定握手 ctx**：`jsonRPCClient.startReader`
   （`internal/mcp/jsonrpc.go:35`）的背景讀協程綁定首次呼叫
   （initialize）的 ctx——ctx 取消會撕掉整個連線。
   現以 `connectWithWatchdog`（`internal/bootstrap/mcp_connect.go:94`）
   長生命周期 ctx＋外部看門狗規避。
2. **HTTP teardown 失效**：`httpTransport.Close()`
   （`internal/mcp/transport.go:443`）僅設 closed 旗標，Send/Recv 從不檢查。

### 為什麼現在不阻塞

- BF-01-F1（commit `50b2427`）已消除所有會話期 pool 寫入
  （有界不遺棄 boot 連接：超時即 cancel＋drain，競速成功同步 Disconnect）
- 約束已文檔化：`MCPServerLister`/`FormatMCPStatus` GoDoc＋
  `docs/project/design/tui.md` §4.1＋三個包的 CONTEXT.md Design Notes
- 全倉 `go test -race` 全綠

### ⚠️ 升級規則（評審裁決，务必遵守）

**本 issue 落地前，任何引入會話期背景 pool 突變的 PR
（自動重連、健康檢查輪詢等）會使 CR-BF03-2 從潛在隱患升級為真實 P0 data race，
應擋下該 PR 或要求先完成本 issue。**

### 修復範圍

- [ ] **快照式 GetAll**：回傳值拷貝（鎖內複製 `ServerConnection` 值），
      讀者永遠不與寫者競態。`MCPServerLister` 介面契約同步更新
      （`internal/commands/mcp.go`）；移除「禁止併發突變」約束文檔
- [ ] **reader 生命周期解耦**：讀協程綁連線生命周期（獨立 ctx），
      不再綁首次呼叫 ctx；移除 bootstrap 看門狗規避
      （`connectWithWatchdog` 簡化為有界 dial）
- [ ] **真實 transport teardown**：Close 後 Send/Recv 立即報錯
      （http/stdio/ws 一致）；檢查各 transport 的 closed 守衛
- [ ] **WS 指數退避自動重連**（BF-16 保留項，ROADMAP §3.3.3）：
      **必須在快照 API 落地後**才安全；重連策略需 Tech Lead 裁決
      （退避參數、會話釘住模型下的 schema 穩定性——重連不得改寫
      本會話已註冊工具，見 `RegisterServerTools` pinning）

### 驗收標準

- [ ] 併發測試：讀者 goroutine 持续 GetAll＋渲染，寫者 goroutine
      持续 Connect/Reconnect/Disconnect，`-race` 零告警
- [ ] reader-ctx 解耦測試：取消 handshake ctx 不撕掉已建立連線
- [ ] teardown 測試：Close 後 Send/Recv 立即返回錯誤（各 transport）
- [ ] 自動重連測試：斷線→退避重連→狀態更新，會話 schema 字節不變
- [ ] 全倉 `CGO_ENABLED=1 go test -race -count=1 ./...` 綠
- [ ] `make docs-check` ✓；設計文檔同步（services.md/tui.md 版本＋變更記錄）
- [ ] 移除過時約束文檔（GoDoc/tui.md §4.1/CONTEXT Notes）

### 參考

- 評審報告：`docs/project/reviews/code-review-tui-bf03.md`（CR-BF03-2）、
  `docs/project/reviews/code-review-tools-bf01-02.md`（CR-BF01-7＋复核附錄）
- 任務台账：`docs/project/discussions/2026-11-18-stub-backfill-plan.md` §5（BF-19 行）
- 巡檢記錄：`docs/project/logs/patrol-13-20261118.md` §11/13/17/19/24
- 相關提交：`50b2427`（BF-01-F1 規避）、`c111afe`（BF-16 WS transport）
- 約束文檔：`docs/project/design/tui.md` §4.1、`internal/mcp/CONTEXT.md`

### 備註

- 內核已搬回本倉（ARCH-01，2026-11-18），全部改動在同倉 `internal/mcp`，
  無需跨倉協調
- API 形態（快照 GetAll 的回傳型別：值切片 vs 深拷貝指標）由 Tech Lead 裁決
- 優先級：P1（健壯性；非阻塞，但阻擋未來自動重連類功能）