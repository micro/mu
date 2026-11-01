package app

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Content versions make every shell request new assets when a build changes
// them, without relying on somebody remembering to bump a layout counter.
var stylesVersion = assetVersion("mu.css")
var scriptsVersion = assetVersion("mu.js")

func assetVersion(name string) string {
	content, err := htmlFiles.ReadFile("html/" + name)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(content)
	return fmt.Sprintf("%x", digest[:8])
}

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
	w.Header().Set("Cache-Control", "no-cache")
	if r.URL.Query().Get("v") == stylesVersion {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Add("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		body = stylesGzip
		w.Header().Set("Content-Encoding", "gzip")
	}
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
