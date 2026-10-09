package x402

import (
	"encoding/json"
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBrowserErrorsUseHostShell(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	for _, path := range []string{"/missing", "/tools/missing_tool", "/verify?token=invalid"} {
		r := hostRequest("GET", path, nil, nil)
		w := httptest.NewRecorder()
		Handler(w, r)
		if w.Code < 400 || !strings.Contains(w.Body.String(), `class="x402-site"`) || strings.Count(w.Body.String(), "<h1>") != 1 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), ">Login</a>") || !strings.Contains(w.Body.String(), ">Signup</a>") {
			t.Fatal("wrong navigation labels")
		}
	}
	for _, status := range []int{400, 401, 403, 404, 405, 429, 500, 503} {
		r := app.WithRenderer(hostRequest("GET", "/tools/missing", nil, nil), renderHTML)
		w := httptest.NewRecorder()
		app.Error(w, r, status, "Example error")
		if w.Code != status || !strings.Contains(w.Body.String(), `class="x402-site"`) || strings.Count(w.Body.String(), "<h1>") != 1 {
			t.Fatalf("error %d not themed", status)
		}
	}
}
func TestPlainBrowserErrorAndJSONPreservation(t *testing.T) {
	for _, kind := range []string{"plain", "json", "success"} {
		r := app.WithRenderer(hostRequest("GET", "/account/subscription", nil, nil), renderHTML)
		w := httptest.NewRecorder()
		response := &browserResponse{ResponseWriter: w, request: r}
		switch kind {
		case "plain":
			http.Error(response, "Payment unavailable", 503)
		case "json":
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(400)
			response.Write([]byte(`{"error":"invalid"}`))
		case "success":
			response.Write([]byte("success"))
		}
		response.finish()
		switch kind {
		case "plain":
			if w.Code != 503 || !strings.Contains(w.Body.String(), `class="x402-site"`) || !strings.Contains(w.Body.String(), "Payment unavailable") {
				t.Fatal("plain error not styled")
			}
		case "json":
			if w.Code != 400 || w.Body.String() != `{"error":"invalid"}` {
				t.Fatal("JSON error changed")
			}
		case "success":
			if w.Code != 200 || w.Body.String() != "success" {
				t.Fatal("success response changed")
			}
		}
	}
	r := httptest.NewRequest("GET", "https://m3o.test/missing", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 404 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("API error changed to HTML")
	}
}

func TestSessionRefreshForBrowserWrites(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	acc := &auth.Account{ID: "session_refresh", Secret: "test-password", Approved: true}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.RemoveAccountForTest(acc.ID) })
	session, err := auth.CreateSession(acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookie, Value: session.Token}
	r := hostRequest("GET", "/session", nil, cookie)
	w := httptest.NewRecorder()
	Handler(w, r)
	var state struct {
		Account string
		Type    string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil || state.Account != acc.ID || state.Type != "account" {
		t.Fatalf("session refresh: %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("session response must not be cached")
	}
	csrf := ""
	for _, c := range w.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrf = c.Value
		}
	}
	if csrf == "" {
		t.Fatal("session refresh did not renew CSRF cookie")
	}
	for _, path := range []string{"/stripe/checkout", "/account/crypto", "/account/tokens", "/logout"} {
		req := hostRequest("POST", path, url.Values{"_csrf": {csrf}, "amount": {"0"}}, cookie)
		if auth.StrictCSRF(browserSession(hostRequest("POST", path, nil, cookie))) {
			t.Fatalf("%s accepted a missing CSRF token", path)
		}
		if !auth.StrictCSRF(browserSession(req)) {
			t.Fatalf("%s rejected refreshed session", path)
		}
	}
	w = httptest.NewRecorder()
	Handler(w, hostRequest("GET", "/session", nil, nil))
	if !strings.Contains(w.Body.String(), `"type":"guest"`) {
		t.Fatal("guest session did not return JSON")
	}
}
