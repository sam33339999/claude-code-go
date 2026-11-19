# 常駐 coordinator＋多 thread＋fork prompt-cache 复用

> 日期：2026-11-18
> 分支：feat/coordinator-fork-cache
> 狀態：Phase 1 已落地（同 process 內常駐 coordinator＋可續話 thread＋cache-preserving fork）
> 來源：Anthropic 洩漏提示词 `/root/system_prompts_leaks/Anthropic/`（claude-projects-coordinator.md、
> claude-projects-thread-claude.md、claude-code/agents/fork.md、claude-code-opus-5.5.md Agent 工具段）

## 動機

使用者需求：像 Claude Projects coordinator 那樣常駐協調＋多 thread 做事；**fork 的核心動機是保住
LLM KV-cache（prompt cache）**——fork 出來的 agent 繼承父的請求前綴，走便宜的快取輸入，而非重新
讀取全部 context。

## 侦察發現的三個缺口（改動前）

1. **Anthropic 路徑完全未送 cache_control**：TUI 設了 `SystemPromptPart.CacheControl="ephemeral"`，
   但 `engine.buildRequestWithModel` 把 system 壓成純字串、`api.MessageRequest` 無任何 cache 標記
   欄位——prompt cache 從未寫入，cache_read 恆 0。
2. **Coordinator system prompt 未接生產**：`GetCoordinatorSystemPrompt` 只有測試在用；主會話
   實際只拿 CLAUDE.md＋記憶 reminder。
3. **Sub-agent 路徑**：`buildRunAgentFn` 未設 `NoWriteBack`（sub-agent 結束覆蓋主對話）、
   `CacheParams` 未接線、inbox 無人消費（`_ <-chan string`）、無歷史保留（SendMessage 無法續話）。

## 從 leak 抓到的設計要點

- **fork.md**：fork＝繼承父完整對話＋父 model（override 無效）＋背景執行＋one-shot＋禁止再 spawn；
  boilerplate 放尾部 user message（「transcript 是繼承的參考，不是你的處境」）；maxTurns 200。
- **opus-5.5 Agent 段**：fork **繼承完全相同的工具列表**（cache bytes 一致的關鍵——用 prompt 禁止
  再委派，而非從 schema 移除工具）；fresh agent 成本警示（「when in doubt, don't spawn」）；
  SendMessage 續話保持 context intact；禁止虛構 pending 結果；brief like a peer。
- **projects-coordinator.md**：不先問就開 thread；標題 3-6 字動詞開頭用使用者的話；instructions
  只給 context 不加 deliverables；對 thread 的話永遠是 coordinator 自己的話、非使用者批准；
  回報精簡 outcome-first；thread 狀態機（working/blocked/review_ready/done/failed/stopped）。

## Cache 命中原理（本次實作）

```
父請求：  [tools schemas＋末位斷點] [system blocks＋穩定段斷點] [歷史…最後 user msg 滾動斷點]
fork：   [完全相同的 tools]       [完全相同的 system]       [父歷史快照（turn 邊界）]＋[尾部 directive]
```

- 快照取於 turn 邊界（壓縮後、inbox drain 後、API call 前）——保證 tool_use/tool_result 序列合法
  （快照不含當前 turn 尚未回 result 的 tool_use）。
- fork 前綴與父 byte-identical（守護測試 `TestForkPrefix_ByteIdentity`）；父後續請求也能反讀
  fork 寫的共享前綴快取。
- cache_control 標記是 write-position metadata，provider 前綴匹配忽略其位置（滾動斷點原理）；
  守護測試比對訊息內容時排除標記。
- 斷點總數 4（system 1＋tools 1＋messages 1＋保留餘裕）；動態記憶 reminder 挂斷點之後（turn 尾部）。

## Phase 1 落地清單

| 層 | 改動 |
|----|------|
| pkg/types | ContentBlock +CacheControl（request-side only，歷史不帶標記） |
| internal/api | SystemBlocks＋MarshalJSON（system block 陣列）、ToolSchema/ContentBlock +CacheControl、SystemText()（OpenAI 路徑純文字、隱式前綴快取） |
| internal/engine | 確定性斷點佈局；SkipCacheWrite 抑制；turn 邊界快照 CacheSafeParams（+Model）至 UseContext.CacheParams；QueryParams.InboxCh turn 邊界 drain（<coordinator-message> 包裝） |
| internal/tools | UseContext +CacheParams any |
| internal/agenttype | AgentTypeFork（builtin 5→6）；profile SystemPrompt/ToolFilter 刻意空（繼承父）；ForkBoilerplate＋FormatForkDirective |
| internal/tools/agent | fork 強制 background＋model 忽略＋CacheParams 轉發；Agent/SendMessage description 依 leak 重寫 |
| internal/bootstrap | 全 sub-agent NoWriteBack（修污染）；buildForkQuery（父前綴＋尾部 directive、完整工具、繼承 model、background retry 語義）；runStates 保留 per-agent QueryParams 供 resume |
| internal/coordinator | SendMessage 重開已結束 thread（resumeAgent 等 loopDone）；enqueue＋exit-check 原子化＋有界 continuation 防訊息擱淺；killed 狀態保留（修 TestAdapterStopAgent flake 根因）；系統提示重寫 |
| internal/tui | coordinatorMode 時注入 coordinator prompt 至穩定前綴；單一斷點掛穩定段末尾 |

測試：engine cache_breakpoints_test.go（含 fork 前綴守護）、api cache_test.go、coordinator
resume_test.go、bootstrap wire_fork_test.go、tools/agent fork 語義×2、tui coordinator_prompt_test.go×2。
全量 -race／vet／gofmt／docs-check／debt-check 全綠。

## Cache-first 閘門三問

1. **是否移動前綴？** cache_control 標記不改文字前綴。coordinator prompt 注入＋Agent/SendMessage
   description 更新為**一次性**前綴變更（新會話基線，會話內釘住）；fork 前綴與父 byte-identical。
2. **是否增大窗口？** 否——fork 繼承既有歷史，標記不增 token。
3. **是否新增呼叫？** fork 是新增背景呼叫，但其前綴走 cache read（便宜輸入）——此即本變更目的。
   529 時 background QuerySource 即放棄（避免容量級聯）。

## 實機驗證修復（2026-11-18，fix/agent-lifetime）

使用者實機跑委派「一直失敗」。根因與修復：

1. **Agent 生命週期綁死父 query ctx**（主因）：`SpawnAgent` 以 caller ctx（＝父 query 的
   tool-call ctx）為 agent ctx 父源。父 turn 結束時 `Query` 的 `defer cancel()` 觸發；TUI 在使用者
   送下一訊息時也會 abort 前一 query——background agent/fork 隨即被取消。**fork 強制 background，
   所以每次 fork 都在父 turn 結束時死亡**。修復：`context.WithoutCancel(ctx)` 為 parentCtx
   （保留 values、去除 cancellation），thread 存活至完成或 StopAgent（Projects thread 語義）。
2. **Interrupt slot 被 background query 劫持**：`engine.Query` 無條件覆蓋 `e.abortFn`，
   sub-agent query 跑在同一 engine 上會使 Ctrl+C 取消到 background query 而非主對話。
   修復：僅 write-back（foreground）query 擁有 interrupt slot。

守護測試：TestSpawnAgent_SurvivesParentContextCancel、TestSendMessage_ResumeAfterParentContextCancel、
TestInterrupt_NotHijackedByBackgroundQuery。

### CWD 事實（使用者提問）→ 已修正（feat/agent-cwd）

原狀：**委派無 per-agent CWD 設定**——Bash 不設 `cmd.Dir`、fileops/glob/grep 用 `os.Getwd()`，
sub-agent 與主會話共用 process CWD。且 EnterWorktree 只改 AppState.WorkingDir 不 chdir，
主會話進 worktree 後工具也仍在舊目錄。

修正：`tools.UseContext.WorkingDir`（per-call 會話工作目錄）——Bash cmd.Dir、fileops
expandPathFrom、Glob/Grep 預設目錄皆讀它；TUI／headless／buildRunAgentFn 三條接線從
AppStateStore live 讀注入（buildRunAgentFn 改收 store，model 也 live）。空值 fallback
process CWD（向後兼容）。sub-agent 繼承會話目錄；**per-agent 獨立 worktree isolation**
（leak 的 isolation:"worktree"）仍列 Phase 2。
另確認 PermCtx=nil 安全（所有 CheckPermissions override 忽略 UseContext；checker 用自身快照＋ModeFn）。

## 實機驗證紀錄（2026-11-19，編譯產物 bin/claude @ 09fff1b）

環境：CLAUDE_PROVIDER=openai、OPENAI_BASE_URL=token-plan maas（qwen4-max）、
OPENAI_API_KEY／OPENAI_MODEL／CC26_MODEL 已設定。全數通過：

| # | 驗證 | 命令／情境 | 結果 |
|---|------|-----------|------|
| 1 | 基本連線 | `-p "Reply with exactly: OK"` | `OK` |
| 2 | CWD | 於 /tmp/opencode/ccg-live-test 跑 Bash pwd | 輸出該目錄（WorkingDir 接線生效） |
| 3 | 同步委派 | Agent worker（background false）跑 pwd 回报 | `DELEGATED-OK /tmp/opencode/ccg-live-test`（sub-agent CWD 繼承 ✓） |
| 4 | 背景委派 | background worker 跑 sleep 3；父 sleep 6 後 GetAgentStatus | `completed` / `BG-SURVIVED`（生命週期修復生效） |
| 5 | Cache 可觀測 | 同會話兩 turn（-c），stream-json | turn1 `InputTokens 7186 / CacheReadTokens 7168`；turn2 `7214 / 7168`——穩定前綴 ~99% 走快取，動態尾部新鮮 |
| 6 | Fork 上下文继承 | 父記住 ZX-9971 → spawn fork 問代碼 → 父 sleep 8 → GetAgentStatus | `afork-…` `completed` / `FORK-SAW ZX-9971`（fork 繼承歷史＋背景存活 ✓） |

解讀：CacheReadTokens 7168＝system＋tools 穩定前綴被 provider 快取命中（OpenAI 相容
路徑隱式前綴快取）；fork 請求前綴與父 byte-identical（守護測試），故 fork 的輸入同樣
走快取讀取——使用者 KV-cache 動機成立。OpenAI prompt_tokens 含 cached，命中數由
CacheReadInputTokens 獨立報告（feat/openai-cache-usage 補上此前恆 0 的缺口）。

## Phase 2（未落地，後續任務）

> **狀態（2026-11-19 使用者確認）：無急迫性，全部記錄為 backlog、不排程。**
> 其中項目 2（wake 產品層）與項目 3（worktree isolation）使用者明確指示「先記錄文件」。
> 觸發條件建議：需要平行多個寫入 agent → 項目 3；需要關機再開續話 → 項目 1；
> 需要常駐專案管家介面 → 項目 2；Anthropic 直連且續話間隔 >5m → 項目 4。

- **跨 process 存續**：Projects thread 可跨 coordinator 替換存活（project-activity wake 帶摘要）。
  目前 goroutine 隨 process 死；需接 internal/session＋transcript 持久化＋resume（runStates 已留
  per-agent 歷史，是持久化的天然起點）。
- **Wake 驅動產品層**：Projects coordinator 是「每 turn 一個 wake 事件」的事件迴圈＋project chat。
  本倉 EV-01 MCP 事件喚醒＋msgqueue 已打底；缺「project chat＋thread 清單＋project memory」產品層
  （memory 可對應 internal/memdir；thread 狀態機 working/blocked/review_ready 需擴充 AgentStatus）。
- **1h cache TTL**：leak 提及 session 級 1-hour TTL（extended-cache-ttl）；目前未送 ttl 參數，
  預設 5m。可於 api.CacheControl 加 TTL 欄位＋session 設定。
- **fork gate**：leak 的 fork 是 gated feature（「when the fork gate is on」）；本倉直接開放
  agent_type=fork。若需對齊 oracle 預設行為（fork 非預設、需顯式選擇），現狀已符合（預設 worker）。
- **Worktree isolation**：leak 的 Agent 工具有 `isolation: "worktree"`（agent 在獨立 git worktree
  工作，未變更自動清理）。目前 sub-agent 共用 process CWD，寫入任務可能互相衝突——coordinator
  prompt 已建議寫入任務串行，但 isolation 是更徹底的解法。本倉 EnterWorktree 工具（BF-10）可以為基。

## 已知局限

- fork 無父快照時退化為 boilerplate-as-system-prompt 的 one-shot agent（無 cache 复用，仍 one-shot）。
- sub-agent 的 HIL 權限請求目前共用主 checker；併發 prompt 的 AgentID 路由（agentctx 已有）待 TUI 側驗證。
- runStates／agents map 隨 process 生命週期增長，無清理（與既有 coordinator 行為一致）；持久化落地時一併處理。
