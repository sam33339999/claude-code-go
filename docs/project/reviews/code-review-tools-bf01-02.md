# BF-01/02 Code Review — MCP 工具接線 + Sleep（Tools 層）

> **Reviewer**: Tech Lead Agent
> **Date**: 2026-11-18
> **Subject**: BF-01（MCP 三桩回填：AdaptToTool 接線、資源工具真實調用、啟動註冊）+ BF-02（Sleep）— Agent-Tools
> **Branch**: `feat/bf-01-02-mcp-sleep` · **Commit**: `1732d65` · **Diff base**: `db88b72`
> **Worktree**: `/root/.local/share/opencode/worktree/fcc669/bf-tools-b1`
> **Verdict**: **NOT pass-ready — 1 項 P0（CR-BF01-1，CR-BF03-2 升級裁決成立）+ 1 項 P1**，退回 Agent-Tools（經 PM）。

評審依據：`AGENTS.md`（維護模式版）鐵律與編碼規範、`docs/project/agents/tech-lead-agent.md` SOP、
`docs/project/design/tools.md` v1.1（本 PR 更新）與 `docs/project/design/cli.md`（接線注記）、
`docs/PROMPT_CACHE_AND_COST.md`（cache-first 門控）、
`docs/project/discussions/2026-11-18-stub-backfill-plan.md` §2 驗收門控、
`PM-FOLLOWUP-READ-BEFORE-COMMIT.md`（會話釘住 + CR-BF03-2 追加約束）、
`docs/project/doc-sync-policy.md`、BF-03 評審報告（bf-tui-b1 @ ca72211）之 CR-BF03-2 升級條件。
agtkeel 內核源碼（`/root/go/agtkeel-src`，唯讀）已逐檔核對：`mcp/{pool,adapter,client,jsonrpc,transport}.go`、
`tools/registry.go`、`config/{mcpstore,loader}.go`、`engine/orchestration.go`。

---

## 1. Overall Assessment

實現品質整體高、範圍克制：−4 桩（MCP 三桩 + Sleep），debt 基線 26→22 / 56→44 與實現同提交，
`scripts/debt-check.sh` 通過。刪除從未註冊的 `MCPProxyTool`/`NewMCPProxyTool`/`MCPToolInput` 死桩、
改用內核 `mcp.AdaptToTool`（零 kernel 重複實現）、資源工具構造期注入 Pool、
httptest Streamable-HTTP stub 走真實 Pool→client 全鏈路——這些都是正確且乾淨的做法。
會話釘住模型（包級 `pinnedRegistries`）語義正確、守護測試真實 exercised `pool.Reconnect`。
Sleep 的三路 select、存在性/類型/非負校驗、誠實 IsError 結果均達標。

**但啟動接線的看門狗設計引入了一個會話期間的背景 pool 突變路徑**（>30s 握手遲到落地），
精確命中 PM 追加約束與 BF-03 評審預設的 CR-BF03-2 升級條件（§5 裁決）。
另有一項启动 robustness P1（boot 期 `ListTools` 無超时，可被 hung server 無限阻塞）。
兩項均退回 Agent-Tools；P2 五項記錄跟進。

---

## 2. Architecture / Layering

**結論：合規。**

| 檢查項 | 結果 | 證據 |
|---|---|---|
| tools 不 import bootstrap / 工具間不互 import | ✅ | `go list -f '{{.Imports}}' ./internal/tools/mcp ./internal/tools/misc`：僅 stdlib + `agtkeel/mcp` + `agtkeel/tools`（misc 僅 `agtkeel/tools`） |
| bootstrap 負責接線 | ✅ | `wire.go:122-127,228-233` 先建 pool → `RegisterBuiltinTools(reg, pool)` → `connectConfiguredMCPServers`；tools 層以構造器接收 pool，無反向依賴 |
| `RegisterBuiltinTools` 簽名變更最小性 | ✅ | 僅新增 `pool *agtmcp.Pool` 一參數；全部调用點（wire.go ×2、測試 ×6）已更新；nil pool 優雅降級有測試（`TestRegisterBuiltinTools_NilPool`） |
| 刪除公開符號無殘留引用 | ✅ | 全倉 grep `MCPProxyTool|MCPToolInput|mcp.ListMcpResourcesTool|mcp.ReadMcpResourceTool` 零命中；stub 從未被註冊（基線 tools.go 只註冊兩個資源單例），**無運行時前綴影響** |
| import 三組排序 | ✅ | mcp.go / mcp_connect.go / misc.go 標準庫→agtkeel→內部；wire.go 順帶修正了基線遺留的混排 |
| `pkg/types` 零依賴 | ✅ | 本 PR 未触及 |

**裁決（`reg.Replace` vs `reg.Register`）**：boot 一次性註冊用 `Replace`（mcp.go:63）可接受——
工具名恆含 `__`（`{server}__{tool}`），與 34 個內建名無碰撞可能；`Replace` 避免重複 def  panic。
内核 registry.go 註釋將 Replace 定位為 reconnect 路徑，boot 用之屬輕微語義錯位，不構成問題項。

---

## 3. Design vs Implementation Delta

| 設計文档約定（tools.md v1.1 §4.4 / §7.9、cli.md 接線注記） | 實現狀態 | 說明 |
|---|---|---|
| `RegisterServerTools(ctx, reg, pool) (int, error)`，按服务器名排序、一次性、會話釘住 | ✅ 一致 | mcp.go:41-68；`sort.Slice` + `LoadOrStore` |
| 單服务器 ListTools 失敗不阻塞其餘，`errors.Join` 匯總 | ✅ 一致 | mcp.go:57-61；`TestRegisterServerTools_ListToolsError` |
| nil/空 pool → (0, nil) | ✅ 一致 | mcp.go:42-44；兩個測試 |
| 資源工具構造期注入 pool；nil→誠實錯誤、空→空列表、未知/未連接→IsError | ✅ 一致 | mcp.go:113-120,236-241,82-96；測試齊全 |
| `RegisterBuiltinTools(reg, pool)` 最小簽名變更 + 版本記錄 | ✅ 一致 | tools.go:25；tools.md v1.1 變更記錄表 |
| bootstrap 启动連接：ListMCPServers 語義 + project 批准門 + 30s 看門狗 + 失敗非致命 | ✅ 一致（但看門狗遲到落地有 P0，見 §5） | mcp_connect.go:36-91 |
| 「Connect 用長生命週期 ctx，超時僅約束啟動等待」 | ⚠️ 如實實現，但該設計本身違反 CR-BF03-2 追加約束 | 見 CR-BF01-1 |
| Sleep：timer + select 三路、存在性/類型/非負、0ms 立即成功、nil uctx 回退 | ✅ 一致 | misc.go:199-238；tools.md §7.9 |
| 動態連接狀態只進 /mcp，不進 Description/InputSchema | ✅ 一致 | 適配器 desc/schema 均為 boot 期靜態值（adapter.go:54-71） |

**doc-sync-policy 符合度**：簽名變更、新增/刪除公開符號、模塊職責變化均已同步至
tools.md v1.1（頭部版本 + §4.4 重寫 + §7.9 + 文末變更記錄表）與 cli.md 接線注記；
CONTEXT.md Design Notes 含 cache-stability 決策（PM 約束 #5 達標）。符合 Step 1-3。
唯一文件瑕疵見 CR-BF01-6（過時註釋）。

---

## 4. Cache Gate（cache-first 門控・獨立核驗）

提交说明已陳述釘住決策，但**未以三問形式逐條回答**（Q2/Q3 缺顯式答案，見 CR-BF01-6）。
以下為對照實際 diff 的獨立核驗：

### Q1 — 是否移動前綴？ **是（僅會話邊界，sanctioned 成本決策）**
- 相對基線，provider 工具面在 **boot 期一次性**新增已連接 MCP 服务器的工具（桩回填必然增工具，
  屬計劃內成本決策）；`RegisterServerTools` 的 `LoadOrStore` 釘住保證**會話內名集合與 schema 字節不變**。
- 內建面 byte-identical：兩個資源工具的 `Name/Description/InputSchema` 方法在 diff 中零改動
  （僅 Call/構造器/struct 變化）；刪除的三個 stub 符號從未註冊，無運行時影響。
- 註冊順序確定：服务器按名排序（mcp.go:49），工具按 server 上報名順序，`Registry.order` 保持插入序。
- schema 字節穩定性核驗（內核側）：`mcpTool.InputSchema()`（adapter.go:57-71）每次調用從
  boot 期凍結的 `json.RawMessage` 反序列化出 Properties/Required；`encoding/json` 對 map 鍵排序、
  RawMessage 原樣保留 → 序列化字節確定。`Description()` 返回靜態字串（ListTools 時已截斷 ≤2048 runes）。
  **無實時連接狀態嵌入 schema。** ✅

### Q2 — 是否增大窗口？ **有條件增加（已記錄，一处建議跟進）**
- 配置了 MCP server 時，其工具 schema/description 進入 provider 工具面（description 有內核 2048-rune 上限）。
  這是回填的必然代價，釘住模型保證會話內不增長。
- **ListMcpResources 聚合輸出無显式上限**：跨所有已連接 server 聚合資源列表，
  `BaseTool.MaxResultSizeChars() = -1`，且当前 agtkeel engine 不消费該值（engine 無 truncation）——
  資源極多的 server 可產生大 tool result。建議跟進（CR-BF01-4 附帶項）。

### Q3 — 是否新增調用？ **新增 boot 期 MCP 協議調用；無新增 LLM 調用**
- 每個已配置且獲批的 server：initialize + notifications/initialized + tools/list（此前交互模式為零連接）。
  失敗非致命、不重試、無背景輪詢——**穩態下無週期性新調用**。
- 資源工具調用為用戶/模型顯式觸發的只讀協議請求。

### 守護測試核驗（PM 約束 #4）
`TestRegisterServerTools_PinnedAcrossReconnect`（mcp_test.go:319-396）**真實 exercised**：
boot 註冊 → 快照名集合/schema 字節/description → stub 改 tool list（改描述、改 schema、增工具）→
`pool.Reconnect(context.Background(), "srv")` 真實重連（新 session-id 路由隔離）→ 二次
`RegisterServerTools` 斷言 (0, nil) no-op → 名集合逐位相等 + schema JSON 字節相等 + description 相等。
`-race -count=1 -v` 實跑通過。**守護有效**；可再補強（CR-BF01-3）。

### 釘住機制scrutiny（包級狀態）
- **per-registry 語義正確**：鍵為 `*tools.Registry`，`LoadOrStore` 原子——並發首次調用僅一個勝出；
  每個 container/session 獨享 registry（wire.go 每次 build 新建），語義等價於 per-session。
  `TestConnectConfiguredMCPServers_ProjectApprovalGate` 用共享 pool + 新舊兩個 registry 驗證了
  「新會話生效、舊會話釘住」。
- **測試隔離**：本套件每個測試新建 registry，無跨測試洩漏；nil pool/nil reg 在 LoadOrStore 之前返回，不污染標記。
- **可繞過性**：換一個新 registry 即可繞過——但這等價於「新會話」，與架構一致（registry 生命週期 = 會話）；
  會話中途無任何代碼路徑新建 registry 或調用 RegisterServerTools（全倉 grep 核實，唯一调用點為 boot）。
  `reg.Replace` 本身公開，釘住只守 MCP 註冊路徑——這是設計邊界，非漏洞。
- **包級狀態衛生**：`pinnedRegistries` 永不 prune（registry 進程生命週期保留）、無 reset API；
  CLI 單會話場景可忽略，長生命週期進程/大量 container 測試會緩慢累積。見 CR-BF01-3。
- **失敗即釘住**：ListTools 全部失敗時 registry 仍被標記，本會話 MCP 工具永久缺失（重試 no-op）。
  這是 one-shot 約束的保守後果（boot 期失敗不該在會話中途改工具面），接受，但應在 GoDoc 明示。

**Cache gate 結論：釘住機制本身通過；前綴移動屬 sanctioned 會話邊界決策，守護測試到位。**
（阻擋項不在 cache 機制，而在 §5 的並發約束。）

---

## 5. CR-BF03-2 Escalation Ruling（升級裁決・critical）

### 背景約束
- BF-03 評審 CR-BF03-2（P1，潛在）：`FormatMCPStatus` 無同步讀取 `Pool.GetAll` 返回的
  **共享可變 `*ServerConnection` 活指針**（Status/Error/Client/Config 在 pool 鎖下被 Connect/Reconnect/Disconnect 突變）。
  升級條件白紙黑字：「BF-01 若引入背景連接/健康檢查/重連 goroutine，本隱患即變為真實競爭」。
- PM 追加約束（PM-FOLLOWUP §追加）：「不得引入會話期間背景修改 pool 連接狀態的 goroutine（自動重連等），
  除非先有快照式讀取 API（BF-19，跨倉，未落地）」。patrol-13 §11：「BF-01 合併前須滿足該約束或完成 BF-19」。
- BF-19 狀態：未落地（跨倉 agtkeel 快照式 GetAll，尚在排隊）。

### BF-01 引入了什麼
逐條核對 diff 中的 goroutine（`git diff | grep "go func"` 僅一處生產代碼）：
- ✅ **無自動重連 goroutine、無健康檢查輪詢、無 watchdog 循環**。`RegisterServerTools`、資源工具均同步。
  BF-03 的 /mcp 為唯讀（無重連動作）。穩態下 pool 無寫者。
- ❌ **但 `connectWithWatchdog`（mcp_connect.go:76-91）在 30s 超時時遺棄仍在運行的
  `pool.Connect(context.Background(), …)` goroutine（mcp_connect.go:78-79）**。
  该 goroutine 越過 boot 進入會話；握手遲到完成時（無論成功或失敗），`Pool.Connect`
  在池鎖下寫入 `conn.Status/Client/Error`（agtkeel pool.go:87-97）——**這是會話期間的背景 pool 突變**。
  代码註釋（mcp_connect.go:71-75）明確承认並依賴此行為（「a handshake that completes later still lands in the pool」）。
  `ctx.Done()` 分支（mcp_connect.go:88）同样遺棄 goroutine（今日 wire 傳 Background 不可達，但 API  invitation 存在）。

### 競爭可達性（精確界定）
- 觸發條件：某 server 握手 >30s 但最終完成（現實場景：npx 下載、慢啟動 stdio server），
  且會話中 `/mcp` 渲染（bf-tui-b1 commands/mcp.go:70 `c := *live` 無鎖拷貝全部字段）
  或資源工具 `serverClient`（mcp.go:90 無鎖讀 `conn.Status/conn.Client`）**恰與遲到寫入同時執行**。
- 記憶體模型精確性：寫入在 `p.mu.Lock` 下、讀取側 `GetAll` 的 `RLock` 會與之建立 happens-before——
  故**寫入完成之後**的讀取有序；競爭窗口精確存在於**寫入執行期間**的並發讀取（µs 級）。
  窗口窄，但屬 Go 記憶體模型下的真實 data race，且 `-race` 全綠僅因無測試exercise此路徑
  （`connectWithWatchdog` 覆蓋率 77.8%，超時分支零覆蓋）。
- 附帶觀察：遲到落地的「收益」在釘住模型下是**虛幻的**——其工具不會被本會話註冊
  （RegisterServerTools 已 no-op），註釋所謂「next RegisterServerTools pass, e.g. after a /mcp reconnect」
  與釘住模型矛盾（同 registry 再 pass 恒 no-op；BF-03 /mcp 無重連動作）。
  遲到落地只換來 /mcp 顯示與資源工具可用性，卻製造了競爭窗口——**得不償失**。

### 裁決
**CR-BF03-2 升級為 P0，經本 PR 的 CR-BF01-1 承載。** 理由：
1. PM 約束與 patrol-13 執法規則為 categorical：BF-01 引入了會話內背景 pool 突變 goroutine（超時路徑），
   升級條件成立，無裁量降級空間；BF-19 未落地，不满足豁免前提。
2. P0 定義（並發安全問題）覆蓋真實可達的 data race，窗口窄不改變定性。
3. 內核 reader-goroutine-bind-initialize-ctx  workaround 本身**技術正確**
   （jsonrpc.go:138 `startReader(ctx)` 綁定首次調用 ctx；timeout ctx 會在 boot 後撕掉連接——已核實），
   但「長生命週期 ctx + 超時遺棄」的組合把内核限制轉化成了產品側並發違規。
   正確姿势是讓所有 pool 寫入在 boot 返回前終結（見 CR-BF01-1 修復方向），
   内核側根治（reader 不綁 handshake ctx、httpTransport.Close 真正 teardown）屬跨倉債務，與 BF-19 併案。

**穩態部分維持 P1 潛在**：無自動重連/輪詢的事實予以確認；若 CR-BF01-1 修復後 boot 期外再無 pool 寫者，
CR-BF03-2 回落為 BF-03 側的約束文檔項（其 GoDoc 約束已寫，bf-tui-b1 mcp.go:16-22,45-49），根治仍待 BF-19。

---

## 6. MCP Semantics

- **AdaptToTool 復用**：✅ 零 kernel 複製；轉發 raw name + args verbatim（`TestProxyTool_CallForwardsToServer`
  斷言 stub 收到 `echo`/`{"x":"hello"}`）；上游 isError、RPC error、invalid JSON、cancelled ctx 均為誠實 IsError 結果。
- **per-server 失敗非致命**：✅ ListTools 錯誤 `%w` 包裝 + errors.Join，不阻塞其餘 server（有測試）。
- **名稱順序**：✅ server 按名排序；資源聚合同样按名排序（`TestListMcpResources_ListsAllConnectedServers` 斷言 alpha→beta）。
- **.mcp.json 批准門 parity**：✅ 選擇集合 = `store.ListMCPServers()`（與 `mcp list` 同源：
  user→project→local 去重、剔除 rejected/不可 list 類型、winning scope local>user>project，mcpstore.go:436-478）；
  批准判定用 `GetMCPServer` winning scope == project + `EnableAllProjectMCP`/`EnabledMCPServers`
  （JSON key 與 loader.go:58-59 `enableAllProjectMcpServers`/`enabledMcpjsonServers` 一致）。
  與 `mcpListStatus`（mcp_list.go:76-86）的 winning-scope 判定同構。ws 條目被 ListMCPServers 排除（內核 WS 桩，BF-16），typeless 按 stdio 處理（與 health checker 一致）。_pending_ 項目不連接、不報錯（有測試）。
- **資源工具錯誤路徑**：✅ nil pool / 空 pool / 未知 server / 未連接 server / 上游失敗 / 必填缺失 / invalid JSON /
  cancelled ctx 全覆蓋；二進制返回 mime+base64 佔位說明（誠實、不洩漏二進制）。
- **遺留小項**：聚合輸出的 partial-failure 分支（entries>0 且 errs>0 的 Note 附錄）無測試；
  `callContext(nil)` 分支無測試（覆蓋率 66.7%）；見 CR-BF01-4。

## 7. Sleep（BF-02）

✅ 達標。`time.NewTimer` + `select` 三路（timer.C / Ctx.Done / AbortCh），defer timer.Stop()；
存在性校驗先于類型（正確處理 encoding/json 缺失填零 vs 顯式 0ms）；非負校驗；
取消/超時/中止均 IsError 誠實結果（帶請求時長與原因）；nil uctx 回退 Background；
0ms 立即成功。測試覆蓋完成/零值/nil uctx/四種非法輸入/預取消/中途取消/deadline/AbortCh，
時間斷言寬鬆有度（≤1s）。schema 聲明 required + minimum:0 與校驗一致。
邊界小項：`time.Duration(in.Milliseconds) * time.Millisecond` 在 >~9.2e12 ms（~292 年）時溢位為負 →
timer 立即觸發並謊報成功（CR-BF01-5，P2）。

---

## 8. Findings

### CR-BF01-1 — P0 — 看門狗超時遺棄 connect goroutine：會話內背景 pool 突變（CR-BF03-2 升級承載項）
- **證據**：`internal/bootstrap/mcp_connect.go:76-91`——`go func(){ done <- pool.Connect(context.Background(), …) }()`
  在 `<-timer.C`（:86）/`<-ctx.Done()`（:88）時直接返回，goroutine 越過 boot 存活；
  遲到完成時 agtkeel `Pool.Connect`（pool.go:87-97）在會話期間寫 `conn.Status/Client/Error`。
  無同步讀取側：BF-03 `FormatMCPStatus`（bf-tui-b1 commands/mcp.go:70）、本 PR `serverClient`
  （internal/tools/mcp/mcp.go:90）。註釋 mcp_connect.go:71-75 承認並依賴遲到落地，
  且「next RegisterServerTools pass / after a /mcp reconnect」表述與釘住模型矛盾。
- **違反**：PM-FOLLOWUP 追加約束（禁止會話內背景 pool 突變 goroutine，BF-19 未落地）；
  patrol-13 §11 執法規則；BF-03 CR-BF03-2 升級條件。裁決詳見 §5。
- **修復方向**（本仓可完成，不需改内核）：
  1. 改為**有界且不遺棄**：以 timeout ctx 調用 `pool.Connect`，超時/取消後**等待 goroutine 收尾再返回**
     （cancellation 經 http `NewRequestWithContext` / stdio `Recv` 的 ctx.Done / `call()` select 傳播，
     dialServer 失敗路徑會 `transport.Close()`，收尾有界）。所有 pool 寫入在 boot 返回前終結。
     代價：>30s 握手在 boot 記為 failed（/mcp 可見）——在當前內核下這類握手本就無法經有界 boot
     獲得會話級穩定連接（reader 綁 handshake ctx），且遲到工具因釘住不會被本會話註冊，故代價真實而收益虛幻。
     於 GoDoc/CONTEXT.md 如實記錄該取捨；内核根治（reader ctx 解綁 + Close teardown）登記跨倉債務（與 BF-19 併案，見 CR-BF01-7）。
  2. `mcpConnectTimeout` 改為可注入（var 或參數），補**回歸測試**：慢 initialize stub（延遲 > 注入超時）→
     斷言 boot 有界返回、pool 記錄已終態（failed）、返回後再無 pool 突變（等待觀察窗口後記錄不變）。
  3. 修正 mcp_connect.go:71-75 註釋（刪除「next pass / /mcp reconnect」錯誤表述，改為下個會話生效或失敗如實記錄）。
- **初始狀態**：❌ 待修復

### CR-BF01-2 — P1 — boot 期 `ListTools` 無超时：hung server 可無限阻塞啟動
- **證據**：`wire.go:125,231` 傳 `context.Background()` → `mcp_connect.go:60` →
  `mcp.go:57 conn.Client.ListTools(ctx)` 無 deadline。30s 看門狗只覆蓋 Connect（initialize），
  不覆蓋 tools/list。完成握手但對 tools/list 永不回應的 server 會讓 BuildContainer 永久掛起。
  對照：BF-03 /mcp 路徑對 ListTools 有 2s per-server 上限（bf-tui-b1 commands/mcp.go:35）——boot 更應有界。
- **修復方向**：RegisterServerTools 內對每個 server 的 ListTools 加 per-server timeout ctx
  （建議 5-10s，可與看門狗同量級），超時計入 errors.Join 非致命；補慢 tools/list stub 測試。
- **初始狀態**：❌ 待修復

### CR-BF01-3 — P2 — 釘住守護可補強 + 包級狀態衛生文檔化
- **證據**：
  1. `TestRegisterServerTools_PinnedAcrossReconnect` 未反向證明「live tool list 確實變了」——
     若 stub/Reconnect 靜默失效，測試仍會綠。建議追加：重連後向**新 registry** 註冊，
     斷言新表面（brand-new 工具、新 schema）出現——同時證明差異存在與 per-registry 語義。
  2. `pinnedRegistries`（mcp.go:21）永不 prune、無 reset API；「失敗即釘住、本會話不可恢復」
     未在 RegisterServerTools GoDoc 明示。
- **修復方向**：補 fresh-registry 對照斷言；GoDoc 补「registry 生命週期 = 會話；標記進程生命週期保留；
  註冊失敗亦釘住」三句。可選：測試用 reset helper（僅 test export）。
- **初始狀態**：❌（可選）

### CR-BF01-4 — P2 — 資源工具測試/輸出缺口
- **證據**：
  1. partial-failure 聚合分支（mcp.go:199-203，entries>0 且 errs>0 的 Note 附錄）無測試；
  2. `callContext(nil)`（mcp.go:72-77，覆蓋率 66.7%）與 `ReadMcpResource` empty-content 分支（mcp.go:299）無測試；
  3. `ListMcpResources` 聚合輸出無上限（BaseTool.MaxResultSizeChars=-1，engine 不消費該值）——
     資源極多的 server 可產生大 tool result（cache gate Q2 觀察項）。
- **修復方向**：補雙 server 一敗一成、nil uctx、empty contents 用例；可選：對聚合輸出加显式條數/字節上限
  （超限截斷 + 誠實提示），與 window-small 原則一致。
- **初始狀態**：❌（可選）

### CR-BF01-5 — P2 — Sleep 時長溢位邊界
- **證據**：`internal/tools/misc/misc.go:226` `time.Duration(in.Milliseconds) * time.Millisecond`：
  int 輸入 >~9.2e12 ms（~292 年）時 Duration 乘法溢位為負，NewTimer 立即觸發，謊報「Slept for N ms」。
- **修復方向**：校驗上限（如 ≤ math.MaxInt64/int64(time.Millisecond)，或產品級上限如 24h）並誠實拒絕；補一個溢位輸入用例。
- **初始狀態**：❌（可選）

### CR-BF01-6 — P2 — 過時/矛盾註釋 + 提交說明三問形式不全
- **證據**：
  1. `wire.go:210` BuildHeadlessContainer GoDoc「No MCP connections are pre-established」已過時
     （其實際委託 BuildContainer，現在會連接）；
  2. mcp_connect.go:71-75 遲到落地註釋與釘住模型矛盾（已併入 CR-BF01-1 修復項 3）；
  3. 提交说明陳述了釘住決策，但未以門控三問形式显式回答 Q2（窗口）/Q3（調用）——本報告 §4 已代為核驗記錄。
- **修復方向**：修正 headless GoDoc；後續提交按三問模板寫 cache 說明。
- **初始狀態**：❌（可選）

### CR-BF01-7 — P2 — 內核繼承限制（跨倉登記，非本 PR 代碼缺陷）
- **證據與說明**（均經 agtkeel 源碼核實；BF-01 按任務要求復用內核，不承擔修復責任）：
  1. **HTTP teardown 失效**：`httpTransport.Close()`（transport.go:440-445）僅置 `closed` 旗標，
     Send/Recv 從不檢查；reader goroutine 綁 Background ctx 後，Reconnect/Disconnect 無法真正撕掉
     舊 GET long-poll（舊 reader 洩漏至 server 關流）；container 亦無 pool shutdown 鉤子。
  2. **reader 綁 handshake ctx**（jsonrpc.go:138）：本次 workaround 的根因；內核應改為 connection-scoped ctx。
  3. **InputSchema 保真度**：adapter.go:57-71 僅保留 properties/required，丟棄 additionalProperties/$schema 等
     （字節穩定但保真受限）。
  4. **命名 parity**：`AdaptToTool` 產出 `{server}__{tool}`，註釋自稱匹配 TS `buildMcpToolName()`，
     但 TS oracle 慣例為 `mcp__{server}__{tool}`——影響權限規則字串與 oracle parity，需跨倉裁決。
  5. **nil-uctx 不對稱**：`mcpTool.Call` 直接解引用 `ctx.Ctx`（adapter.go:113），nil UseContext 會 panic；
     本倉資源工具則容忍 nil（callContext）。engine 契約要求非 nil（engine.go:47），今日不可達，記錄備查。
- **處置**：由 PM/Tech Lead 登記 agtkeel 跨倉任務（與 BF-19 快照 API 併案規劃）；**不退回 Agent-Tools 改本倉代碼**。
- **初始狀態**：❌（跨倉跟進）

---

## 9. Test Quality

**httptest stub 設計（mcp_test.go:25-210）——高質量**：
- 真實 Streamable-HTTP 語義：POST→dispatch→**202 + 響應入 session 隊列**，GET long-poll SSE 取響應——
  精確匹配內核 httpTransport「Send 丟棄 POST body、Recv 走 GET」的實現（transport.go:377-438），
  若 stub 誤用 POST-body 回應會掛死，此處設計正確。
- **session-id 路由**：initialize 無 session header 時鑄新 sid，重連隔離響應隊列（釘住測試依賴此隔離）。
- **cleanup 順序正確**：`close(s.done)` 先釋放 long-poll GET，再 `srv.Close()`
  （httptest.Close 等待活躍請求）——避免了 GET 阻塞導致的 cleanup 掛起；未知 session 的 GET hold 至 done 而非空轉。
- 共享字段（toolDefs/failMethods/lastCall*）均在 `s.mu` 下讀寫；`-race` 全綠。
- bootstrap 側 wiringStubServer 同構（單連接場景用共享 respCh，足夠）。

**覆蓋面**：轉發/上游 isError/RPC error/invalid input/cancelled ctx、資源 list/read 全錯誤路徑、
空池/nil 池、批准門 pending→approved（含跨 registry 釘住斷言）、連接失敗非致命（真實不存在 binary）、
Sleep 八路徑。釘住守護真實 Reconnect（§4）。

**缺口**：看門狗超時分支零覆蓋（77.8%，併入 CR-BF01-1 修復項 2）；boot ListTools 慢路徑（CR-BF01-2）；
partial-failure/nil-uctx/empty-content（CR-BF01-4）；釘住守護補強（CR-BF01-3）。

**覆蓋率宣稱複核（go test -cover，本次評審實測）**：
| 包 | 宣稱 | 實測 | 結論 |
|---|---|---|---|
| internal/tools/mcp | 89.5% | **89.5%** | ✅ 屬實 |
| internal/tools/misc | 81.5% | **81.5%** | ✅ 屬實 |
| internal/bootstrap | 76.7% | **76.7%** | ✅ 屬實 |

---

## 10. Conventions

| 檢查 | 結果 |
|---|---|
| gofmt（本 PR 触及文件/包） | ✅ `gofmt -l` 零輸出（HEAD 既有他層未格式化文件不記本 PR，BF-20 跟進） |
| 函數 ≤80 行 | ✅ 新增最長 63 行（listMcpResourcesTool.Call）；RegisterServerTools 28、connectWithWatchdog 16、sleepTool.Call 39 |
| 文件 ≤800 行 / 測試檔 ≤1600 行 | ✅ mcp.go 302、mcp_connect.go 108、wire.go 797（逼近上限，後續注意）；mcp_test.go 881 |
| 錯誤處理 | ✅ ListTools/connect 錯誤 `%w` 包裝 + errors.Join；tool 層上游錯誤按慣例轉 IsError 結果；marshal 硬錯誤 `%w`；無 `_ = fn()` |
| GoDoc | ✅ 新增導出符號全有文檔（含釘住決策長註釋）；包註釋更新 |
| import 排序 | ✅ 三組分明；wire.go 順帶修正基線混排 |
| 共享狀態註明 | ✅ pinnedRegistries 有語義註釋；stub 字段 mu 保護 |

---

## 11. Doc Sync

- `tools.md` v1.0→**v1.1**：頭部版本/日期、§3.1 MCPTool 行、§4.4 全節重寫（簽名+釘住+接線）、
  §6 註冊示例、§7.9 Sleep、文末變更記錄表——符合 doc-sync-policy Step 1-3。✅
- `cli.md`：BuildContainer 節新增 MCP 启动接線注記（pool 先建、批准門、看門狗、非致命）。✅
- CONTEXT.md Design Notes：mcp（接線決策 + **cache-stability 釘住決策**，PM 約束 #5 達標）、
  misc（Sleep 設計）、bootstrap（簽名變更 + 內核 ctx 約束 + 看門狗取捨）均手寫補充；
  docgen 部分（簽名/結構體計數/INDEX）自動重生成。✅
- `make docs-check`：**docs are up to date**。✅
- 缺口：CR-BF01-6（headless 過時 GoDoc）；CR-BF01-1 修復後須同步更新 bootstrap CONTEXT.md 的看門狗取捨描述。

---

## 12. Verification Runs（本次評審實測，worktree bf-tools-b1 @ 1732d65）

| 檢查 | 結果 |
|---|---|
| `go build ./...` | ✅ 通過 |
| `go vet ./...` | ✅ 通過 |
| `CGO_ENABLED=1 go test -race -count=1 ./...`（全倉 fresh，非 cached） | ✅ **21/21 有測試包全 ok**（gcc 就位） |
| `go test -race -count=1 -v -run 'TestRegisterServerTools\|TestSleepTool'`（mcp/misc） | ✅ 全 PASS（含釘住守護） |
| `go test -cover`（mcp / misc / bootstrap） | ✅ 89.5% / 81.5% / 76.7%（與宣稱精確一致） |
| `make docs-check` | ✅ docs are up to date |
| `scripts/debt-check.sh` | ✅ 欠账红线檢查通過；實測桩 22 / TODO(dep) 44 = 基線（與實現同提交） |
| `gofmt -l`（触及文件） | ✅ clean |
| 層級依賴（go list） | ✅ tools/mcp、tools/misc 僅 stdlib + agtkeel |

**環境備註**：worktree 中 `docs/project/log.md` 有一行未提交 PM 巡檢索引改動、
PM-FOLLOWUP/參考文檔副本為 untracked——均按指示**不納入本次評審提交**，留待 PM/主工作區處理。

---

## 13. Verdict

- **P0 列表（1 項，阻擋合併）**：
  - **CR-BF01-1**：看門狗超時遺棄 connect goroutine → 會話內背景 pool 突變。
    **CR-BF03-2 升級裁決成立（P1→P0）**：PM 追加約束與 patrol-13 執法規則的升級條件精確命中；
    BF-19 未落地，無豁免。修復方向：有界且不遺棄的 boot 連接（timeout ctx + 收尾等待）+
    可注入超時 + 慢握手回歸測試 + 註釋修正。
- **P1（1 項）**：CR-BF01-2（boot ListTools 無超时，啟動可被無限阻塞）。
- **P2（5 項，記錄跟進）**：CR-BF01-3（守護補強+狀態衛生）、CR-BF01-4（資源工具測試/輸出缺口）、
  CR-BF01-5（Sleep 溢位）、CR-BF01-6（過時註釋+三問形式）、CR-BF01-7（內核繼承限制，**跨倉登記，不退回 Agent-Tools**）。

**評審門控：NOT pass-ready。** 依 stub-backfill-plan §2.3（P0 全關閉）與 tech-lead SOP，
本提交須先關閉 CR-BF01-1（P0）與 CR-BF01-2（P1）方可進入 QA。
其余維度——架構鐵律、cache-first 釘住機制與守護測試、MCP 語義、Sleep、慣例、文檔同步、
全部驗證運行（含全倉 fresh -race）——均合格；修復預期是局部重work（mcp_connect.go 看門狗 +
RegisterServerTools 超时 + 對應測試），不影響已驗證的釘住模型與工具層實現。
由 PM 退回 Agent-Tools；修復完成後 Tech Lead 按複核規範更新下方跟蹤記錄。

**給 PM 的備註**：
1. CR-BF03-2 在 BF-03 側的約束文檔（GoDoc）已到位；本裁決不新增 BF-03 工作量。
   CR-BF01-1 修復後，穩態 pool 無會話內寫者，CR-BF03-2 回落 P1 潛在（根治仍待 BF-19）。
2. 建議將 CR-BF01-7 的五項內核限制與 BF-19（快照式 GetAll）併案為 agtkeel 跨倉工作包，
   其中 reader-ctx 解綁與 HTTP Close teardown 是 BF-01 看門狗取捨的根治前提。
3. 合併順序預報（patrol-13 §12）不變：BF-01/02 先合、BF-03 rebase；本評審不改變該順序，
   但 BF-01/02 須先過修復複核。

---

## 修復跟踪記錄

| 問題編號 | 級別 | 描述摘要 | 狀態 | 複核時間 | 備註 |
|---------|------|---------|------|---------|------|
| CR-BF01-1 | P0 | 看門狗遲到落地＝會話內背景 pool 突變（CR-BF03-2 升級） | ❌ 待修復 | — | 有界不遺棄 + 回歸測試 + 註釋修正 |
| CR-BF01-2 | P1 | boot ListTools 無超时，啟動可被 hung server 阻塞 | ❌ 待修復 | — | per-server timeout + 測試 |
| CR-BF01-3 | P2 | 釘住守護 fresh-registry 補強 + 包級狀態衛生 GoDoc | ❌（可選） | — | |
| CR-BF01-4 | P2 | partial-failure/nil-uctx/empty-content 測試 + 聚合輸出上限 | ❌（可選） | — | cache Q2 觀察項 |
| CR-BF01-5 | P2 | Sleep Duration 溢位上限校驗 | ❌（可選） | — | |
| CR-BF01-6 | P2 | headless 過時 GoDoc + 提交三問形式 | ❌（可選） | — | mcp_connect 註釋併入 P0 修復 |
| CR-BF01-7 | P2 | 內核繼承限制（Close/reader-ctx/schema 保真/mcp__ 命名/nil-uctx） | ❌（跨倉跟進） | — | PM/Tech Lead 登記，與 BF-19 併案 |
