// Package thumbnail stores bounded JPEG previews beside original raster images.
package thumbnail

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"mu/internal/blob"
	"sync"
)

var mu sync.Mutex

const Width = 640

// Get builds at most one thumbnail at a time and reuses the stored result thereafter.
func Get(key string, original []byte) ([]byte, error) {
	mu.Lock()
	defer mu.Unlock()
	cached := key + ".thumb640.jpg"
	if b, err := blob.Get(cached); err == nil {
		return b, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, fmt.Errorf("image dimensions exceed preview limit")
	}
	src, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if w > Width || h > Width {
		if w >= h {
			h = h * Width / w
			w = Width
		} else {
			w = w * Width / h
			h = Width
		}
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	bounds := src.Bounds()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Area sampling preserves fine detail while reducing large source images.
			x0, x1 := x*cfg.Width/w, (x+1)*cfg.Width/w
			y0, y1 := y*cfg.Height/h, (y+1)*cfg.Height/h
			var rr, gg, bb, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, a := src.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					rr += uint64(r + 65535 - a)
					gg += uint64(g + 65535 - a)
					bb += uint64(b + 65535 - a)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(rr / n >> 8), uint8(gg / n >> 8), uint8(bb / n >> 8), 255})
		}
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 78}); err != nil {
		return nil, err
	}
	b := out.Bytes()
	if err = blob.Put(cached, b, "image/jpeg"); err != nil {
		return nil, err
	}
	return b, nil
}
