package thumbnail

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mu/internal/blob"
	"testing"
)

func TestPreviewIsBoundedCachedAndLeavesOriginalIntact(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	src := image.NewRGBA(image.Rect(0, 0, 1600, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1600; x++ {
			src.Set(x, y, color.RGBA{uint8(x % 255), uint8(y % 255), 100, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	original := buf.Bytes()
	if err := blob.Put("preview-original.png", original, "image/png"); err != nil {
		t.Fatal(err)
	}
	small, err := Get("preview-original.png", original)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(small))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || cfg.Width != 640 || cfg.Height != 320 {
		t.Fatalf("wrong preview: %s %+v", format, cfg)
	}
	cached, err := Get("preview-original.png", nil)
	if err != nil || !bytes.Equal(cached, small) {
		t.Fatal("preview not reused")
	}
	saved, err := blob.Get("preview-original.png")
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatal("original changed")
	}
	if _, err := Get("invalid", []byte("not an image")); err == nil {
		t.Fatal("invalid image accepted")
	}
}
