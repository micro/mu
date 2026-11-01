package app

import (
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestShellAssetsUseContentVersions(t *testing.T) {
	for _, name := range []string{"mu.css", "mu.js"} {
		source, err := htmlFiles.ReadFile("html/" + name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(source)
		expected := "/" + name + "?v=" + fmt.Sprintf("%x", digest[:8])
		for _, page := range []string{ConsoleHTML("Home", "", nil), RenderIndex(Index{Title: "Micro"})} {
			if !strings.Contains(page, expected) {
				t.Fatalf("shell does not version %s by its contents", name)
			}
		}
	}
}

func TestStylesOnlyCacheMatchingContentVersion(t *testing.T) {
	for _, tc := range []struct{ url, cache string }{
		{"/mu.css", "no-cache"},
		{"/mu.css?v=layout-142", "no-cache"},
		{"/mu.css?v=" + stylesVersion, "public, max-age=86400"},
	} {
		w := httptest.NewRecorder()
		serveStyles(w, httptest.NewRequest("GET", tc.url, nil))
		if got := w.Header().Get("Cache-Control"); got != tc.cache {
			t.Errorf("%s: cache=%q", tc.url, got)
		}
		if w.Body.String() != Styles() {
			t.Fatal("unexpected stylesheet")
		}
	}
}
