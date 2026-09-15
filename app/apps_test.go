package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuiltInViewsDoNotReplaceServiceData(t *testing.T) {
	next := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
	h := Page(next, "Notes")
	r := httptest.NewRequest("GET", "/notes?id=sample", nil)
	w := httptest.NewRecorder()
	h(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("native service URL did not render its app")
	}
	for _, method := range []string{"GET", "POST"} {
		r = httptest.NewRequest(method, "/notes", nil)
		r.Header.Set("Accept", "application/json")
		w = httptest.NewRecorder()
		h(w, r)
		if w.Body.String() != `{"ok":true}` {
			t.Fatal("data or mutation intercepted")
		}
	}
}
