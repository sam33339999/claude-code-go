---
package: config
import_path: internal/config
layer: infra
generated_at: 2026-11-18T03:53:13Z
source_files: [loader.go, mcpstore.go, orderedjson.go, pluginstore.go, settings.go, writer.go]
---

# internal/config

> Layer: **Infra** · Files: 6 · Interfaces: 1 · Structs: 18 · Functions: 19

## Interfaces

### ConfigLoader (1 methods)
> ConfigLoader is the interface for loading layered settings.

```go
type ConfigLoader interface {
    Load() (*LayeredSettings, error)
}
```

## Structs

- **AttributionConfig** — 2 fields: Commit, PR
- **EnvPair** — 2 fields: Key, Value
- **InstalledPluginRecord** — 6 fields: Scope, InstallPath, Version, InstalledAt, LastUpdated, ProjectPath
- **InstalledPlugins** — 2 fields
- **KnownMarketplace** — 4 fields: SourceType, SourcePath, InstallLocation, LastUpdated
- **KnownMarketplaces** — 1 fields
- **LayeredSettings** — 5 fields: User, Project, Local, Policy, Merged
- **Loader** — 2 fields
- **MCPServerEntry** — 2 fields: Name, Raw
- **MCPStore** — 2 fields
- **MarketplaceEntry** — 4 fields: Name, Source, Version, Description
- **MarketplaceManifest** — 6 fields: Name, Description, HasOwner, Plugins, RootDir, ManifestPath
- **OrderedMap** — 2 fields
- **PermissionsConfig** — 6 fields: Allow, Deny, Ask, DefaultMode, DisableBypass, AdditionalDirs
- **PluginManifest** — 4 fields: Name, Version, Description, HasAuthor
- **PluginStore** — 2 fields
- **SettingsJson** — 28 fields: Schema, APIKey, APIKeyHelper, BaseURL, Provider, AWSCredentialExport, AWSAuthRefresh, GCPAuthRefresh, ...
- **WorktreeConfig** — 2 fields: SymlinkDirectories, SparsePaths

## Functions

- `AddPermissionRule(projectDir string, list string, rule string) error`
- `ClaudeJsonPath(homeDir string) string`
- `DesktopConfigServers(homeDir string) ([]MCPServerEntry, error)`
- `LoadMarketplaceManifest(rootDir string) (*MarketplaceManifest, error)`
- `LoadMarketplaceManifestFile(path string) (*MarketplaceManifest, error)`
- `LoadOrderedJSON(path string) (*OrderedMap, error)`
- `LoadPluginManifest(dir string) (*PluginManifest, error)`
- `McpJsonPath(projectDir string) string`
- `NewLoader(homeDir string, projectDir string) *Loader`
- `NewMCPStore(homeDir string, projectDir string) *MCPStore`
- `NewOrderedMap() *OrderedMap`
- `NewPluginStore(homeDir string, projectDir string) *PluginStore`
- `OrphanCache(installPath string) error`
- `PluginNamePart(idOrName string) string`
- `PluginTimestamp(t time.Time) string`
- `ScopeLabel(scope string) string`
- `SettingsLocalPath(projectDir string) string`
- `ValidateMCPScope(scope string) error`
- `WriteOrderedJSONAtomic(path string, doc *OrderedMap) error`

## Constants

- `ClaudeDir`
- `ClaudeLocalDir`
- `MCPScopeLocal`
- `MCPScopeProject`
- `MCPScopeUser`
- `ManagedSettingsFile`
- `PermissionListAllow`
- `PermissionListAsk`
- `PermissionListDeny`
- `PluginScopeLocal`
- `PluginScopeProject`
- `PluginScopeUser`
- `SessionsDir`
- `SettingsFile`
- `SourceLocal`
- `SourcePolicy`
- `SourceProject`
- `SourceUser`
- `StatsFile`
- `TodosDir`

## Change Impact

**Exported type references (files that use types from this package):**
- `EnvPair` → `internal/bootstrap/mcp_health.go`, `internal/bootstrap/mcp_render.go`, `internal/bootstrap/mcp_render_test.go` (test), `internal/bootstrap/mcp_run.go`
- `InstalledPluginRecord` → `internal/bootstrap/plugin_install.go`, `internal/bootstrap/plugin_run.go`
- `InstalledPlugins` → `internal/bootstrap/plugin_run.go`
- `KnownMarketplace` → `internal/bootstrap/plugin_marketplace.go`
- `LayeredSettings` → `internal/bootstrap/bootstrap_test.go` (test), `internal/bootstrap/mcp_connect.go`, `internal/bootstrap/mcp_connect_test.go` (test), `internal/bootstrap/wire.go`
- `MCPServerEntry` → `internal/bootstrap/mcp_health.go`, `internal/bootstrap/mcp_health_test.go` (test), `internal/bootstrap/mcp_list.go`, `internal/bootstrap/mcp_list_test.go` (test), `internal/bootstrap/mcp_render.go` + 2 more
- `MCPStore` → `internal/bootstrap/mcp_cmd_test.go` (test), `internal/bootstrap/mcp_connect.go`, `internal/bootstrap/mcp_connect_test.go` (test), `internal/bootstrap/mcp_list.go`, `internal/bootstrap/mcp_list_test.go` (test) + 2 more
- `MarketplaceEntry` → `internal/bootstrap/plugin_install.go`
- `MarketplaceManifest` → `internal/bootstrap/plugin_install.go`, `internal/bootstrap/plugin_marketplace.go`
- `OrderedMap` → `internal/bootstrap/mcp_health_test.go` (test), `internal/bootstrap/mcp_list.go`, `internal/bootstrap/mcp_list_test.go` (test), `internal/bootstrap/mcp_render_test.go` (test), `internal/bootstrap/mcp_run.go`
- `PluginStore` → `internal/bootstrap/plugin_cmd_test.go` (test), `internal/bootstrap/plugin_run.go`
- `SettingsJson` → `internal/bootstrap/bootstrap_test.go` (test), `internal/bootstrap/mcp_connect_test.go` (test), `internal/state/store.go`

## Dependencies

**Imports:** `pkg/types`

**Imported by:** `internal/bootstrap`, `internal/permissions`, `internal/state`

<!-- AUTO-GENERATED ABOVE — DO NOT EDIT -->
<!-- MANUAL NOTES BELOW — preserved across regeneration -->
