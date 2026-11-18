---
package: codemode
import_path: internal/codemode
layer: unknown
generated_at: 2026-11-18T06:40:49Z
source_files: [catalog.go, moejs.go, runtime.go]
---

# internal/codemode

> Layer: **unknown** · Files: 3 · Interfaces: 1 · Structs: 4 · Functions: 5

## Interfaces

### Runtime (1 methods)
> Runtime executes model-written code in a sandbox. Implementations must be

```go
type Runtime interface {
    Execute(ctx context.Context, code string, call CallFunc, opts Options) (*Result, error)
}
```

## Structs

- **CatalogEntry** — 3 fields: Name, Description, InputSchema
- **MoejsRuntime**
- **Options** — 4 fields: Timeout, MemoryLimit, Search, Describe
- **Result** — 2 fields: Value, Logs

## Function Types

- `CallFunc` — `func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error)`

## Functions

- `BuildCatalog(reg *tools.Registry) []CatalogEntry`
- `CatalogText(entries []CatalogEntry) string`
- `Describe(entries []CatalogEntry, name string) (CatalogEntry, bool)`
- `NewMoejsRuntime() *MoejsRuntime`
- `Search(entries []CatalogEntry, query string) []string`

## Constants

- `DefaultMemoryLimit`
- `DefaultTimeout`

## Change Impact

**Exported type references (files that use types from this package):**
- `CallFunc` → `internal/tools/codemode/tool.go`
- `CatalogEntry` → `internal/tools/codemode/tool.go`
- `Options` → `internal/tools/codemode/tool.go`
- `Result` → `internal/tools/codemode/tool.go`
- `Runtime` → `internal/tools/codemode/tool.go`

## Dependencies

**Imports:** `internal/tools`

**Imported by:** `internal/bootstrap`, `internal/tools/codemode`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
