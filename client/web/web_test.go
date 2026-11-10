package web_test

import (
	"encoding/json"
	web "mu/client/web"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestStandaloneClientAndRuntimeInstallation(t *testing.T) {
	w := httptest.NewRecorder()
	web.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("client not served")
	}
	if strings.Contains(w.Body.String(), "manifest") || strings.Contains(w.Body.String(), "/mu.js") {
		t.Fatal("client must not be a runtime PWA or use runtime scripts")
	}
	for _, path := range []string{"/manifest.webmanifest", "/sw.js", "/no-such-page"} {
		w := httptest.NewRecorder()
		web.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("unexpected route %s", path)
		}
	}
	w = httptest.NewRecorder()
	web.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 405 {
		t.Fatal("client accepted execution")
	}
	root, err := os.ReadFile("../../internal/app/html/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if json.Unmarshal(root, &manifest) != nil || manifest["id"] != "/" || manifest["start_url"] != "/home" {
		t.Fatal("runtime PWA identity changed")
	}
}
