---
package: codemode
import_path: internal/tools/codemode
layer: tools
generated_at: 2026-11-18T06:40:49Z
source_files: [tool.go]
---

# internal/tools/codemode

> Layer: **Tools** · Files: 1 · Interfaces: 0 · Structs: 0 · Functions: 1

## Functions

- `NewCodeModeTool(reg *tools.Registry, checker permissions.Checker, engine codemode.Runtime) tools.Tool`

## Dependencies

**Imports:** `internal/codemode`, `internal/permissions`, `internal/tools`

**Imported by:** `internal/bootstrap`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->

## Design Notes

- **2026-11-18 CM-01 CodeMode 工具**：bridge 完整复用引擎权限流程（ValidateInput→CheckPermissions→RequestPermission HIL→Call），沙箱≠授权；内部拒绝以 JS Error 拒绝可 catch。结果 32KiB cap＋截断提示；字串 return 原样、其他 JSON。description 含紧凑 catalog（首行描述），完整 schema 经沙箱内 describe 按需取。wire.go 于 checker 后注册（非 RegisterBuiltinTools）。
