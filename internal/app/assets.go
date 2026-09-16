package app

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"
	"sync"
)

var stylesOnce sync.Once
var stylesText string
var stylesGzip []byte

// Styles serves the single shared UI stylesheet in html/mu.css.
func Styles() string {
	stylesOnce.Do(func() {
		data, err := htmlFiles.ReadFile("html/mu.css")
		if err != nil {
			panic(err)
		}
		stylesText = string(data)
		var compressed bytes.Buffer
		z := gzip.NewWriter(&compressed)
		_, _ = z.Write([]byte(stylesText))
		_ = z.Close()
		stylesGzip = compressed.Bytes()
	})
	return stylesText
}

func serveStyles(w http.ResponseWriter, r *http.Request) {
	body := []byte(Styles())
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Add("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		body = stylesGzip
		w.Header().Set("Content-Encoding", "gzip")
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
