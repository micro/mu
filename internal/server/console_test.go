package server

import (
	"net/http/httptest"
	"testing"
)

func TestRetiredPagesOnlyPrefillCommands(t *testing.T) {
	for _, path := range []string{"/home", "/inbox", "/admin", "/admin/config", "/admin/log"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		if !consoleRedirect(w, r) || w.Code != 303 {
			t.Fatalf("%s did not converge on console", path)
		}
		if location := w.Header().Get("Location"); len(location) < 2 || location[:2] != "/#" {
			t.Fatalf("unexpected destination %s", location)
		}
		r.Header.Set("Accept", "application/json")
		if consoleRedirect(httptest.NewRecorder(), r) {
			t.Fatal("JSON API redirected")
		}
		r.Method = "POST"
		r.Header.Del("Accept")
		if consoleRedirect(httptest.NewRecorder(), r) {
			t.Fatal("mutation redirected")
		}
	}
}
