# 桩代码全量回填计划（Stub Backfill Plan）

> **维护人**：PM Agent
> **创建**：2026-11-18
> **决策来源**：人类于 2026-11-18 拍板「全量回填」（patrol-13 §7 决策项 2，方案 A）
> **跟踪方式**：task-registry.yaml 已封存（2026-08-29），本档案为维护期任务台账。
> 任务 ID 系列 `BF-xx` 不与构建期 T-xxx 混用。**建议人类将本表镜像为 GitHub Issues**（当前环境无 GitHub 凭据，PM 无法代建）。
> **合并方式**：feat 分支 + PR（维护模式约定）。当前环境无 push 凭据，Agent 提交到本地分支，push/PR 由人类执行。

---

## 1. 范围

- 26 处本仓用户可见 `not yet implemented` 桩（patrol-13 §2 清单）
- 1 处原 agtkeel 库内桩（MCP WebSocket transport，BF-16；内核搬回后已进入扫描范围，基线 19）
- 56 处 `TODO(dep)` 中随回填自然消除的部分（不单独追求清零，debt 红线只降不升）
- `docs/ROADMAP.md` 虚报修正（BF-00，与回填并行）

## 2. 通用验收门控（每个 BF 任务适用）

1. 开发 Agent 完成实现 + 单元测试（同包 `_test.go`），`go test -race` 全绿
2. **Cache-first 评审门控（2026-11-18 人类决策正式采纳，依据 `docs/PROMPT_CACHE_AND_COST.md`）**：
   凡触及 prompt 构造、工具 schema、工具注册/增删/改名、MCP 注册、压缩、请求序列化的变更，
   提交说明必须回答三问：是否移动前缀？是否增大窗口？是否新增调用？
   - 移动前缀 = 成本决策，须附经济学论证；动态数据优先 turn-tail 投影，不改前缀
   - 会话内工具集合与 schema 字节钉住（MCP 重连差异下个会话生效）
   - 在变更面附近加聚焦守护测试（schema 稳定性 / 前缀字节一致性）
   - 压缩是唯一被认可的前缀重置；其余前缀打破须评审注记
3. Tech Lead 代码评审（先读 `docs/project/design/<layer>.md` + cache-first 门控），P0 全关闭
4. QA 验收（功能完整性 + 测试质量；集成测试如需新增由 QA 负责）
5. 文档同步：受影响包 `make docs` 重生成 CONTEXT.md（含 Design Notes），`make docs-check` 通过；
   接口变更按 `docs/project/doc-sync-policy.md` 更新设计文档
6. 桩减少后 `scripts/debt-check.sh --update` 刷新基线，**与实现同一次提交**
7. 按 `docs/project/harness/protocols/agent-output.md` 输出 JSON 汇报

## 3. 任务分解与批次

### Batch 1 — 高就绪度（2026-11-18 派发）

| ID | 任务 | 角色 | 涉及文件 | 依赖 | 验收要点 |
|----|------|------|----------|------|----------|
| BF-01 | MCP 三工具接线：MCPProxyTool 改用 agtkeel `mcp.AdaptToTool`；ListMcpResources/ReadMcpResource 经 Pool→client 真实调用；启动时从 Pool 注册各 server 工具 | agent-tools | `internal/tools/mcp/`、`internal/bootstrap/tools.go`、`internal/bootstrap/wire.go`（注册路径） | 无（agtkeel 能力已就绪） | 对 mock MCP server 的真实调用测试；−3 桩 |
| BF-02 | Sleep 工具：context-aware 定时，支持 AbortCh 取消 | agent-tools | `internal/tools/misc/misc.go`（Sleep 部分）+ 测试 | 无 | 取消/超时路径测试；−1 桩 |
| BF-03 | /mcp、/resume 斜杠命令真实实现 | agent-tui | `internal/commands/builtins.go` + 测试；如需注入接口，最小化变更并汇报 | /mcp 依赖 BF-01 的 Pool 状态暴露（可先用现有 AppContainer.MCPPool）；/resume 复用 `internal/bootstrap/session.go` 已有逻辑 | /mcp 展示连接状态；/resume 列出/恢复会话；−2 桩 |
| BF-00 | ROADMAP 虚报修正（docs PR） | maintainer-docs | `docs/ROADMAP.md` | patrol-13 §4 清单 | 与 cases.md 对齐；不虚构新状态；**追加**：概览框"会话持久化 ✅"亦属虚报（store 存在但生产零接线，BF-18），一并修正 |
| BF-17 | Cache-first 门控制度化（2026-11-18 人类决策采纳） | maintainer-docs + tech-lead | `CONTRIBUTING.md`（新增 Cache-first review gate 节）、`docs/project/agents/tech-lead-agent.md`（评审清单加入门控）、`docs/PROMPT_CACHE_AND_COST.md` 入库、本仓路径映射说明（Reasonix 路径 → agtkeel/本仓对应物） | 无 | 门控文本引用参考规范；映射表区分"已有/缺失"机制；Tech Lead 评审通过 |

### Batch 2 — 中就绪度（Batch 1 评审通过后派发）

| ID | 任务 | 角色 | 备注 |
|----|------|------|------|
| BF-05 | /config、/terminal-setup 斜杠命令 | agent-tui | /config 设置查看/编辑对话框；/terminal-setup 生成 shell 补全脚本 |
| BF-06 | CLI：update（含 --check）、install、agents | agent-cli | **agents 形态需 Tech Lead 先裁决**（cases.md §B11：oracle 为 options 驱动，Go 为三桩子命令）；update 对齐 GitHub releases 检查 |
| BF-07 | TodoWrite：会话 todo 状态 + TUI 面板 | agent-tools + agent-tui | 契约先行：todo 状态接口由 Tech Lead 定义；agtkeel `TodosDir` 常量可落地 |
| BF-08 | AskUserQuestion：engine Msg 类型 + TUI 问答对话框 | agent-core + agent-tui | 内核搬回后为**同仓变更**（internal/engine Msg 契约），需 Tech Lead 评审；参照权限对话框管线 |
| BF-09 | Enter/ExitPlanMode：模式状态机 + 切换入口 | agent-tools + agent-tui | agtkeel permissions 已有 `PermissionModePlan` 执行半边；先核查 mode 存储位置再定方案 |
| BF-18 | 交互/ headless 会话持久化接线（BF-03 发现的新缺口，P1） | agent-cli + agent-core | 生产代码零调用 session.New/AppendEntry（仅测试有），--resume 与 /resume 无真实数据源；ROADMAP"会话持久化 ✅"属同类虚报（能力在 agtkeel，接线缺失）。先核查引擎是否有会话写入钩子，若需 agtkeel 新契约则走跨仓流程 |

### Batch 3 — 低就绪度（需新建子系统，Batch 2 期间做契约设计）

| ID | 任务 | 角色 | 备注 |
|----|------|------|------|
| BF-10 | Enter/ExitWorktree：git worktree 管理器 | agent-tools | 工具间禁止互依赖，git 操作自包含 |
| BF-11 | NotebookEdit：.ipynb JSON 解析与编辑 | agent-tools | stdlib encoding/json 即可，无需第三方 |
| BF-12 | ToolSearch：注册表搜索索引 | agent-tools | agtkeel `SearchHint()` 契约已存在，registry 有 Filter/Names |
| BF-13 | Skill：skill 注册表与加载器 | agent-tools | markdown skill 文件加载（~/.claude/skills + 项目级） |
| BF-14 | Brief：摘要工具 | agent-tools | 复用 agtkeel compact 管线 |
| BF-15 | SyntheticOutput：会话操纵层 | agent-core | 依赖引擎会话接口，契约先行 |
| BF-16 | MCP WebSocket transport（internal/mcp/transport.go 桩） | agent-services | 内核搬回后为**同仓**；nhooyr.io/websocket 或等价方案由 Tech Lead 定 |

## 4. 并发与批次纪律

- **并发 Agent 上限 3 个（含主会话）**——人类 2026-11-18 指示；即子 Agent 最多 2 个。
  代码实现（重）≤2；修复（轻）≤3 且受总数上限约束；评审 ≤2。
- 配额受限时优先主会话直接执行（维护模式单人负责人＋主会话，AGENTS.md），少开子 Agent。
- 每个开发 Agent 在独立 worktree/feat 分支工作，**不触碰主工作区未提交的用户改动**
  （go.mod replace、wire.go OpenAI 修复、.agtkeel-uncommitted.diff）
- 跨层接口变更：先 Tech Lead 契约评审，再实现（Contract-First）
- 内核已于 2026-11-18 搬回本仓（ARCH-01）：原跨仓任务 BF-08/16/19 改为同仓，直接 feat 分支 + PR

## 5. 状态跟踪

| ID | 状态 | worktree/分支 | 备注 |
|----|------|---------------|------|
| BF-00 | completed | bf-docs-roadmap | 提交 42e0fe0；Phase 2 虚报全修正（含会话持久化勘误），与 cases.md 对齐 |
| BF-17 | completed | bf-docs-roadmap | 提交 726e0f4；参考规范入库＋CONTRIBUTING 闸门节＋映射表＋tech-lead 维护期补充 |
| BF-01 | completed（评审通过） | bf-tools-b1 | 实现 1732d65＋修复 50b2427＋复核 f7a9eb1；P0 关闭，pass-ready；待 QA／人类 PR 确认 |
| BF-02 | completed（评审通过） | bf-tools-b1 | 同上；Sleep overflow 上限已补 |
| BF-01-F1/F2 | completed | bf-tools-b1 | 主会话接手（子 Agent 四度配额失败）；F1=50b2427、F2=de4ab23；复核记录 f7a9eb1；全仓 -race 绿、docs-check/debt-check 通过（22/44） |
| BF-03-F1/F2 | completed | bf-tui-b1 | 主会话接手（配额）；F1=1637769（CR-BF03-1/2/3/4/5）、F2=10df48b（CR-BF03-6 守护测试）；全仓 -race 绿、docs-check/debt-check 通过（24） |
| BF-03 | completed（评审通过，已合并） | bf-tui-b1 | 实现 286e0c7＋修复＋复核 4b38842；P1 全关闭；CR-BF03-2 升级条件已解除；已合并 feat/impl |
| BF-20 | completed | reinternalize-kernel | 全仓 gofmt 50 檔，随内核搬回提交 59b589e 一并了结；gofmt -l 全仓 0 |
| ARCH-01 | completed（已合并） | reinternalize-kernel | 内核搬回（反转 PR #31）：13 包 87 檔恢复原始路径＋89 檔 import 改写＋go.mod 去 agtkeel＋OpenAI 增强正式入库；59b589e→合并 b1c0e68；40 包 -race 全绿；debt 基线 44/19（WS 桩 scope 变化）；BF-08/16/19 改为同仓任务 |
| BF-19 | pending（同仓技术债） | 待建 | CR-BF03-2 根因修复：internal/mcp Pool 快照式 GetAll API（现返回共享可变指针）。**已非合并阻塞**——BF-01-F1 消除会话期 pool 写入后回落为潜在隐患＋约束文档化；内核搬回后根治在同仓做。併案 CR-BF01-7（jsonRPC reader 绑定 initialize ctx、HTTP Close 语义）。API 形态由 Tech Lead 裁决 |
| BF-18 | completed（已合并，主会话代行核验） | bf-tui-b1 | 提交 a75c1c7；internal/transcript Recorder＋TUI turn 边界落盘＋headless 收尾＋/resume 换文件；已合并 feat/impl，合并后全仓 -race 复验绿；正式评审留待人类 PR |
| BF-05 | completed（已合并，主会话代行核验） | bf-tui-b1 | 提交 e7411d4；/config 只读面板＋/terminal-setup 检测；已合并 feat/impl；正式评审留待人类 PR |
| BF-06 | completed（已合并） | bf-cli-b2 | 提交 d629d14；agents 对齐 oracle 单一命令形态＋--setting-sources；update＋upgrade 别名（GitHub releases 检查）；install [target] 平台 archive 原子安装；cases.md B8/B9/B10 关闭＋二级缺口剩 1；debt 19→13；全仓 -race 绿；实机 `claude agents` ✓ |
| BF-07..BF-16 | pending | — | 按批次派发（BF-08/16 已改同仓） |

> 状态变更由 PM 独家更新本表；Agent 汇报按 agent-output.md JSON 格式。