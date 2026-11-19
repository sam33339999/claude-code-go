package fileops_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tunsuy/glaude/internal/tools"
	"github.com/tunsuy/glaude/internal/tools/fileops"
)

// ── NotebookEditTool ──────────────────────────────────────────────────────────

func TestNotebookEditTool_Name(t *testing.T) {
	if fileops.NotebookEditTool.Name() != "NotebookEdit" {
		t.Errorf("expected NotebookEdit, got %q", fileops.NotebookEditTool.Name())
	}
}

func TestNotebookEditTool_IsConcurrencySafe_False(t *testing.T) {
	if fileops.NotebookEditTool.IsConcurrencySafe(nil) {
		t.Error("NotebookEditTool must not be concurrency-safe")
	}
}

func TestNotebookEditTool_IsReadOnly_False(t *testing.T) {
	if fileops.NotebookEditTool.IsReadOnly(nil) {
		t.Error("NotebookEditTool must not be read-only")
	}
}

func TestNotebookEditTool_InputSchema_RequiredFields(t *testing.T) {
	schema := fileops.NotebookEditTool.InputSchema()
	for _, key := range []string{"notebook_path", "cell_number", "new_source"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("schema missing %q", key)
		}
	}
	reqMap := make(map[string]bool)
	for _, r := range schema.Required {
		reqMap[r] = true
	}
	for _, key := range []string{"notebook_path", "cell_number", "new_source"} {
		if !reqMap[key] {
			t.Errorf("expected %q in Required", key)
		}
	}
}

func TestNotebookEditTool_InputSchema_OptionalFields(t *testing.T) {
	schema := fileops.NotebookEditTool.InputSchema()
	for _, key := range []string{"cell_type", "edit_mode"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("schema missing optional field %q", key)
		}
	}
}

func TestNotebookEditTool_UserFacingName(t *testing.T) {
	in, _ := json.Marshal(fileops.NotebookEditInput{
		NotebookPath: "/tmp/nb.ipynb",
		CellNumber:   0,
		NewSource:    "print('hello')",
	})
	name := fileops.NotebookEditTool.UserFacingName(in)
	if name != "NotebookEdit(/tmp/nb.ipynb)" {
		t.Errorf("unexpected name: %q", name)
	}
}

func TestNotebookEditTool_UserFacingName_NoInput(t *testing.T) {
	if fileops.NotebookEditTool.UserFacingName(nil) != "NotebookEdit" {
		t.Error("expected fallback 'NotebookEdit'")
	}
}

// writeTestNotebook creates a .ipynb with the given cells JSON and returns its path.
func writeTestNotebook(t *testing.T, cellsJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nb.ipynb")
	body := `{"cells":` + cellsJSON + `,"metadata":{"kernelspec":{"name":"python3"}},"nbformat":4,"nbformat_minor":5,"custom_top":"keep-me"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func callNotebook(t *testing.T, in fileops.NotebookEditInput) *tools.Result {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fileops.NotebookEditTool.Call(raw, nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	return result
}

func readCells(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var nb map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nb); err != nil {
		t.Fatal(err)
	}
	var cells []map[string]any
	if err := json.Unmarshal(nb["cells"], &cells); err != nil {
		t.Fatal(err)
	}
	return cells
}

func TestNotebookEditTool_ReplaceCell(t *testing.T) {
	path := writeTestNotebook(t, `[{"cell_type":"code","source":["old\n"],"metadata":{},"outputs":[],"execution_count":1}]`)
	result := callNotebook(t, fileops.NotebookEditInput{NotebookPath: path, CellNumber: 0, NewSource: "print('new')\nprint(2)"})
	if result.IsError {
		t.Fatalf("replace failed: %v", result.Content)
	}
	cells := readCells(t, path)
	src := cells[0]["source"].([]any)
	if len(src) != 2 || src[0] != "print('new')\n" || src[1] != "print(2)" {
		t.Errorf("source lines wrong: %+v", src)
	}
	// Unknown top-level fields survive the round trip.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "keep-me") {
		t.Error("unknown top-level field lost")
	}
}

func TestNotebookEditTool_ReplaceAcceptsStringSource(t *testing.T) {
	// Jupyter allows source as a plain string on input.
	path := filepath.Join(t.TempDir(), "str.ipynb")
	os.WriteFile(path, []byte(`{"cells":[{"cell_type":"code","source":"old code","metadata":{}}],"nbformat":4}`), 0o644)
	result := callNotebook(t, fileops.NotebookEditInput{NotebookPath: path, CellNumber: 0, NewSource: "new"})
	if result.IsError {
		t.Fatalf("string-source notebook rejected: %v", result.Content)
	}
}

func TestNotebookEditTool_InsertAndDelete(t *testing.T) {
	path := writeTestNotebook(t, `[{"cell_type":"code","source":["a"],"metadata":{}}]`)
	result := callNotebook(t, fileops.NotebookEditInput{NotebookPath: path, CellNumber: 0, NewSource: "# intro", CellType: "markdown", EditMode: "insert"})
	if result.IsError {
		t.Fatalf("insert failed: %v", result.Content)
	}
	cells := readCells(t, path)
	if len(cells) != 2 || cells[0]["cell_type"] != "markdown" {
		t.Fatalf("insert at 0 wrong: %+v", cells)
	}
	result = callNotebook(t, fileops.NotebookEditInput{NotebookPath: path, CellNumber: 1, NewSource: "", EditMode: "delete"})
	if result.IsError {
		t.Fatalf("delete failed: %v", result.Content)
	}
	cells = readCells(t, path)
	if len(cells) != 1 || cells[0]["cell_type"] != "markdown" {
		t.Errorf("delete wrong: %+v", cells)
	}
}

func TestNotebookEditTool_MarkdownConversionClearsOutputs(t *testing.T) {
	path := writeTestNotebook(t, `[{"cell_type":"code","source":["x"],"metadata":{},"outputs":[{"text":"out"}],"execution_count":3}]`)
	result := callNotebook(t, fileops.NotebookEditInput{NotebookPath: path, CellNumber: 0, NewSource: "# doc", CellType: "markdown"})
	if result.IsError {
		t.Fatal(result.Content)
	}
	cells := readCells(t, path)
	if _, has := cells[0]["outputs"]; has {
		t.Error("markdown conversion must drop outputs")
	}
	if _, has := cells[0]["execution_count"]; has {
		t.Error("markdown conversion must drop execution_count")
	}
}

func TestNotebookEditTool_Validation(t *testing.T) {
	path := writeTestNotebook(t, `[{"cell_type":"code","source":["a"],"metadata":{}}]`)
	cases := []fileops.NotebookEditInput{
		{NotebookPath: "relative.ipynb", CellNumber: 0, NewSource: "x"},
		{NotebookPath: filepath.Join(t.TempDir(), "f.txt"), CellNumber: 0, NewSource: "x"},
		{NotebookPath: path, CellNumber: 5, NewSource: "x"},
		{NotebookPath: path, CellNumber: -1, NewSource: "x"},
		{NotebookPath: path, CellNumber: 0, NewSource: "x", EditMode: "bogus"},
		{NotebookPath: path, CellNumber: 0, NewSource: "x", EditMode: "insert"}, // missing cell_type
		{NotebookPath: path, CellNumber: 0, NewSource: "x", CellType: "raw"},
	}
	for i, in := range cases {
		if result := callNotebook(t, in); !result.IsError {
			t.Errorf("case %d: expected IsError, got %+v", i, result)
		}
	}
}

func TestNotebookEditTool_Call_InvalidJSON(t *testing.T) {
	result, err := fileops.NotebookEditTool.Call([]byte("not-json"), nil, nil)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
}

func TestNotebookEditTool_Call_WithEditMode(t *testing.T) {
	path := writeTestNotebook(t, `[{"cell_type":"code","source":["a"],"metadata":{}},{"cell_type":"code","source":["b"],"metadata":{}}]`)
	result := callNotebook(t, fileops.NotebookEditInput{
		NotebookPath: path,
		CellNumber:   1,
		NewSource:    "# new cell",
		CellType:     "markdown",
		EditMode:     "insert",
	})
	if result.IsError {
		t.Fatalf("insert failed: %v", result.Content)
	}
	if cells := readCells(t, path); len(cells) != 3 {
		t.Errorf("expected 3 cells after insert, got %d", len(cells))
	}
}

func TestNotebookEditTool_ImplementsToolInterface(t *testing.T) {
	var _ tools.Tool = fileops.NotebookEditTool
}
