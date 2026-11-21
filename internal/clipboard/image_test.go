package clipboard

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"

	"golang.org/x/image/tiff"
)

func TestNormalize_SmallPNGUnchanged(t *testing.T) {
	data := mustPNG(t, 100, 50)
	out, media, err := NormalizeImage(data)
	if err != nil {
		t.Fatalf("NormalizeImage: %v", err)
	}
	if media != "image/png" {
		t.Fatalf("media = %q, want image/png", media)
	}
	if !bytes.Equal(out, data) {
		t.Fatal("output bytes differ from input")
	}
}

func TestNormalize_DownscalesLongEdge(t *testing.T) {
	data := mustPNG(t, 3000, 1000)
	out, _, err := NormalizeImage(data)
	if err != nil {
		t.Fatalf("NormalizeImage: %v", err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("DecodeConfig: %v", err)
	}
	if format != "png" || cfg.Width != 1568 || cfg.Height != 523 {
		t.Fatalf("got %s %dx%d, want png 1568x523", format, cfg.Width, cfg.Height)
	}
}

func TestNormalize_TIFFConvertedToPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	img.SetRGBA(1, 1, color.RGBA{R: 255, G: 128, B: 64, A: 255})
	var buf bytes.Buffer
	if err := tiff.Encode(&buf, img, nil); err != nil {
		t.Fatalf("tiff encode: %v", err)
	}
	out, media, err := NormalizeImage(buf.Bytes())
	if err != nil {
		t.Fatalf("NormalizeImage: %v", err)
	}
	if media != "image/png" {
		t.Fatalf("media = %q, want image/png", media)
	}
	if bytes.Equal(out, buf.Bytes()) {
		t.Fatal("tiff was returned unchanged")
	}
}

func TestNormalize_NoiseFallsBackToJPEG(t *testing.T) {
	data := noisePNG(t, MaxImageEdge, MaxImageEdge)
	if len(data) <= MaxImageBytes {
		t.Fatalf("png = %d bytes, want > %d", len(data), MaxImageBytes)
	}
	out, media, err := NormalizeImage(data)
	if err != nil {
		t.Fatalf("NormalizeImage: %v", err)
	}
	if media != "image/jpeg" {
		t.Fatalf("media = %q, want image/jpeg", media)
	}
	if len(out) > MaxImageBytes {
		t.Fatalf("jpeg = %d bytes, want <= %d", len(out), MaxImageBytes)
	}
}

func TestNormalize_Garbage(t *testing.T) {
	_, _, err := NormalizeImage([]byte("not an image"))
	if !errors.Is(err, ErrImageFormat) {
		t.Fatalf("err = %v, want ErrImageFormat", err)
	}
}

func mustPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func noisePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for i := range img.Pix {
		img.Pix[i] = uint8(rng.Intn(256))
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}
