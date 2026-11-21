---
package: acptest
import_path: internal/acpclient/acptest
layer: services
generated_at: 2026-11-21T09:43:04Z
source_files: [agent.go]
---

# internal/acpclient/acptest

> Layer: **Services** · Files: 1 · Interfaces: 0 · Structs: 1 · Functions: 2

## Structs

- **Agent** — 8 fields

## Functions

- `New() *Agent`
- `Pipe(ctx context.Context) (agent *Agent, stdin io.WriteCloser, stdout io.Reader, done chan any, kill func())`

## Dependencies

**Imports:** *(none — zero-dependency)*

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
