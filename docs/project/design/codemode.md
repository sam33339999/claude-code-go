# Code Mode 设计文档（CM-01）

> 负责 Agent：主会话（维护模式）
> 状态：已实现（v2，2026-11-18）
> 版本：v2.0
> 研究依据：`docs/project/discussions/2026-11-18-codemode-feasibility.md`（含 moejs/goja 实测）

---

## 1. 概述

Code Mode 是一种工具使用范式：模型写 JavaScript 在沙箱内编排多个工具调用，
一次执行完成多步工作，只把最终结果返回模型——省 LLM 往返、省上下文 token、
支持循环/分支/过滤/并行。业界先例：Cloudflare Code Mode、Anthropic Code
Execution with MCP、OpenCode execute tool。

**本仓实现**：`CodeMode` 工具（`internal/tools/codemode`）＋沙箱引擎
（`internal/codemode`，moejs 主引擎，Runtime 介面可切换）。

## 2. 架构

```
LLM ──tool_call: CodeMode{code}──▶ engine 权限管线（CodeMode 本身过一次）
                                      │
                                      ▼
                              codeModeTool.Call
                                      │ bridge（每个内部调用）
                                      ▼
                    ValidateInput → CheckPermissions → checker.RequestPermission（HIL）→ Tool.Call
                                      ▲
                                      │ tools.*（Promise，跨 goroutine settle）
                              moejs 沙箱（async/await、Promise.all 真并行）
                              MemoryLimit 64MiB／Timeout 120s／eval 禁用
                              无 fs/net/env——唯一对外通道是 bridge
```

### 2.1 沙箱模型

- 每次调用新建隔离 runtime（moejs：~µs 级创建、~KiB 级内存）
- 代码契约：**async 函数 body**（语句＋return）；包装为 `(async () => { code })().then(capture, captureErr)`
- **TypeScript 输入（v2）**：input `typescript=true` → moejs CompileTS（Node 式类型剥离，错误指向 TS 源码）
- 暴露 globals：`tools.*`（Proxy 动态分发）、`codemode.search/describe`、`console.log`（捕获）
- `DisableDynamicCode: true`（eval/new Function 禁用）；builtins 冻结共享
- **受控 import（v2）**：`import { x } from "name"` 经 host resolver 读
  `dataDir/modules/<name>.js`（flat 名称、禁路径穿越、256KiB 上限）；
  无 dataDir 时 import 禁用。沙箱仍无任意 fs 访问
- 护栏：`Options.MemoryLimit`（默认 64MiB，超限不可 catch 地中断）、
  Timeout（默认 120s，rt.Interrupt）、ctx/AbortCh 取消联动

### 2.2 权限（沙箱≠授权）

bridge 完整复用引擎流程（orchestration.go 同构）：
1. `registry.Get(name)` → 未知工具报错（提示 codemode.search）
2. `tool.ValidateInput`
3. `tool.CheckPermissions`（Deny 即拒）
4. `checker.RequestPermission`（九层管线＋Ask→TUI 对话框；ToolUseID=codemode-N）
5. `tool.Call`；IsError 结果以 JS Error 拒绝（代码可 catch）

**每次内部调用独立过权限**——CodeMode 不是权限旁路。YOLO 模式下内部调用同样放行
（与直接调用一致）。

**批次批准（v2）**：bridge 先 CanUseTool 分流——
- 直接 Allow（规则/YOLO）：**不缓存**，每次重查（live 模式切换即时生效）
- Ask → RequestPermission（TUI 对话框）→ 用户批准 → **该工具在本次执行内
  预批准**（execAllow，执行 scope，不持久化）；后续同工具调用免询问
- Deny：**不缓存**，每次如实拒绝
语义：N 次同工具写调用＝1 次询问；不同工具分别询问；执行结束缓存即弃。

### 2.3 Progressive discovery（cache gate 友好）

- 工具 description 含**紧凑 catalog**（每工具一行：name＋首行描述），构造期快照
  （会话钉住，前缀稳定）
- 完整 schema 按需取：沙箱内 `codemode.search(query)`（关键词评分，name×3/desc×1，
  top-10）＋`codemode.describe(name)`（全描述＋InputSchema）
- 大 catalog（MCP 大量接入）时模型先 search 再 describe，避免全量 schema 进上下文

### 2.4 结果

- return 值（字串原样、其他 JSON indent）＋console.log 捕获
- 上限 32KiB（window-small）；**超限 spill（v2）**：全量写
  `dataDir/spill/<execID>.txt`，回传 4KiB preview＋文件指针（模型用 Read/Grep 取回）；
  无 dataDir 时降级为截断＋提示
- 执行错误（编译/超时/内存/拒绝未 catch）→ IsError 结果回模型自纠

### 2.5 执行历史与复用（v2）

- 每次执行追加 `dataDir/history.jsonl`：{id, ts, ok, duration_ms, typescript,
  code（≤2000 字符）, tools（去重调用名单）, preview, error}——best-effort 不致命
- 沙箱内 `codemode.history(limit)`（默认 10、上限 50，newest-first）取回历史，
  模型读取过去代码改编复用
- replay/rollback（副作用追踪＋重放审批）属 durable runtime 级别，留 v3

## 3. 引擎选型（实测依据见可行性研究 §2/§9）

| | moejs（主） | goja（fallback 预留） |
|---|---|---|
| 纯 Go 零 CGO | ✓ | ✓ |
| async/await＋Promise | ✓ 实测 | ✓ 实测 |
| 真并行（跨 goroutine settle） | ✓ 实测（3×60ms=60ms） | 需自建 loop |
| MemoryLimit | ✓ 原生 | ✗ |
| TypeScript | ✓ CompileTS（v2 可开放） | ✗ |
| 成熟度 | alpha v0.1.1（钉版＋介面隔离） | 成熟 |

Runtime 介面（`internal/codemode/runtime.go`）隔离引擎：Execute(ctx, code, CallFunc, Options)。
升级/切换引擎走独立 PR＋全量测试。

## 4. 接线

`bootstrap/wire.go` 两条构建路径于 permChecker 建立后注册：
`reg.Register(cmtool.NewCodeModeTool(reg, permChecker, cmengine.NewMoejsRuntime()))`。
RegisterBuiltinTools 不含 CodeMode（依赖 checker，同 plan/HIL 工具模式）。

## 5. 已知局限（v2 后）

- 无 replay/rollback/审批持久化（Cloudflare durable runtime 级别，留 v3）
- 批次批准为执行 scope 同工具粒度（非 read/write 分类闸门；Agent Lee 模式留 v3）
- import 仅 host 预置模块（沙箱不能创建模块——无 fs，结构性限制）
- moejs alpha：API 可能变动（钉版 v0.1.1＋介面隔离缓解）

## 5.1 事件驱动待命模式（EV-01 搭配）

Code Mode＋MCP 通知路由组成零轮询待命：
1. CodeMode 脚本握手＋订阅：`await tools.mcp__svc__subscribe({...})`（MCP 连接
   由 Pool 管理，initialize 握手自动；订阅调服务工具）
2. 脚本返回，代理待命——不烧 token
3. 服务有事 → MCP notification → Pool handler → msgqueue（ModeExternalPrompt）
   → TUI drain 唤醒代理 → 代理处理（可再跑 CodeMode 编排，codemode.history 复用逻辑）

## 6. 测试覆盖

- `internal/codemode`：return/logs、bridge async、Promise.all 并行计时、
  错误可 catch、MemoryLimit、Timeout、ctx 取消、eval 禁用、语法错误、search/describe
- `internal/codemode` catalog：构建/排序/首行截断、search 排名、describe 大小写、catalog 文本
- `internal/tools/codemode`：端到端、权限拒绝传播＋可 catch、未知工具、空输入、
  32KiB cap、沙箱内 discovery、description catalog、metadata

## 变更记录

| 版本 | 日期 | 修改人 | 变更摘要 |
|------|------|--------|---------|
| v1.0 | 2026-11-18 | 主会话 | CM-01 v1 实现：moejs 沙箱＋权限 bridge＋progressive discovery＋32KiB cap＋23 测试 |
| v2.0 | 2026-11-18 | 主会话 | CM-02 v2：TypeScript 输入（CompileTS）；批次批准（Ask→批准→执行内同工具预批准，Deny/直接 Allow 不缓存）；结果 spill（>32KiB→文件＋4KiB preview＋指针）；执行历史（history.jsonl＋codemode.history 复用）；受控 import（host modules 目录、flat 名称、256KiB cap）。新增测试×11（工具端×8＋引擎×3），总 34 测试。dataDir 接线 ~/.claude/codemode。 |