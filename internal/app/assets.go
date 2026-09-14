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

// Styles is the native UI and app component stylesheet, assembled from shared modules.
func Styles() string {
	stylesOnce.Do(func() {
		var b strings.Builder
		for _, name := range []string{"mu.css", "components.css", "composition.css"} {
			data, err := htmlFiles.ReadFile("html/" + name)
			if err != nil {
				panic(err)
			}
			b.Write(data)
			b.WriteByte('\n')
		}
		b.WriteString(conversationCSS)
		stylesText = b.String()
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
