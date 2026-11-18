# Code Mode 设计文档（CM-01）

> 负责 Agent：主会话（维护模式）
> 状态：已实现（v1，2026-11-18）
> 版本：v1.0
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
- 暴露 globals：`tools.*`（Proxy 动态分发）、`codemode.search/describe`、`console.log`（捕获）
- `DisableDynamicCode: true`（eval/new Function 禁用）；builtins 冻结共享
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

### 2.3 Progressive discovery（cache gate 友好）

- 工具 description 含**紧凑 catalog**（每工具一行：name＋首行描述），构造期快照
  （会话钉住，前缀稳定）
- 完整 schema 按需取：沙箱内 `codemode.search(query)`（关键词评分，name×3/desc×1，
  top-10）＋`codemode.describe(name)`（全描述＋InputSchema）
- 大 catalog（MCP 大量接入）时模型先 search 再 describe，避免全量 schema 进上下文

### 2.4 结果

- return 值（字串原样、其他 JSON indent）＋console.log 捕获
- 上限 32KiB（window-small），超限截断＋诚实提示
- 执行错误（编译/超时/内存/拒绝未 catch）→ IsError 结果回模型自纠

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

## 5. 已知局限（v1）

- 代码契约为同步 bridge＋async 包装；`import` 不开放（Importer 未接）
- TypeScript 输入未开放（CompileTS 就绪，v2 可加）
- 无执行历史/replay/rollback（Cloudflare durable runtime 级别，超出 v1）
- 权限轰炸：N 个内部写调用＝N 次 Ask（v2 考虑批次批准 UX）
- moejs alpha：API 可能变动（钉版 v0.1.1＋介面隔离缓解）

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