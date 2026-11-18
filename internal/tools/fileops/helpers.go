// Package fileops implements file-operation tools: Read, Write, Edit, Glob, Grep, NotebookEdit.
package fileops

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tunsuy/claude-code-go/internal/tools"
)

// blockedDevicePaths is the list of device files that must never be read.
var blockedDevicePaths = map[string]bool{
	"/dev/zero":    true,
	"/dev/random":  true,
	"/dev/urandom": true,
	"/dev/full":    true,
	"/dev/stdin":   true,
	"/dev/tty":     true,
	"/dev/console": true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
	"/dev/fd/0":    true,
	"/dev/fd/1":    true,
	"/dev/fd/2":    true,
}

// imageExtensions lists file extensions treated as binary images.
var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".webp": true, ".svg": true, ".ico": true,
	".tiff": true, ".tif": true,
}

// expandPath converts a raw path from the model into an absolute OS path,
// resolving relative paths from the process working directory. Callers with
// a tools.UseContext should prefer expandPathFrom with the session WorkingDir.
func expandPath(raw string) string {
	return expandPathFrom(raw, "")
}

// expandPathFrom converts a raw path from the model into an absolute OS path.
// It handles:
//   - "~" home-directory prefix
//   - Already-absolute paths (returned as-is after cleaning)
//   - Relative paths (resolved from workDir, or the process CWD when empty)
//   - Windows-style backslashes converted to forward slashes on non-Windows
func expandPathFrom(raw, workDir string) string {
	if raw == "" {
		return raw
	}
	// Normalise separators on non-Windows
	if runtime.GOOS != "windows" {
		raw = strings.ReplaceAll(raw, "\\", "/")
	}
	// Expand leading ~
	if strings.HasPrefix(raw, "~/") || raw == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			raw = filepath.Join(home, raw[1:])
		}
	}
	if !filepath.IsAbs(raw) {
		base := workDir
		if base == "" {
			if wd, err := os.Getwd(); err == nil {
				base = wd
			}
		}
		if base != "" {
			raw = filepath.Join(base, raw)
		}
	}
	return filepath.Clean(raw)
}

// workDirOf extracts the session working directory from a UseContext
// ("" when unset, meaning tools fall back to the process CWD).
func workDirOf(ctx *tools.UseContext) string {
	if ctx == nil {
		return ""
	}
	return ctx.WorkingDir
}

// isBlockedDevicePath returns true if the given absolute path is a blocked
// device file.
func isBlockedDevicePath(path string) bool {
	return blockedDevicePaths[path]
}

// isImageExtension returns true for image file extensions.
func isImageExtension(ext string) bool {
	return imageExtensions[strings.ToLower(ext)]
}
