package x402

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/tool"
	"mu/service/news"
)

func hostRequest(method, path string, form url.Values, cookie *http.Cookie) *http.Request {
	r := httptest.NewRequest(method, "https://m3o.test"+path, strings.NewReader(form.Encode()))
	r.Header.Set("Accept", "text/html")
	if method == "POST" {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://m3o.test")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return r
}
func TestHostAccountFlow(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("ADMIN", "operator")
	acc := &auth.Account{ID: "developer_flow", Secret: "test-password", Approved: true}
	if err := auth.Create(acc); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.RemoveAccountForTest(acc.ID) })
	for _, path := range []string{"/account", "/account/tokens", "/account/topup", "/account/usage"} {
		w := httptest.NewRecorder()
		Handler(w, hostRequest("GET", path, nil, nil))
		if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login?redirect=") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	form := url.Values{"id": {acc.ID}, "secret": {"test-password"}, "redirect": {"/account/tokens"}}
	r := hostRequest("POST", "/login", form, nil)
	bad := r.Clone(r.Context())
	bad.Header.Set("Origin", "https://evil.test")
	w := httptest.NewRecorder()
	Handler(w, bad)
	if w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
	// TLS terminated at the proxy, with no forwarded scheme header.
	proxied := hostRequest("POST", "/login", form, nil)
	proxied.TLS = nil
	proxied.URL.Scheme = "http"
	proxied.Header.Set("Sec-Fetch-Site", "same-origin")
	w = httptest.NewRecorder()
	Handler(w, proxied)
	if w.Code != 303 || w.Header().Get("Location") != "/account/tokens" {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil || cookie.Domain != "" || !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("session cookie is not host-only and secure")
	}
	for _, path := range []string{"/account", "/account/tokens", "/account/topup", "/account/usage"} {
		w = httptest.NewRecorder()
		Handler(w, hostRequest("GET", path, nil, cookie))
		if w.Code != 200 || !strings.Contains(w.Body.String(), `class="x402-site"`) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "/agents?") {
			t.Fatal("consumer account navigation leaked")
		}
	}
	// A normal-host cookie is not accepted by the developer host.
	normal := *cookie
	normal.Name = "session"
	w = httptest.NewRecorder()
	Handler(w, hostRequest("GET", "/account", nil, &normal))
	if w.Code != 303 {
		t.Fatal("consumer session accepted")
	}
	// No fallthrough to consumer resources.
	w = httptest.NewRecorder()
	Handler(w, hostRequest("GET", "/home", nil, cookie))
	if w.Code != 404 {
		t.Fatal("consumer route served")
	}
	if _, ok := service.SpecFor(news.Spec.Name); !ok {
		if err := service.Register(news.Spec); err != nil {
			t.Fatal(err)
		}
	}
	tool.DeriveTools()
	create := url.Values{"name": {"Test agent"}}
	w = httptest.NewRecorder()
	Handler(w, hostRequest("POST", "/account/tokens", create, cookie))
	if w.Code != 403 {
		t.Fatal("token created without CSRF")
	}
	req := hostRequest("POST", "/account/tokens", create, cookie)
	create.Set("_csrf", auth.CSRFToken(browserSession(req)))
	w = httptest.NewRecorder()
	Handler(w, hostRequest("POST", "/account/tokens", create, cookie))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Token created") {
		t.Fatalf("create token: %d %s", w.Code, w.Body.String())
	}
	tokens := auth.ListTokens(acc.ID)
	if len(tokens) != 1 || len(tokens[0].Services()) == 0 || tokens[0].ExpiresAt.Before(time.Now()) {
		t.Fatal("missing service-scoped token")
	}
	body := w.Body.String()
	_, raw, ok := strings.Cut(body, `<pre id="new-token">`)
	if !ok {
		t.Fatal("token not shown")
	}
	raw, _, _ = strings.Cut(raw, "</pre>")
	call := httptest.NewRequest("POST", "https://m3o.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("Authorization", "Bearer "+raw)
	w = httptest.NewRecorder()
	Handler(w, call)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "news_") {
		t.Fatalf("token MCP: %d %s", w.Code, w.Body.String())
	}
	session, err := auth.GetSession(api.CredentialRequest(browserSession(call)))
	if err != nil || session.Account != acc.ID {
		t.Fatal("API token did not resolve shared account")
	}
	// An invalid explicit token cannot fall back to an authenticated browser cookie.
	invalid := hostRequest("GET", "/account", nil, cookie)
	invalid.Header.Set("Authorization", "Bearer invalid")
	w = httptest.NewRecorder()
	Handler(w, invalid)
	if w.Code != 401 {
		t.Fatal("explicit invalid credential fell back to cookie")
	}
	logout := url.Values{"_csrf": {auth.CSRFToken(browserSession(hostRequest("GET", "/account", nil, cookie)))}}
	w = httptest.NewRecorder()
	Handler(w, hostRequest("POST", "/logout", logout, cookie))
	if w.Code != 303 {
		t.Fatal("logout failed")
	}
	w = httptest.NewRecorder()
	Handler(w, hostRequest("GET", "/account", nil, cookie))
	if w.Code != 303 {
		t.Fatal("logout did not invalidate session")
	}
}
func TestLocalLoginDestination(t *testing.T) {
	for _, to := range []string{"https://evil.test", "//evil.test", "/\\evil.test", "/home"} {
		r := hostRequest("GET", "/login?redirect="+url.QueryEscape(to), nil, nil)
		if destination(r) != "/account" {
			t.Fatalf("unsafe destination %s", to)
		}
	}
	to := "/account?plan=pro#subscription"
	if destination(hostRequest("GET", "/login?redirect="+url.QueryEscape(to), nil, nil)) != to {
		t.Fatal("plan return lost")
	}
}

func TestBrowserOriginBehindProxy(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	for _, tc := range []struct {
		site, origin string
		allowed      bool
	}{
		{"same-origin", "https://m3o.test", true},
		{"cross-site", "https://evil.test", false},
		{"same-site", "https://other.m3o.test", false},
		{"", "https://evil.test", false},
		{"", "https://m3o.test", true},
	} {
		r := httptest.NewRequest("POST", "http://m3o.test/login", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		if got := browserOriginAllowed(r); got != tc.allowed {
			t.Errorf("site=%q origin=%q: got %v", tc.site, tc.origin, got)
		}
	}
}
