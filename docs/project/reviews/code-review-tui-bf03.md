# BF-03 Code Review — /mcp + /resume Slash Commands (TUI/commands)

> **Reviewer**: Tech Lead Agent
> **Date**: 2026-11-18
> **Subject**: BF-03 桩回填 — /mcp、/resume 真实实现（Agent-TUI）
> **Branch**: `feat/bf-03-slash-mcp-resume` · **Commit**: `286e0c7` · **Diff base**: `db88b72`
> **Worktree**: `/root/.local/share/opencode/worktree/fcc669/bf-tui-b1`
> **Verdict**: **APPROVED — 评审门控通过（P0 = 0）**；2 项 P1 退回 Agent-TUI 修复（合并前或紧随的修复提交），4 项 P2 记录跟进。

评审依据：`AGENTS.md` 铁律与编码规范、`docs/project/agents/tech-lead-agent.md` SOP、
`docs/project/design/tui.md` v1.1（本 PR 更新）、`docs/PROMPT_CACHE_AND_COST.md`（cache-first 门控，
2026-11-18 正式采纳）、`docs/project/discussions/2026-11-18-stub-backfill-plan.md` §2 验收门控、
`docs/project/doc-sync-policy.md`。

---

## 1. Overall Assessment

实现质量高，范围克制。两处用户可见桩被真实实现替换（−2 桩，基线 26→24 与实现同提交刷新，
`scripts/debt-check.sh` 通过）：

- **/mcp**：Execute 只置 `Result.FetchMCPStatus` 标志，报告经 `fetchMCPStatusCmd → MCPStatusMsg →
  Update` 异步生成（仿 `TriggerDream` 先例且更干净——只捕获注入的 pool 接口，不捕获模型指针）。
  每 server `ListTools` 2s 超时 + 整体 15s 上限，失败降级，nil/空池友好提示，按名排序保证确定性输出。
- **/resume**：裸命令列出最近 10 个会话（id/mtime/消息数/首问片段），`/resume <id>` 经 agtkeel
  `session.Resume` 加载 user/assistant 历史，ID 路径穿越校验，损坏条目跳过，与 CLI `--resume`
  路径（`internal/bootstrap/session.go`）语义逐行对齐但在 commands 层独立实现，未 import bootstrap。
  TUI 侧经既有 `engine.SetMessages` 替换历史（无新引擎契约），采纳 `NewSessionID`，
  **查询进行中（isLoading）明确拒绝**。

架构铁律全部守住（§2）；cache-first 三问与实际 diff 核对一致（§4）；MVU 合规（§5）；
文档同步完整（§8）；全部验证运行通过，**含全仓 -race**（gcc 已就位，§9）。
覆盖率宣称经复核属实：commands 75.6%、tui 57.7%。

无 P0。两项 P1：`applyCommandResult` 函数超长（107 行 > 80 行规范）；`FormatMCPStatus`
对池内共享连接字段的无同步读取构成**潜在**并发隐患（当前接线无并发写者，-race 全绿，
但 BF-01/重连特性落地前必须解决或钉住约束）。

---

## 2. Architecture / Layering

**结论：合规。唯一的跨层触点是 sanctioned 的最小接线。**

| 检查项 | 结果 | 证据 |
|---|---|---|
| commands/tui 不 import bootstrap / internal/tools | ✅ | `go list -deps ./internal/commands ./internal/tui` 内部依赖仅 commands/coordinator/memdir/state/tui，无 bootstrap、无 internal/tools |
| commands 新增依赖方向 | ✅ | 仅向下依赖 agtkeel 内核库 `mcp`/`session`/`types`/`utils/fs`（第三方内核，合法） |
| `pkg/types` 零外部依赖 | ✅ | 本 PR 未触及 |
| 工具间互依赖 | ✅ | 本 PR 未触及 tools 层 |
| bootstrap 一行接线 | ✅ sanctioned | `internal/bootstrap/root.go:234` 仅在 `runInteractive` 的 `tui.New(...)` 追加 `container.MCPPool` |

**裁决（bootstrap 触点）**：CLI 层拥有装配职责（`AppContainer` 为顶层连线点，AGENTS.md 设计模式），
`runInteractive` 向 TUI 构造器注入依赖是**向下**的 DI，与既有 `memoryStore` 注入完全同构。
一行、无逻辑改动、CONTEXT.md Design Notes 已记录。**接受，不构成违规。**

**裁决（`commands.MCPServerLister` 接口）**：
- **最小性** ✅：单方法 `GetAll() []*mcp.ServerConnection`，只读视图，恰好覆盖 /mcp 所需；
  未引入 Connect/Reconnect 等写能力，符合最小权限注入。
- **可假性** ✅：commands 与 tui 测试各自用 ~5 行 fake 实现（`fakePool`/`fakeMCPPool`），
  无需真实网络。
- **命名** ✅：单方法接口 `-er` 结尾，符合规范。
- **契约钉住** ✅：`var _ MCPServerLister = (*mcp.Pool)(nil)` 编译期断言（mcp.go:21），
  agtkeel 侧漂移会在编译期暴露。
- **保留意见**：`GetAll` 返回的是**活指针**而非快照，见 CR-BF03-2。接口本身接受；
  快照语义的根治在 agtkeel 侧（跨仓），需经 PM/Tech Lead 走跨仓变更流程。

---

## 3. Design vs Implementation Delta

| 设计文档约定（tui.md v1.1 §4.1） | 实现状态 | 说明 |
|---|---|---|
| `/mcp` Execute 只返回 `DisplayNone + FetchMCPStatus` | ✅ 一致 | builtins.go:300-312 |
| `fetchMCPStatusCmd` 在 Update 循环外渲染，经 `MCPStatusMsg` 回 Update | ✅ 一致 | cmds.go:240-246、update.go:266-271 |
| 每 server 2s / 整体 15s 超时，失败降级 | ✅ 一致 | mcp.go:25（2s）、cmds.go:235（15s）、mcp.go:96-99 降级 |
| nil/空池友好提示 | ✅ 一致 | mcp.go:35-41，测试覆盖 |
| `/resume` 裸列表 10 条、mtime 倒序、60 rune 片段 | ✅ 一致 | resume.go:17-21、36-97 |
| ID 路径穿越校验（拒 `/`、`\`、`..`） | ✅ 一致 | resume.go:122-124，4 个攻击向量测试 |
| `NewMessages []types.Message` 断言消费 + `SetMessages` + 采纳 `NewSessionID` | ✅ 一致 | update.go:417-433 |
| isLoading 拒绝恢复 | ✅ 一致 | update.go:418-423 + 测试 |
| /clear 遗留 `[]interface{}{}` 断言失败被忽略、行为不变 | ✅ 一致 | update.go:417 断言门 + 回归测试 |
| `tui.New`/`newAppModel` 末尾新增 `mcpPool` 参数（可 nil） | ✅ 一致 | init.go:39、model.go:167 |
| bootstrap 唯一调用点一行接线 | ✅ 一致 | root.go:234 |
| 已知局限：交互模式不落盘 JSONL | ✅ 已记录 | §4.1 末段；`m.sessionID` 仅用于 CommandContext 与 compact 记忆保存，无落盘写者，采纳语义安全 |
| 交互池在 BF-01 前为空（/mcp 显示 "No MCP servers configured"） | ⚠️ 未显式记录 | 见 CR-BF03-5（P2） |

**doc-sync-policy 符合度**：接口签名变更（`tui.New`）、新增公开类型
（`MCPServerLister`、`FormatMCPStatus`、`MCPStatusMsg`）、新增字段
（`Result.FetchMCPStatus`/`NewSessionID`）、新增依赖均已同步至 tui.md v1.1（头部版本号 +
§4.1 + 文末变更记录表），符合 Step 1-3 全部要求。§4 初稿契约与实际 `Result` 驱动模式的
差异以 §4.1 头部注记显式声明（"实际生效契约以 registry.go 为准"），处理得当。

---

## 4. Cache-first Gate（三问核验）

提交说明已按门控回答三问；以下为**对照实际 diff 的独立核验**：

### Q1 — 是否移动前缀？ **否（会话边界除外，sanctioned）**
- /mcp：报告仅作为系统消息进入 `m.messages`（显示历史，update.go:266-271）。
  **显示历史与引擎历史分离**：`startQueryCmd` 从 `queryEngine.GetMessages()` 构建请求
  （cmds.go:50），不读 `m.messages`；`MCPStatusMsg` 分支不调用任何引擎方法。
  报告永不进入 system prompt 或工具 schema。diff 中无任何 schema/注册/序列化触点。
- /resume：替换引擎历史 = 前缀内容变化，但属**会话边界**操作（与 CLI `--resume` 同一
  sanctioned 语义），且 `isLoading` 时拒绝（update.go:418-423，
  `TestSlashResume_RejectedWhileLoading` 钉住：拒绝时引擎历史与 sessionID 均不变）。
  会话内无中途前缀重写；压缩仍是唯一被认可的前缀重置。恢复确认文案只进显示历史
  （引擎 2 条 vs 显示 3 条，`TestSlashResume_LoadsHistoryIntoEngine` 钉住），不污染前缀。
- 恢复后后续轮次经引擎正常 append，prepend-only 得到遵守。

### Q2 — 是否增大窗口？ **否**
无 token 上限/上下文窗口改动。/resume 加载的是既有会话的持久化历史（本就要发送的内容，
非新增投影）；/mcp 报告是有界显示文本（每 server 一行），不进请求。

### Q3 — 是否新增调用？ **否（无新增 LLM 调用）**
/mcp 发起的是**只读 MCP `ListTools` 协议请求**（非 LLM 调用），每 server 2s、整体 15s 有界，
不注册/增删/改名任何工具，不改 schema 字节——不触及"会话内工具集合与 schema 字节钉住"规则。
/resume 纯文件 I/O，零网络、零 LLM 调用；加载的历史仅在**下一次用户发起的查询**时发送。

### 守护测试
变更面附近的聚焦守护已部分到位：isLoading 拒绝测试（前缀不被中途打破）、
引擎/显示分离测试（显示文本不入前缀）、/clear 回归。缺口见 CR-BF03-6（P2）：
建议补一条 "/mcp 后引擎历史不变" 的显式断言。本仓尚无 `scripts/cache-guard.sh`
等释放级守护（BF-17 待落地）；本变更非 prefix-bearing，不要求释放级运行。

**Cache gate 结论：通过。** 三问答案与 diff 事实一致，无动态状态泄漏进 schema 或前缀。

---

## 5. MVU Compliance

**结论：合规，且优于既有先例。**

- 链路完整：`Result.FetchMCPStatus` → `applyCommandResult`（update.go:439-441）→
  `fetchMCPStatusCmd(m.mcpPool)`（tea.Cmd）→ `MCPStatusMsg` → `Update` case（update.go:266-271）
  追加系统消息 + `syncViewportContent`。
- **副作用零状态突变**：`fetchMCPStatusCmd` 只捕获注入的 pool 接口值，不捕获模型
  （对比既有 `triggerDreamCmd(&m)` 的模型指针捕获先例——新代码更干净）；
  全部模型变更发生在 Update 循环内的值拷贝上。
- 空报告防御：`MCPStatusMsg{Text:""}` 被忽略（有测试）。
- /resume 的文件 I/O 在 Execute（Update 循环）内同步执行——与既有 /memory 同步模式一致，
  设计文档已声明理由；列表读取有界（≤10 文件）。**接受**，但大会话文件下的 UI 停顿
  值得在后续观察（不作为本次问题项）。
- `applyCommandResult` 中 NewMessages 块先于 Display switch 执行，确认文案随后追加，
  顺序正确（显示 = 加载历史 + 确认；引擎 = 仅加载历史）。

---

## 6. Findings

### CR-BF03-1 — P1 — `applyCommandResult` 超出函数长度规范（107 行 > 80 行）
- **证据**：`internal/tui/update.go:359-465`（107 行）。基线 `db88b72` 已 82 行（轻微超标，
  属既有问题），本 PR 新增 ~25 行使其显著恶化。AGENTS.md：函数 ≤80 行。
- **修复方向**：将 NewMessages/resume 块（update.go:414-433）提取为私有 helper
  （如 `applyResumeResult(m AppModel, result commands.Result) (AppModel, bool)`），
  使主函数回到 ~82 行以下；顺带可消化基线超标。
- **初始状态**：❌ 待修复

### CR-BF03-2 — P1 — `FormatMCPStatus` 无同步读取池内共享连接的可变字段（潜在数据竞争）
- **证据**：`internal/commands/mcp.go:49-53,71-83,91-96` 直接读 `c.Status`/`c.Error`/
  `c.Client`/`c.Config`。agtkeel `Pool.GetAll`（pool.go:151-159）返回**新切片但共享
  `*ServerConnection` 活指针**；`Connect`/`Reconnect`/`Disconnect`（pool.go:71-135）
  在池锁内突变这些字段。读取侧（TUI fetch goroutine）拿不到池锁 → 一旦存在并发写者
  即构成数据竞争；`ListTools` 也可能撞上被 `Reconnect` 关闭的 client（此路径已优雅降级）。
- **当前实际风险**：**低且不可触发**——本提交接线中交互池为空/延迟连接
  （wire.go:127-128），`mcp_health.go` 用独立新池，全仓无对共享池的并发突变；
  全仓 `-race` 全绿。排序作用于 GetAll 返回的新切片，不扰动池内部（已核实）。
- **升级条件**：BF-01（启动接线）若引入后台连接/健康检查/重连 goroutine，本隐患即变为
  真实竞争。**必须在任何并发池突变落地前关闭或钉住约束。**
- **修复方向**：
  1. （本仓，Agent-TUI）在 `FormatMCPStatus` GoDoc 与 tui.md §4.1 显式记录
     "GetAll 返回活指针，读取期间禁止并发池突变" 的约束；
  2. （根治，跨仓）agtkeel `GetAll` 改为返回值快照（`[]ServerConnection` 或深拷贝），
     由 Tech Lead 定契约、经 PM 走跨仓变更流程（参照 BF-16 模式）。
- **定级说明**：并发安全类问题按定义可列 P0，但本提交内无并发写者、不可触发、
  且根治在跨仓侧，故列 P1 并附明确升级条件；若 BF-01 评审时约束仍未钉住，升级为 P0。
- **初始状态**：❌ 待修复

### CR-BF03-3 — P2 — `loadSessionMessages` 丢弃 `session.Resume` 底层错误（无 %w）
- **证据**：`internal/commands/resume.go:106-110`：友好文案替换了原始 err，诊断信息丢失。
  与 CLI 路径（bootstrap/session.go:34-35）同构，属 parity 行为。
- **修复方向**：可选——`fmt.Errorf("session %q not found for this project: %w\nTip: ...", ...)`
  保留底层错误；用户可见文案不变。
- **初始状态**：❌（可选）

### CR-BF03-4 — P2 — `defer mgr.Close() //nolint:errcheck` 与错误处理规范不完全一致
- **证据**：`internal/commands/resume.go:111`。AGENTS.md 要求所有 error 被处理；
  同文件 `listSessions`（resume.go:86-87）已检查 `closeErr`，而此处 nolint。
  与 CLI 路径（bootstrap/session.go:37）parity。
- **修复方向**：改为显式处理（如 close 失败时记入返回错误或 slog.Warn），与 listSessions 一致。
- **初始状态**：❌（可选）

### CR-BF03-5 — P2 — 设计文档应显式记录 /mcp 对 BF-01 的活状态依赖
- **证据**：本分支交互池无任何连接写者（wire.go:127 "deferred connections happen on
  first use"；internal/tools/mcp 仍为桩），故 BF-01 落地前交互模式 /mcp 恒显示
  "No MCP servers configured"（即使配置了 server）。计划 §3 BF-03 行已 sanction
  （"可先用现有 AppContainer.MCPPool"），但 tui.md §4.1 未写明该依赖，易被误读为功能缺陷。
- **修复方向**：§4.1 补一句：活连接状态依赖 BF-01 的启动接线；此前空池提示为预期行为。
- **初始状态**：❌（可选）

### CR-BF03-6 — P2 — 守护/边界测试小缺口
- **证据**：
  1. 无 "/mcp 之后 `queryEngine.GetMessages()` 不变" 的显式断言（cache gate 显示-only
     守护目前靠代码路径审查 + /resume 侧的分离测试间接覆盖）；
  2. `mcpToolCountSuffix` 的 `Status==connected && Client==nil` 分支（mcp.go:91-93）
     与 2s 超时路径无直接测试（降级路径已有 fake error 测试）。
- **修复方向**：在 `TestSlashMCP_FetchesStatusViaCmd` 追加引擎历史不变断言；
  补 nil-client 用例；超时可用阻塞 fake + ctx 取消驱动（可选）。
- **初始状态**：❌（可选）

---

## 7. Test Quality

**新增测试 3 个文件、688 行，成功/错误/边界路径覆盖充分：**

- `commands/mcp_test.go`（164 行）：nil 池、空池、connected+工具数、failed 带错误文案、
  全部 5 种状态标签、ListTools 失败降级、按名排序、命令接线（FetchMCPStatus/DisplayNone/桩文案消失）。
- `commands/resume_test.go`（251 行）：无会话、mtime 倒序（Chtimes 钉住确定性）、
  10 条上限、空会话列表、按 ID 恢复（角色/内容/确认文案/NewSessionID）、未知 ID、
  4 个路径穿越向量、损坏+无关条目跳过。临时目录真实 JSONL，无 mock 失真。
- `tui/mcp_resume_test.go`（273 行）：异步 Cmd 全链路（handleSlashCommand → cmd() →
  MCPStatusMsg → Update）、nil 池友好提示、空报告忽略、裸列表、历史**替换**而非追加
  （预置 stale history）、isLoading 拒绝（引擎与 sessionID 双断言）、未知 ID 错误显示、
  **/clear 遗留值回归**、`New()` 注入断言、`var _ tea.Cmd` 编译期钉住。

**桩钉住测试**：基线中 commands/tui 不存在钉住这两个桩文案的测试（`git grep` 核实，
仅 tools 层 notebookedit 有，不在本任务范围）；新测试反向断言桩文案消失，处理正确。

**覆盖率复核（go test -cover，本次评审实测）**：
| 包 | 宣称 | 实测 | 结论 |
|---|---|---|---|
| internal/commands | 60.6%→75.6% | **75.6%** | ✅ 属实 |
| internal/tui | 56.6%→57.7% | **57.7%** | ✅ 属实（<60% 为既有基线问题，不记本 PR） |
| internal/bootstrap | — | 76.7% | 参考 |

小缺口见 CR-BF03-6（P2）。

---

## 8. Doc Sync

- `docs/project/design/tui.md`：v1.0→**v1.1**，头部版本/日期/状态更新，新增 §4.1
  （接口、MVU 链路、超时、拒绝语义、已知局限、注入路径），文末变更记录表符合
  doc-sync-policy 模板。✅
- CONTEXT.md Design Notes：commands（新接口/新文件/新依赖/契约语义）、tui（注入参数/
  新 Msg/applyCommandResult 行为）、bootstrap（一行接线说明）均已手写补充；
  state/INDEX 为 docgen 自动重生成。✅
- `make docs-check`：**"docs are up to date"** ✅
- 时间戳-only churn 已按提交说明回退（diff 中无无关包的内容改动）。✅
- 缺口：CR-BF03-5（P2，BF-01 依赖未显式记录）。

---

## 9. Verification Runs（本次评审实测，worktree bf-tui-b1 @ 286e0c7）

| 检查 | 结果 |
|---|---|
| `go build ./...` | ✅ 通过 |
| `go vet ./...` | ✅ 通过 |
| `go test ./...`（全仓） | ✅ 全部 ok |
| `CGO_ENABLED=1 go test -race ./internal/commands/... ./internal/tui/... ./internal/bootstrap/...` | ✅ 全部 ok（gcc 已就位：/usr/bin/gcc，go1.27.4，CGO_ENABLED=1） |
| `CGO_ENABLED=1 go test -race ./...`（全仓） | ✅ 全部 ok |
| `go test -cover`（commands/tui/bootstrap） | ✅ 75.6% / 57.7% / 76.7% |
| `gofmt -l`（本 PR 触及文件/包） | ✅ 无输出（clean） |
| `make docs-check` | ✅ docs are up to date |
| `scripts/debt-check.sh` | ✅ 欠账红线检查通过（NOT_IMPLEMENTED 26→24，与实现同提交） |

**提交说明中 "-race unavailable（no C compiler）" 已过时**：本评审环境 gcc 安装完成，
-race 已补跑并全绿，无环境阻塞项遗留 QA。

**既有问题（不记本 PR）**：HEAD 全仓仍有 19 个未 gofmt 文件（实测确认，均在本 PR
触及范围之外）；tui 覆盖率 <60% 为基线问题（56.6%→57.7%，本 PR 净提升）。

---

## 10. Verdict

- **P0 列表：空。**
- **P1（2 项，退回 Agent-TUI，经 PM）**：
  - CR-BF03-1：`applyCommandResult` 107 行超 80 行规范 → 提取 resume helper。
  - CR-BF03-2：池连接活字段无同步读取的潜在竞争 → 本仓钉住约束文档 + 跨仓快照契约
    走 Tech Lead/PM 流程；**BF-01 引入并发池突变前必须关闭，否则升级 P0**。
- **P2（4 项，记录跟进）**：CR-BF03-3（错误未 %w）、CR-BF03-4（Close nolint）、
  CR-BF03-5（设计文档补 BF-01 依赖注记）、CR-BF03-6（守护/边界测试小缺口）。

**评审门控：通过（pass-ready）。** 依据 stub-backfill-plan §2.3（P0 全关闭）与
tech-lead-agent SOP，本提交无 P0，架构铁律、cache-first 三问、MVU、文档同步、
全部验证运行（含全仓 -race）均合格，可进入 QA 验收。P1 两项应在合并前或紧随的
修复提交中关闭，由 PM 退回 Agent-TUI；修复完成后 Tech Lead 按复核规范更新下方跟踪记录。

---

## 修复跟踪记录

| 问题编号 | 级别 | 描述摘要 | 状态 | 复核时间 | 备注 |
|---------|------|---------|------|---------|------|
| CR-BF03-1 | P1 | applyCommandResult 107 行 > 80 行规范 | ❌ 待修复 | — | 提取 resume helper |
| CR-BF03-2 | P1 | 池活字段无同步读取（潜在竞争） | ❌ 待修复 | — | 本仓钉约束 + 跨仓快照；BF-01 前必须关闭 |
| CR-BF03-3 | P2 | Resume 底层错误未 %w 保留 | ❌（可选） | — | 与 CLI 路径 parity |
| CR-BF03-4 | P2 | mgr.Close nolint:errcheck | ❌（可选） | — | 与 listSessions 一致性 |
| CR-BF03-5 | P2 | 设计文档补 BF-01 活状态依赖注记 | ❌（可选） | — | tui.md §4.1 |
| CR-BF03-6 | P2 | /mcp 引擎不变断言 + nil-client/超时用例 | ❌（可选） | — | cache 守护补强 |

---

## 复核记录（2026-11-18，主会话代行 Tech Lead 复核）

> 背景：修复 Agent 撞模型配额上限，依 AGENTS.md 维护模式由主会话接手修复并代行复核；
> 供人类 PR 评审确认。只核修复项。

修复提交：`1637769`（BF-03-F1 代码＋文档）、`10df48b`（BF-03-F2 纯测试）。

| 问题 | 复核结果 | 证据 |
|------|----------|------|
| CR-BF03-1（P1） | ✅ 关闭 | applyCommandResult 107→72 行（awk 实测）；applyResumeResult/applyDisplayResult 提取，既有 resume/clear/display 测试全绿证明逐路径行为不变 |
| CR-BF03-2（P1） | ✅ 关闭（本仓范围） | MCPServerLister/FormatMCPStatus GoDoc 钉住无并发池突变约束；tui.md v1.2 §4.1 同步；渲染侧值拷贝防御明示非同步。根治（GetAll 快照）维持跨仓 BF-19。**升级条件已解除**：BF-01-F1（50b2427）消除会话期 pool 写入，本项回落为潜在隐患＋文档化约束 |
| CR-BF03-3（P2） | ✅ 关闭 | session.Resume 错误经 %w 保留，用户可见文案不变 |
| CR-BF03-4（P2） | ✅ 关闭 | mgr.Close 失败显式返回错误，与 listSessions 模式一致 |
| CR-BF03-5（P2） | ✅ 关闭 | tui.md v1.2 补记 /mcp 活状态依赖 BF-01 启动接线（空池提示为预期行为） |
| CR-BF03-6（P2） | ✅ 关闭 | 引擎历史 DeepEqual 不变断言（cache gate 显示-only 守护）；connected+nil-client 分支；blockingMCPClient＋取消 ctx 驱动超时降级（诚实 tools unavailable） |

**门控复验**：全仓 `CGO_ENABLED=1 go test -race -count=1 ./...` 绿；make docs-check 通过；
debt-check 通过（本分支 24 不变）；触及文件 gofmt 干净；tui.md v1.2 变更记录完备。

**复核结论：P1 全关闭，评审门控通过（pass-ready）**，待 QA 验收与人类 PR 确认。
