package clipboard

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func (r *Reader) readImageDarwin(ctx context.Context) ([]byte, error) {
	f, err := os.CreateTemp(r.TempDir, "glaude-clip-*.png")
	if err != nil {
		return nil, fmt.Errorf("clipboard: create temp file: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("clipboard: close temp file: %w", err)
	}

	_, err = r.run(ctx, "osascript",
		"-e", "set d to (the clipboard as «class PNGf»)",
		"-e", darwinWriteScript(path),
		"-e", "set eof f to 0",
		"-e", "write d to f",
		"-e", "close access f",
	)
	if err != nil {
		return nil, ErrNoImage
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("clipboard: read temp file: %w", err)
	}
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	return data, nil
}

func darwinWriteScript(path string) string {
	return `set f to open for access POSIX file "` + escapeAppleScriptString(path) + `" with write permission`
}

// escapeAppleScriptString escapes characters that terminate or break a
// double-quoted AppleScript string. Backslash is escaped first by scanning
// each byte once, so an inserted \" is not escaped again.
func escapeAppleScriptString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func (r *Reader) readTextDarwin(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "pbpaste")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
