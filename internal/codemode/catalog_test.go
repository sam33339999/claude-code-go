package codemode

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/tools"
)

type fakeCatalogTool struct {
	tools.BaseTool
	name string
	desc string
}

func (f *fakeCatalogTool) Name() string                                                { return f.name }
func (f *fakeCatalogTool) Description(_ tools.Input, _ tools.PermissionContext) string { return f.desc }
func (f *fakeCatalogTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(map[string]json.RawMessage{}, []string{})
}
func (f *fakeCatalogTool) UserFacingName(_ tools.Input) string  { return f.name }
func (f *fakeCatalogTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (f *fakeCatalogTool) IsReadOnly(_ tools.Input) bool        { return true }
func (f *fakeCatalogTool) Call(_ tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	return &tools.Result{Content: "ok"}, nil
}

func testCatalog() []CatalogEntry {
	reg := tools.NewRegistry()
	reg.Register(&fakeCatalogTool{name: "FileRead", desc: "Reads a file from disk.\nMore detail."})
	reg.Register(&fakeCatalogTool{name: "Grep", desc: "Search file contents with regex."})
	reg.Register(&fakeCatalogTool{name: "Bash", desc: "Run shell commands."})
	return BuildCatalog(reg)
}

func TestBuildCatalog(t *testing.T) {
	entries := testCatalog()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	// sorted by name; description trimmed to first line
	if entries[0].Name != "Bash" || entries[1].Name != "FileRead" {
		t.Errorf("sort order: %v", entries)
	}
	if entries[1].Description != "Reads a file from disk." {
		t.Errorf("description must be first line only, got %q", entries[1].Description)
	}
	if len(entries[1].InputSchema) == 0 {
		t.Error("schema must be captured")
	}
}

func TestSearchRanking(t *testing.T) {
	entries := testCatalog()
	got := Search(entries, "read file")
	if len(got) == 0 || got[0] != "FileRead" {
		t.Errorf("expected FileRead first for 'read file', got %v", got)
	}
	got = Search(entries, "regex search")
	if len(got) == 0 || got[0] != "Grep" {
		t.Errorf("expected Grep first for 'regex search', got %v", got)
	}
	if names := Search(entries, ""); len(names) != 3 {
		t.Errorf("empty query must list all, got %v", names)
	}
	if names := Search(entries, "zzzznothing"); len(names) != 0 {
		t.Errorf("no-match must be empty, got %v", names)
	}
}

func TestDescribe(t *testing.T) {
	entries := testCatalog()
	e, ok := Describe(entries, "fileread") // case-insensitive
	if !ok || e.Name != "FileRead" {
		t.Errorf("describe case-insensitive lookup failed: %+v %v", e, ok)
	}
	if _, ok := Describe(entries, "absent"); ok {
		t.Error("absent tool must not be found")
	}
}

func TestCatalogText(t *testing.T) {
	text := CatalogText(testCatalog())
	for _, want := range []string{"tools.Bash", "tools.FileRead", "tools.Grep"} {
		if !strings.Contains(text, want) {
			t.Errorf("catalog text missing %q", want)
		}
	}
}
