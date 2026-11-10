package web_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	web "mu/client/web"
)

func TestClientAssetsAndInstallationIdentity(t *testing.T) {
	for _, path := range []string{"/", "/client.js", "/sw.js", "/manifest.webmanifest"} {
		w := httptest.NewRecorder()
		web.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	web.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 405 {
		t.Fatal("static client accepted a write")
	}
	w = httptest.NewRecorder()
	web.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/no-such-page", nil))
	if w.Code != 404 {
		t.Fatal("unknown paths must not silently open client")
	}
	data, err := os.ReadFile("static/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if json.Unmarshal(data, &manifest) != nil {
		t.Fatal("invalid manifest")
	}
	if manifest["id"] != "./" || manifest["scope"] != "./" || manifest["start_url"] != "./" {
		t.Fatal("client must stay within its own installation scope")
	}
	root, err := os.ReadFile("../../internal/app/html/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(root, &manifest) != nil || manifest["id"] != "/" || manifest["start_url"] != "/home" {
		t.Fatal("existing full-web PWA identity changed")
	}
	script, err := os.ReadFile("static/client.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"innerHTML", "DOMParser", "/assistant", "localStorage", "sessionStorage"} {
		if strings.Contains(string(script), forbidden) {
			t.Fatalf("client must not use %s", forbidden)
		}
	}
}
