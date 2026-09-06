package saved

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mu/internal/auth"
	store "mu/internal/saved"
	"mu/internal/service"
)

func request(t *testing.T, owner, method, target string, form url.Values) *http.Request {
	t.Helper()
	if _, err := auth.GetAccount(owner); err != nil {
		if err := auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
	return r
}
func TestPageAndToolsSharePrivateCollection(t *testing.T) {
	owner := "saved_page"
	defer DeleteAll(owner)
	w := httptest.NewRecorder()
	Handler(w, request(t, owner, "POST", "/saved", url.Values{"action": {"add"}, "url": {"https://example.com/story"}, "title": {"Source"}, "note": {"private phrase"}}))
	if w.Code != 303 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	var rsp ListResponse
	if err := (Server{}).List(service.WithAccount(context.Background(), owner), &ListRequest{Query: "private phrase"}, &rsp); err != nil || rsp.Total != 1 {
		t.Fatalf("tool cannot retrieve page save: %+v %v", rsp, err)
	}
	id := rsp.Items[0].ID
	for _, who := range []string{"", "another"} {
		var got ItemResponse
		if err := (Server{}).Get(service.WithAccount(context.Background(), who), &GetRequest{ID: id}, &got); err == nil {
			t.Fatal("tool returned another account's item")
		}
	}
	r := request(t, owner, "POST", "/saved/search", url.Values{"query": {"private phrase"}})
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	Handler(w, r)
	var found ListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &found); err != nil || found.Total != 1 {
		t.Fatalf("search failed: %s", w.Body.String())
	}
	// A query in a URL is never accepted as a private search.
	r = request(t, owner, "GET", "/saved?query=not-present", nil)
	r.Header.Set("Accept", "application/json")
	w = httptest.NewRecorder()
	Handler(w, r)
	if err := json.Unmarshal(w.Body.Bytes(), &found); err != nil || found.Total != 1 {
		t.Fatal("read a private search from the URL")
	}
	w = httptest.NewRecorder()
	Handler(w, request(t, owner, "GET", "/saved?id="+id, nil))
	body := w.Body.String()
	if !strings.Contains(body, `name="_csrf"`) {
		t.Fatal("forms lack the recognized CSRF field")
	}
	if !strings.Contains(body, "/agent/micro?saved="+id) || strings.Contains(body, "/chat?id=") {
		t.Fatal("saved material does not lead to private Micro")
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private notes may be cached")
	}
	w = httptest.NewRecorder()
	Handler(w, request(t, "another", "GET", "/saved?id="+id, nil))
	if w.Code != 404 {
		t.Fatalf("other account read the page: %d", w.Code)
	}
}
func TestReturnDestinationIsConstrained(t *testing.T) {
	owner := "saved_redirect"
	defer store.Clear(owner)
	for _, back := range []string{"https://evil.com/news", "//evil.com/news", "/logout", "/admin"} {
		w := httptest.NewRecorder()
		Handler(w, request(t, owner, "POST", "/saved", url.Values{"action": {"add"}, "url": {"https://example.com/"}, "back": {back}}))
		if !strings.HasPrefix(w.Header().Get("Location"), "/saved?id=") {
			t.Fatalf("unsafe redirect: %q", w.Header().Get("Location"))
		}
	}
}

func TestWritesRequireCSRF(t *testing.T) {
	r := request(t, "saved_csrf", "POST", "/saved", url.Values{"action": {"add"}, "url": {"https://example.com"}})
	r.Header.Del("X-CSRF-Token")
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("tokenless mutation accepted: %d", w.Code)
	}
}
