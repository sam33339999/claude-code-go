# Code Mode 可行性研究（CM-01）

> **维护人**：PM Agent（主会话）
> **创建**：2026-11-18
> **状态**：researched（调查＋运行时实测完成，待人类批准立项）
> **触发**：人类询问「codemode 是啥？为我的工具添加 codemode 有机会吗？」

---

## 1. Code Mode 是什么

**一种工具使用范式：模型写代码来编排工具，而不是逐个发起 tool call。**

传统模式：模型选一个工具→结果回到模型→再选下一个。每步都是一次 LLM 往返，
中间数据全部进入上下文。

Code Mode：模型只看到**一个代码执行工具**。它写一段 JS/TS，在沙箱里调用
暴露为 typed API 的工具集，用循环/分支/过滤编排多步工作，**只把最终结果返回模型**。

```js
// 模型写的代码（一次执行完成多步编排）
async () => {
  const projects = await tools.list_projects({ status: "active" });
  const tasks = [];
  for (const p of projects) {
    tasks.push(...(await tools.list_tasks({ projectId: p.id })));
  }
  return tasks.filter(t => t.status === "blocked");
}
```

### 业界实作（已研究）

| 来源 | 要点 |
|------|------|
| **Cloudflare Code Mode**（2025-09 博客＋Agents SDK） | MCP tools→TypeScript API；V8 isolate 沙箱（无 fs/net/env）；工具调用经 RPC 回到 agent loop 分发；console.log 回传结果 |
| **Cloudflare Code Mode MCP**（2026-02） | 整个 Cloudflare API 只暴露 search()＋execute() 两个工具，**~1000 tokens 固定占用**（vs 117 万 tokens，−99.9%）；progressive discovery |
| **Anthropic「Code Execution with MCP」** | 独立探索同一模式；中间数据留在执行环境，token 大幅下降 |
| **OpenCode execute tool**（本会话亲身运行中） | tools.* catalog＋JS 沙箱＋Promise.allSettled 并行；catalog 完整列在 system prompt |
| **Agent Lee（Cloudflare 2026-04）** | 权限架构：代码分类 read/write，read 直接代理、write 需显式批准；凭证不在代码中，服务端注入 |

### 为什么有效

1. **LLM 见过海量真实代码，没见过多少 tool call**——写代码比直接调工具更准（Cloudflare 原话：莎士比亚上一个月中文课写剧本）
2. **省往返**：多步编排一次执行完成，中间结果不进上下文
3. **省 token**：大 catalog 使用 progressive discovery（search/describe），固定占用
4. **可组合**：循环、分支、过滤、结果塑形——图灵完备的编排

## 2. Go 运行时实测（关键证据）

**goja（github.com/dop251/goja，纯 Go、零 CGO、活跃维护 2026-11-17）实测**：

```
arrow+let+const        OK: 42
template literal       OK: val=42
destructuring          OK: 3
Promise.then           OK（microtask 需排水）
async/await            OK: [object Promise]   ← 语法已支援！
generator              OK: 1
for-of                 OK: 6
spread                 OK: 9
class                  OK: 7

bridged 同步函数＋async/await：
  vm.Set("toolAdd", func(a,b int) int {...})
  (async () => { __result = await toolAdd(1,2); })()
  → __result = 3   ← goja leave() 自动排水 jobQueue，实测通过
```

**结论**：goja 已支援 ES5.1＋大量 ES6+（async/await、Promise、generator、class），
且 top-level 返回时自动排水 microtask（runtime.go leave()）。
**bridged 工具函数可以是同步 Go 函数**——await 非 Promise 值经 microtask 解析，
async 代码自然运行。无需 esbuild/babel 转译。

其他选项对比：v8go/quickjs-go（CGO，重，违背本仓轻量风格）；
starlark/lua（非 JS，LLM 熟练度低）。**goja 是明确最优选**。

## 3. 本仓架构适配度：高

现有基础设施几乎全部可复用：

| Code Mode 需要 | 本仓已有 |
|---------------|---------|
| 工具 catalog | `tools.Registry`（All/Get/Names）＋InputSchema＋SearchHint（BF-12 刚做的搜索可直接用于 progressive discovery） |
| 权限闸门 | `permissions.Checker.RequestPermission` 九层管线＋Ask→TUI 对话框（HIL 全套） |
| 工具执行 | `Tool.Call(Input, *UseContext, OnProgressFn)` 统一介面 |
| 取消传播 | UseContext.Ctx/AbortCh → goja `vm.Interrupt()` |
| DI 接线 | bootstrap AppContainer 模式（registry＋checker 注入） |
| MCP 工具 | AdaptToTool 已在 registry——codemode 自动获得 MCP 编排能力 |
| 输出上限 | 32KiB cap＋spill 模式（Reasonix 规范，cache gate Q2） |

**插入点**：新包 `internal/codemode`（runtime＋bridge）＋
`internal/tools/codemode`（Tool 包装）＋bootstrap 注册。零内核改动。

## 4. 设计草案（v1）

### 4.1 工具介面

```
CodeMode 工具：input { code: string }
description：写 JS async 箭头函数；tools.* namespace catalog（或 progressive discovery）
```

### 4.2 沙箱与 bridge

- 每次调用新建 goja VM；**无 fs/net/env/process**——唯一对外通道是 bridged tools.*
- bridge 流程：JSON 参数 → **checker.RequestPermission（完整管线，含 Ask→TUI）** →
  tool.Call → JSON 结果回传。**绝不绕过权限**（Agent Lee 原则：沙箱≠授权）
- console.log 捕获＋return 值序列化作为 tool result
- 超时：time.AfterFunc → vm.Interrupt；ctx/AbortCh 联动
- 结果上限 32KiB，超限截断＋诚实提示（或 spill 到文件＋指针）

### 4.3 Catalog 策略

- 工具少（本仓 30+）：完整 catalog 进 CodeMode 工具 description（一次性）
- 工具多（MCP 大量接入）：progressive discovery——`codemode.search(query)`
  （复用 BF-12 评分搜索）＋`codemode.describe(name)`（返回 schema）
- cache gate：catalog 是静态 prefix 的一部分，会话内钉住（同 MCP schema 钉住原则）

### 4.4 权限 UX

- CodeMode 工具本身过一次权限；内部每次 tool call 独立过管线
- 增强（v2）：「本次执行内 always allow」批次批准；read/write 分类闸门（Agent Lee 模式）

### 4.5 并行（v2）

goja 单线程——v1 bridged 同步（顺序执行，组合收益已拿满）。
真并行需 goroutine→jobQueue 调度器（goja_nodejs event loop 或自建），v2 再做。

## 5. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| goja 无堆内存上限 | 中 | 超时 Interrupt＋结果上限；恶意/失控代码最多跑到超时 |
| 权限轰炸（内部 N 次 Ask） | 中 | v1 如实走管线；v2 批次批准 UX |
| LLM 代码出错 | 低 | 错误信息回传模型自纠（标准 agent 循环）；schema 校验前置 |
| 提示注入经工具结果进代码 | 低 | 沙箱隔离＋写操作权限闸门（结构性防护，Agent Lee 模式） |
| goja ES 兼容边角 | 低 | 实测主流语法全过；catalog 明示支援范围 |
| 单线程无真并行 | 低 | v1 顺序已满足编排收益；v2 event loop |

## 6. 工作量与阶段

| 阶段 | 内容 | 估计 |
|------|------|------|
| P1 | internal/codemode runtime＋bridge（同步）＋CodeMode 工具＋权限集成＋取消/超时＋测试 | 1 会话 |
| P2 | catalog 生成＋progressive discovery（search/describe）＋输出 cap/spill | 0.5 会话 |
| P3 | 权限 UX（批次批准）＋并行 event loop（可选） | 1 会话 |
| P4 | 文档（tools.md/cli.md/CONTEXT）＋parity 记录＋门控 | 0.5 会话 |

**v1（P1+P2+P4）约 1.5-2 个工作会话**。架构适配度高，无阻塞性技术风险。

## 7. 结论

**有机会，而且很适合。** 本仓的 Tool 介面＋Registry＋权限管线＋DI 正是
Code Mode 需要的全部基础设施；goja 实测证明纯 Go 沙箱可行（async/await 直接跑）；
BF-12 ToolSearch 可直接复用为 progressive discovery。建议立项 CM-01（v1 范围 P1+P2+P4）。

---

## 9. moejs 补充研究（2026-11-18，人类指示查看）

**moejs（github.com/MoeForge/moejs v0.1.1，前 Calcium-Ion/moejs）——纯 Go JS 运行时，
alpha，Go 1.25+（本仓刚升级 ✓），零 CGO，唯一依赖 moego（Base64 SIMD）。**

### 实测证据（/tmp/opencode/goja-probe/moejs_probe.go，五项全过）

```
1. Promise.all parallel: map[a:map[slept:100] b:map[slept:100] elapsed:100]
   ← 两个 100ms bridged 工具调用，总耗时 100ms＝真并行（跨 goroutine settle promise）
2. MemoryLimit stopped: true     ← 1MiB 上限，无限分配被强制停止（不可被 JS catch）
3. Interrupt infinite loop: true ← 任意 goroutine 可中断死循环
4. CompileTS: 42                 ← TypeScript 类型剥离原生支援（Node 式）
5. Unmarshal to struct: {[1 2 3] true}  ← JSON 结果直接进 Go struct（DecodeOptions 可限大小）
```

### moejs vs goja（CM-01 选型对比）

| 维度 | goja | moejs |
|------|------|-------|
| 纯 Go/零 CGO | ✓ | ✓ |
| async/await | ✓（实测） | ✓（实测） |
| **内存上限** | ✗（CM-01 中风险） | **✓ MemoryLimit 原生**（风险消除） |
| **真并行工具调用** | 需 v2 event loop | **✓ v1 即可**（loop.NewPromise＋跨 goroutine settle，实测） |
| ES 完整度 | ES5.1+部分 ES6 | ES modules/Proxy/BigInt/optional chaining/async iteration；test262 80,971 过 0 败 |
| TypeScript | 需转译 | **CompileTS 原生** |
| 性能 | 基准 | 官方基准：2× Sobek（goja 系）、1/3 内存；新 runtime 1.4µs/81KiB |
| 沙箱控制 | 基础 | eval/Function 可禁用/限长；import 经 host hook；builtins 冻结共享 |
| 成熟度 | 多年成熟 | **alpha（v0.1.1，2026-11 发布），API 可能变** |
| Go 版本 | 1.21+ | 1.25+（本仓已升 ✓） |

### 修订建议

**首选 moejs**：MemoryLimit＋真并行＋TS 支援直接消除 goja 方案的两个主要缺口
（内存无上限风险、并行延后 v2），且性能/内存更优，与本仓 Go 1.25 刚好匹配。

**风险控制**：alpha API 不稳定 → (1) go.mod 钉住版本；(2) internal/codemode 定义
自有 `Runtime` 介面包装引擎（bridge/沙箱/结果提取），goja 作为可切换 fallback；
(3) 引擎升级走独立 PR＋全量测试。

**CM-01 设计修订**：§4.5 并行从 v2 提前到 v1（eventloop.NewPromise 模式）；
§5 内存风险降为低（MemoryLimit）；新增 TS 支援（模型可写 TS，CompileTS 剥离类型）。
工作量估计不变（1.5-2 会话），v1 能力更强。

## 8. 参考

- Cloudflare Code Mode 博客（2025-09-26）＋Code Mode MCP（2026-02-20）＋Agents SDK docs
- Anthropic「Code Execution with MCP」
- OpenCode execute tool（本会话运行环境，第一手）
- goja 实测：/tmp/opencode/goja-probe（2026-11-18，输出见 §2）
- 本仓基础设施：internal/tools/registry.go、internal/permissions/checker.go、
  BF-12 ToolSearch（misc.go）、HIL 管线（BF-08）