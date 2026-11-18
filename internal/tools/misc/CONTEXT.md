---
package: misc
import_path: internal/tools/misc
layer: tools
generated_at: 2026-11-18T05:23:18Z
source_files: [doc.go, misc.go]
---

# internal/tools/misc

> Layer: **Tools** · Files: 2 · Interfaces: 0 · Structs: 5 · Functions: 1

## Structs

- **BriefInput** — 1 fields: Content
- **SkillInput** — 2 fields: Skill, Args
- **SleepInput** — 1 fields: Milliseconds
- **SyntheticOutputInput** — 2 fields: Content, IsError
- **ToolSearchInput** — 1 fields: Query

## Functions

- `NewToolSearchTool(reg *tools.Registry) tools.Tool`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。

- **2026-11-18 BF-02 Sleep 实现（桩回填）**：`SleepTool.Call` 以 `time.NewTimer` + `select` 监听定时完成 / `UseContext.Ctx.Done()` / `UseContext.AbortCh` 三路信号；取消/超时返回 `IsError=true` 的诚实结果（不返回硬 error）。输入校验先查 `milliseconds` 字段存在性（json 缺失填零无法与显式 0ms 区分），再查类型与非负。nil UseContext 回退 background ctx。其余 4 个 misc 桩（Skill/Brief/ToolSearch/SyntheticOutput）属后续批次，未触碰。
- **2026-11-18 BF-01-F1/F2 评审修复（CR-BF01-5）**：Sleep 加产品级上限 `maxSleepMilliseconds=24h`，超限诚实拒绝。动机除产品边界外：int 输入 >~9.2e12 ms 时 `time.Duration(ms)*time.Millisecond` 溢位为负，NewTimer 立即触发并谎报成功。上限仅运行时校验，provider-facing schema 字节不变（cache gate：不前缀移动）。测试：overflow 拒绝＋at-cap 接受。
- **2026-11-18 BF-12 ToolSearch 实现**：NewToolSearchTool(reg) 注册表注入（RegisterBuiltinTools 自引用注册）；关键词评分（name camelCase 拆词×3／SearchHint×2／description×1，前缀匹配），top-10 JSON [{name,description}]；空 query 拒绝、无匹配诚实提示、nil registry 诚实报错。schema 字节不变。
- **2026-11-18 BF-14 Brief 实现**：抽取式摘要（确定性、零 LLM 调用——cache gate Q3 友好）；句子词频评分（停用词排除＋长度归一＋首尾位置加分），top budget（3..8）按原序输出＋压缩统计头；≤3 句全文保留；200k 字符输入上限。**分歧文档化**：oracle legacy Brief=SendUserMessage（Kairos feature-gated），本仓设计规格定义为内容摘要器，按规格实现。
