package clipboard

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

const linuxToolHint = "Install wl-clipboard or xclip to paste from the clipboard"

var preferredImageTypes = []string{"image/png", "image/jpeg", "image/gif", "image/webp"}

func (r *Reader) readImageLinux(ctx context.Context) ([]byte, error) {
	tool, err := r.linuxTool()
	if err != nil {
		return nil, err
	}
	if tool == "wl-paste" {
		return r.readWaylandImage(ctx)
	}
	return r.readXclipImage(ctx)
}

func (r *Reader) readTextLinux(ctx context.Context) (string, error) {
	tool, err := r.linuxTool()
	if err != nil {
		return "", err
	}
	var out []byte
	if tool == "wl-paste" {
		out, err = r.run(ctx, "wl-paste", "--no-newline")
	} else {
		out, err = r.run(ctx, "xclip", "-selection", "clipboard", "-o")
	}
	if err != nil {
		if emptyClipboardExit(err) {
			return "", nil
		}
		return "", err
	}
	return string(out), nil
}

func (r *Reader) linuxTool() (string, error) {
	if r.Getenv("WAYLAND_DISPLAY") != "" && r.hasTool("wl-paste") {
		return "wl-paste", nil
	}
	if r.hasTool("xclip") {
		return "xclip", nil
	}
	return "", &MissingToolError{Hint: linuxToolHint}
}

func (r *Reader) hasTool(name string) bool {
	_, err := r.LookPath(name)
	return err == nil
}

func (r *Reader) readWaylandImage(ctx context.Context) ([]byte, error) {
	out, err := r.run(ctx, "wl-paste", "--list-types")
	if err != nil {
		if emptyClipboardExit(err) {
			return nil, ErrNoImage
		}
		return nil, err
	}
	mime, ok := pickImageType(string(out))
	if !ok {
		return nil, ErrNoImage
	}
	return r.run(ctx, "wl-paste", "--type", mime)
}

func (r *Reader) readXclipImage(ctx context.Context) ([]byte, error) {
	out, err := r.run(ctx, "xclip", "-selection", "clipboard", "-t", "TARGETS", "-o")
	if err != nil {
		if emptyClipboardExit(err) {
			return nil, ErrNoImage
		}
		return nil, err
	}
	mime, ok := pickImageType(string(out))
	if !ok {
		return nil, ErrNoImage
	}
	return r.run(ctx, "xclip", "-selection", "clipboard", "-t", mime, "-o")
}

// emptyClipboardExit reports a non-zero wl-paste or xclip exit whose stderr
// means the selection is empty. Other failures, including a later --type read,
// stay wrapped so a dead display server still surfaces as a read error.
func emptyClipboardExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	stderr := string(exitErr.Stderr)
	return strings.Contains(stderr, "Nothing is copied") ||
		strings.Contains(stderr, "No selection") ||
		strings.Contains(stderr, "no owner")
}

func pickImageType(raw string) (string, bool) {
	have := make(map[string]struct{})
	first := ""
	for _, line := range strings.Split(raw, "\n") {
		mime := strings.TrimSpace(line)
		if mime == "" {
			continue
		}
		have[mime] = struct{}{}
		if first == "" && strings.HasPrefix(mime, "image/") {
			first = mime
		}
	}
	for _, mime := range preferredImageTypes {
		if _, ok := have[mime]; ok {
			return mime, true
		}
	}
	if first == "" {
		return "", false
	}
	return first, true
}
