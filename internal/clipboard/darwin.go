package clipboard

import (
	"context"
	"fmt"
	"os"
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
		"-e", `set f to open for access POSIX file "`+path+`" with write permission`,
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

func (r *Reader) readTextDarwin(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "pbpaste")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
