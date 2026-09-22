package account

import (
	"mu/internal/auth"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoginCarriesBlogReturnThroughMethods(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test")
	const destination = "/blog?view=archive&page=2"
	for _, key := range []string{"redirect", "next"} {
		path := "/login?" + url.Values{key: {destination}}.Encode()
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		Login(w, r)
		body := w.Body.String()
		for _, prefix := range []string{`action="/login`, `href="/signup`, `href="/oauth2/google`} {
			if !strings.Contains(body, prefix+`?redirect=`+url.QueryEscape(destination)+`"`) {
				t.Fatalf("%s lost return in %s", key, prefix)
			}
		}
	}
	const id = "blog_return_login"
	if err := auth.Create(&auth.Account{ID: id, Secret: "test-password", SecretSet: true}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ to, want string }{{destination, destination}, {"https://example.org/", "/"}, {"//example.org/", "/"}, {"/\\example.org/", "/"}} {
		r := httptest.NewRequest("POST", "/login?redirect="+url.QueryEscape(tc.to), strings.NewReader(url.Values{"id": {id}, "secret": {"test-password"}}.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		Login(w, r)
		if w.Code != 302 || w.Header().Get("Location") != tc.want {
			t.Fatalf("login returned %d %q", w.Code, w.Header().Get("Location"))
		}
	}
}
