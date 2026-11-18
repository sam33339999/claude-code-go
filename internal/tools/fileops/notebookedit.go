package fileops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// NotebookEditTool is the exported singleton instance (BF-11: full .ipynb
// cell editing — replace/insert/delete with loose-JSON round trip).
var NotebookEditTool tools.Tool = &notebookEditTool{}

type notebookEditTool struct{ tools.BaseTool }

// NotebookEditInput is the input schema for the NotebookEdit tools.
type NotebookEditInput struct {
	// NotebookPath is the absolute path to the .ipynb file (required).
	NotebookPath string `json:"notebook_path"`
	// CellNumber is the 0-indexed cell to edit (required).
	CellNumber int `json:"cell_number"`
	// NewSource is the new cell source content (required).
	NewSource string `json:"new_source"`
	// CellType is "code" or "markdown" (optional, defaults to current type).
	CellType string `json:"cell_type,omitempty"`
	// EditMode is "replace", "insert", or "delete" (optional, defaults to "replace").
	EditMode string `json:"edit_mode,omitempty"`
}

func (t *notebookEditTool) Name() string { return "NotebookEdit" }

func (t *notebookEditTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Completely replaces the contents of a specific cell in a Jupyter notebook (.ipynb file).
The notebook_path parameter must be an absolute path. The cell_number is 0-indexed.
Use edit_mode=insert to add a new cell. Use edit_mode=delete to delete a cell.`
}

func (t *notebookEditTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"notebook_path": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The absolute path to the Jupyter notebook file",
			}),
			"cell_number": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "The 0-indexed cell number to edit",
			}),
			"new_source": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The new source for the cell",
			}),
			"cell_type": tools.PropSchema(map[string]any{
				"type":        "string",
				"enum":        []string{"code", "markdown"},
				"description": `Cell type: "code" or "markdown"`,
			}),
			"edit_mode": tools.PropSchema(map[string]any{
				"type":        "string",
				"enum":        []string{"replace", "insert", "delete"},
				"description": `Edit mode: "replace" (default), "insert", or "delete"`,
			}),
		},
		[]string{"notebook_path", "cell_number", "new_source"},
	)
}

func (t *notebookEditTool) IsConcurrencySafe(_ tools.Input) bool { return false }
func (t *notebookEditTool) IsReadOnly(_ tools.Input) bool        { return false }

// CheckPermissions denies edits to notebooks inside protected directories
// (see dangerous.go).
func (t *notebookEditTool) CheckPermissions(input tools.Input, _ *tools.UseContext) (tools.PermissionResult, error) {
	if _, reason, dangerous := checkDangerousPath(writeTargetPath(input)); dangerous {
		return tools.PermissionResult{Behavior: tools.PermissionDeny, Reason: reason}, nil
	}
	return tools.PermissionResult{Behavior: tools.PermissionPassthrough}, nil
}

func (t *notebookEditTool) UserFacingName(input tools.Input) string {
	var in NotebookEditInput
	if json.Unmarshal(input, &in) == nil && in.NotebookPath != "" {
		return fmt.Sprintf("NotebookEdit(%s)", in.NotebookPath)
	}
	return "NotebookEdit"
}

// GetPath implements tools.PathTool.
func (t *notebookEditTool) GetPath(input tools.Input) string {
	var in NotebookEditInput
	if json.Unmarshal(input, &in) == nil {
		return expandPath(in.NotebookPath)
	}
	return ""
}

// Call executes the NotebookEdit tool (BF-11): replace/insert/delete a cell
// in a Jupyter .ipynb notebook. The notebook is parsed as loose JSON
// (map[string]json.RawMessage) so unknown top-level and cell fields survive
// the round trip. Source may be a string or []string on input (both are valid
// Jupyter); it is always written as []string split after newlines.
func (t *notebookEditTool) Call(input tools.Input, _ *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in NotebookEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}
	path := expandPath(in.NotebookPath)
	if !filepath.IsAbs(path) {
		return &tools.Result{IsError: true, Content: "notebook_path must be an absolute path"}, nil
	}
	if filepath.Ext(path) != ".ipynb" {
		return &tools.Result{IsError: true, Content: "notebook_path must point to a .ipynb file"}, nil
	}
	mode := in.EditMode
	if mode == "" {
		mode = "replace"
	}
	switch mode {
	case "replace", "insert", "delete":
	default:
		return &tools.Result{IsError: true, Content: fmt.Sprintf("edit_mode must be replace, insert or delete, got %q", mode)}, nil
	}
	if in.CellType != "" && in.CellType != "code" && in.CellType != "markdown" {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cell_type must be code or markdown, got %q", in.CellType)}, nil
	}
	if mode == "insert" && in.CellType == "" {
		return &tools.Result{IsError: true, Content: "cell_type is required when edit_mode=insert"}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("read notebook: %v", err)}, nil
	}
	var nb map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nb); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("parse notebook JSON: %v", err)}, nil
	}
	var cells []map[string]json.RawMessage
	if cellsRaw, ok := nb["cells"]; ok {
		if err := json.Unmarshal(cellsRaw, &cells); err != nil {
			return &tools.Result{IsError: true, Content: fmt.Sprintf("parse cells: %v", err)}, nil
		}
	}

	n := in.CellNumber
	switch mode {
	case "replace", "delete":
		if n < 0 || n >= len(cells) {
			return &tools.Result{IsError: true, Content: fmt.Sprintf("cell_number %d out of range (notebook has %d cells)", n, len(cells))}, nil
		}
	case "insert":
		if n < 0 || n > len(cells) {
			return &tools.Result{IsError: true, Content: fmt.Sprintf("cell_number %d out of range for insert (0..%d)", n, len(cells))}, nil
		}
	}

	switch mode {
	case "replace":
		setCellSource(cells[n], in.NewSource)
		if in.CellType != "" {
			cells[n]["cell_type"], _ = json.Marshal(in.CellType)
			if in.CellType == "markdown" {
				delete(cells[n], "outputs")
				delete(cells[n], "execution_count")
			}
		}
	case "insert":
		cell := map[string]json.RawMessage{
			"cell_type": mustJSON(in.CellType),
			"metadata":  json.RawMessage(`{}`),
			"source":    mustJSON(sourceLines(in.NewSource)),
		}
		if in.CellType == "code" {
			cell["outputs"] = json.RawMessage(`[]`)
			cell["execution_count"] = json.RawMessage(`null`)
		}
		cells = append(cells, nil)
		copy(cells[n+1:], cells[n:])
		cells[n] = cell
	case "delete":
		cells = append(cells[:n], cells[n+1:]...)
	}

	cellsRaw, err := json.Marshal(cells)
	if err != nil {
		return nil, fmt.Errorf("NotebookEdit: marshal cells: %w", err)
	}
	nb["cells"] = cellsRaw
	out, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return nil, fmt.Errorf("NotebookEdit: marshal notebook: %w", err)
	}
	if err := writeFileAtomic(path, append(out, '\n')); err != nil {
		return nil, fmt.Errorf("NotebookEdit: write notebook: %w", err)
	}
	switch mode {
	case "insert":
		return &tools.Result{Content: fmt.Sprintf("Inserted %s cell at index %d in %s", in.CellType, n, path)}, nil
	case "delete":
		return &tools.Result{Content: fmt.Sprintf("Deleted cell %d from %s (%d cells remain)", n, path, len(cells))}, nil
	default:
		return &tools.Result{Content: fmt.Sprintf("Updated cell %d in %s", n, path)}, nil
	}
}

// setCellSource replaces a cell's source with newSource (as []string lines).
func setCellSource(cell map[string]json.RawMessage, newSource string) {
	cell["source"] = mustJSON(sourceLines(newSource))
}

// sourceLines splits s into Jupyter source lines: each line keeps its
// trailing newline except the last. An empty string yields an empty slice.
func sourceLines(s string) []string {
	if s == "" {
		return []string{}
	}
	var lines []string
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
		if s == "" {
			break
		}
	}
	return lines
}

// mustJSON marshals v, panicking only on impossible inputs (string/[]string).
func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("NotebookEdit: marshal %T: %v", v, err))
	}
	return raw
}
