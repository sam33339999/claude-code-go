package codemode

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// snippetNameOK enforces flat snippet names (same safety shape as controlled
// imports, CM-02 v2): no path separators, no traversal.
func snippetNameOK(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/\\") && !strings.Contains(name, "..")
}

// readSnippet loads one snippet source from dataDir/snippets (CM-03 v3).
func (t *codeModeTool) readSnippet(name string) (string, error) {
	if t.dataDir == "" {
		return "", errNoDataDir
	}
	if !snippetNameOK(name) {
		return "", fmt.Errorf("invalid snippet name %q (flat names only)", name)
	}
	path := filepath.Join(t.dataDir, "snippets", name+".js")
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("snippet %q not found (use codemode.snippets() to list)", name)
	}
	if info.Size() > maxModuleBytes {
		return "", fmt.Errorf("snippet %q exceeds %d bytes", name, maxModuleBytes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read snippet %q: %w", name, err)
	}
	return string(raw), nil
}

// saveSnippet stores code as a named snippet (input save_as, CM-03 v3).
// The FULL code is stored (unlike history's 2000-char cap), so long workflows
// survive replay via codemode.run.
func (t *codeModeTool) saveSnippet(name, code string) error {
	if t.dataDir == "" {
		return errNoDataDir
	}
	if !snippetNameOK(name) {
		return fmt.Errorf("invalid snippet name %q (flat names only)", name)
	}
	if len(code) > maxModuleBytes {
		return fmt.Errorf("code exceeds the %d-byte snippet limit", maxModuleBytes)
	}
	dir := filepath.Join(t.dataDir, "snippets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create snippet directory: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, name+".js"), []byte(code), 0o644)
}

// listSnippets returns the available snippet names (sorted; empty when the
// directory does not exist yet).
func (t *codeModeTool) listSnippets() ([]string, error) {
	if t.dataDir == "" {
		return nil, errNoDataDir
	}
	entries, err := os.ReadDir(filepath.Join(t.dataDir, "snippets"))
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".js") {
			names = append(names, strings.TrimSuffix(e.Name(), ".js"))
		}
	}
	sort.Strings(names)
	return names, nil
}
