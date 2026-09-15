package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPagePreparesViewDataWithoutCachingJSON(t *testing.T) {
	calls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/account", func(w http.ResponseWriter, r *http.Request) {
		if Page(w, r, "Account") {
			return
		}
		calls++
		if r.Method != "GET" || r.Header.Get("Accept") != "application/json" {
			t.Error("incorrect initial read")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"</script><script>bad()</script>"}`))
	})
	w := httptest.NewRecorder()
	WithData(mux).ServeHTTP(w, httptest.NewRequest("GET", "/account", nil))
	if calls != 1 || !strings.Contains(w.Body.String(), `id="client-data"`) {
		t.Fatal("missing prepared view data")
	}
	if strings.Contains(w.Body.String(), `<script>bad()`) {
		t.Fatal("view data escaped script boundary")
	}
	r := httptest.NewRequest("GET", "/account", nil)
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	WithData(mux).ServeHTTP(w, r)
	if w.Header().Get("Vary") != "Accept" || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("JSON could replace the page in cache")
	}
}

func TestAdminInitialDataKeepsSelectedView(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/log", func(w http.ResponseWriter, r *http.Request) { Page(w, r, "Logs") })
	mux.HandleFunc("/admin/client", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") != "log" || r.URL.Query().Get("tab") != "mail" {
			t.Errorf("lost selected log view: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"messages":[]}`))
	})
	w := httptest.NewRecorder()
	WithData(mux).ServeHTTP(w, httptest.NewRequest("GET", "/admin/log?tab=mail", nil))
	if !strings.Contains(w.Body.String(), `"messages":[]`) {
		t.Fatal("mail log data missing")
	}
}
