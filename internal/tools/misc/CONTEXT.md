---
package: misc
import_path: internal/tools/misc
layer: tools
generated_at: 2026-09-23T11:03:32Z
source_files: [doc.go, misc.go]
---

# internal/tools/misc

> Layer: **Tools** · Files: 2 · Interfaces: 0 · Structs: 5 · Functions: 0

## Structs

- **BriefInput** — 1 fields: Content
- **SkillInput** — 2 fields: Skill, Args
- **SleepInput** — 1 fields: Milliseconds
- **SyntheticOutputInput** — 2 fields: Content, IsError
- **ToolSearchInput** — 1 fields: Query

## Dependencies

**Imports:** *(none — zero-dependency)*

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-09-23 基座换库（agtkeel import-swap）**：可复用 agent 内核已逐字提取到 `github.com/tunsuy/agtkeel`（MIT），本包 import 随之改写：`internal/{api,compact,msgqueue,engine,session,permissions,hooks,mcp,config}` → `agtkeel/*`、`internal/tools`（接口契约+注册表）→ `agtkeel/tools`、`pkg/types` → `agtkeel/types`、`pkg/utils/{fs,ids}` → `agtkeel/utils/*`。本包代码**仅 import 路径重写，零逻辑改动**，`go build`/`go vet`/`go test -race ./...` 全绿。已知表现局限：docgen 依赖图按本模块前缀分类，agtkeel 视作第三方库，故上方 Imports 栏不再展示内核依赖（与 cobra 等一致）。

- **2026-11-18 BF-02 Sleep 实现（桩回填）**：`SleepTool.Call` 以 `time.NewTimer` + `select` 监听定时完成 / `UseContext.Ctx.Done()` / `UseContext.AbortCh` 三路信号；取消/超时返回 `IsError=true` 的诚实结果（不返回硬 error）。输入校验先查 `milliseconds` 字段存在性（json 缺失填零无法与显式 0ms 区分），再查类型与非负。nil UseContext 回退 background ctx。其余 4 个 misc 桩（Skill/Brief/ToolSearch/SyntheticOutput）属后续批次，未触碰。
