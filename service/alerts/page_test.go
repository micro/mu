package alerts

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	store "mu/internal/alerts"
	"mu/internal/auth"
)

func TestPageRequiresSessionCSRFAndEscapesContent(t *testing.T) {
	const actor = "alerts-page-test"
	auth.SetAccountForTest(&auth.Account{ID: actor, Name: actor})
	defer auth.RemoveAccountForTest(actor)
	defer store.Clear(actor)
	sess, err := auth.CreateSession(actor)
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := store.Raise(actor, store.Item{Source: "custom", Key: "one", Title: `<script>alert(1)</script>`, Detail: `<img src=x onerror=alert(1)>`, URL: "https://example.com/"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method string, csrf bool) *http.Request {
		vals := url.Values{"id": {item.ID}, "state": {"acknowledged"}}
		r := httptest.NewRequest(method, "/alerts", strings.NewReader(vals.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/alerts", nil))
	if w.Code == 200 {
		t.Fatal("anonymous page access")
	}
	w = httptest.NewRecorder()
	Handler(w, request("GET", false))
	if w.Code != 200 || strings.Contains(w.Body.String(), item.Title) || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("unsafe rendering", w.Code)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private page cacheable")
	}
	w = httptest.NewRecorder()
	Handler(w, request("POST", false))
	if w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	got, _ := store.Get(actor, item.ID)
	if got.State != "open" {
		t.Fatal("CSRF changed record")
	}
	w = httptest.NewRecorder()
	Handler(w, request("POST", true))
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	got, _ = store.Get(actor, item.ID)
	if got.State != "acknowledged" {
		t.Fatal("action did not persist")
	}
}
