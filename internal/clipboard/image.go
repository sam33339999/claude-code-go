package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"net/http"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	// MaxImageBytes is the maximum size of a normalized image.
	MaxImageBytes = 3_750_000
	// MaxImageEdge is the maximum longer-side length of a normalized image.
	MaxImageEdge = 1568

	jpegQuality = 85
)

// ErrImageTooLarge is returned when an image still exceeds MaxImageBytes
// after it is re-encoded as JPEG.
var ErrImageTooLarge = errors.New("clipboard: image too large")

// ErrImageFormat is returned when data is not a supported image format.
var ErrImageFormat = errors.New("clipboard: unsupported image format")

// NormalizeImage returns attachment-ready image bytes and their media type.
// PNG, JPEG, GIF, and WebP already within MaxImageEdge and MaxImageBytes are
// returned unchanged, which keeps a small animated GIF intact. Every other
// supported image is decoded, scaled with Catmull-Rom when its longer side
// exceeds MaxImageEdge, and encoded as PNG. A PNG over MaxImageBytes is
// re-encoded as JPEG at quality 85. That result still over the limit returns
// ErrImageTooLarge. DecodeConfig failure or an unrecognized format returns
// ErrImageFormat.
func NormalizeImage(data []byte) ([]byte, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !supportedFormat(format) {
		return nil, "", ErrImageFormat
	}
	if passthrough(format, cfg.Width, cfg.Height, len(data)) {
		return data, http.DetectContentType(data), nil
	}
	img, err := decodeImage(data)
	if err != nil {
		return nil, "", err
	}
	return encodeLimited(scaleImage(img))
}

func supportedFormat(format string) bool {
	switch format {
	case "png", "jpeg", "gif", "webp", "tiff", "bmp":
		return true
	default:
		return false
	}
}

func passthrough(format string, w, h, n int) bool {
	switch format {
	case "png", "jpeg", "gif", "webp":
	default:
		return false
	}
	return n <= MaxImageBytes && longerEdge(w, h) <= MaxImageEdge
}

func longerEdge(w, h int) int {
	if w > h {
		return w
	}
	return h
}

func decodeImage(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("clipboard: decode image: %w", err)
	}
	return img, nil
}

func scaleImage(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if longerEdge(w, h) <= MaxImageEdge {
		return img
	}
	nw, nh := scaledEdges(w, h)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func scaledEdges(w, h int) (int, int) {
	long := longerEdge(w, h)
	return roundHalfUp(w, long), roundHalfUp(h, long)
}

// roundHalfUp returns n * MaxImageEdge / long, rounded half up, and at least 1.
func roundHalfUp(n, long int) int {
	v := (int64(n)*MaxImageEdge + int64(long)/2) / int64(long)
	if v < 1 {
		return 1
	}
	return int(v)
}

func encodeLimited(img image.Image) ([]byte, string, error) {
	pngBytes, err := encodePNG(img)
	if err != nil {
		return nil, "", err
	}
	if len(pngBytes) <= MaxImageBytes {
		return pngBytes, "image/png", nil
	}
	jpegBytes, err := encodeJPEG(img)
	if err != nil {
		return nil, "", err
	}
	if len(jpegBytes) <= MaxImageBytes {
		return jpegBytes, "image/jpeg", nil
	}
	return nil, "", ErrImageTooLarge
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("clipboard: encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	opt := &jpeg.Options{Quality: jpegQuality}
	if err := jpeg.Encode(&buf, img, opt); err != nil {
		return nil, fmt.Errorf("clipboard: encode jpeg: %w", err)
	}
	return buf.Bytes(), nil
}
