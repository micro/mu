package x402

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/origin"
	"mu/internal/service"
	"mu/internal/tool"
	"mu/service/news"
	"mu/x402/billing"
)

func TestSignupVerifyCreditsAndToken(t *testing.T) {
	t.Setenv("X402_HOST", "m3o.test")
	t.Setenv("MU_DOMAIN", "micro.test")
	t.Setenv("ADMIN", "operator")
	t.Setenv("INVITE_ONLY", "false")
	t.Setenv("STRIPE_SECRET_KEY", "test")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "test")
	oldSender := app.EmailSender
	t.Cleanup(func() { app.EmailSender = oldSender })
	var verificationURL string
	app.EmailSender = func(to, subject, plain, html, reply string) error {
		for _, word := range strings.Fields(plain) {
			if strings.HasPrefix(word, "https://") {
				verificationURL = word
				break
			}
		}
		return nil
	}
	w := httptest.NewRecorder()
	Handler(w, hostRequest("GET", "/signup", nil, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "captcha_nonce") || !strings.Contains(w.Body.String(), `class="x402-site"`) {
		t.Fatal("signup form missing")
	}
	c := app.NewCaptchaChallenge()
	var a, b int
	fmt.Sscanf(c.Question, "What is %d + %d?", &a, &b)
	id := "new_developer_flow"
	form := url.Values{"id": {id}, "secret": {"test-password"}, "captcha": {strconv.Itoa(a + b)}, "captcha_nonce": {c.Nonce}, "captcha_ts": {c.Timestamp}, "captcha_sig": {c.Signature}}
	r := hostRequest("POST", "/signup", form, nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.TLS = nil
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w = httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 303 || w.Header().Get("Location") != "/account" {
		t.Fatalf("signup: %d %s", w.Code, w.Body.String())
	}
	t.Cleanup(func() { auth.RemoveAccountForTest(id); billing.DeleteCredits(id) })
	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil || !cookie.Secure || cookie.Domain != "" {
		t.Fatal("signup lost secure host session")
	}
	if billing.SignupRemaining(id) != billing.SignupCredits {
		t.Fatal("signup allowance missing")
	}
	w = httptest.NewRecorder()
	Handler(w, hostRequest("GET", "/account/tokens", nil, cookie))
	if !strings.Contains(w.Body.String(), "Verify your email") {
		t.Fatal("unverified user has no path to token access")
	}
	csrf := auth.CSRFToken(browserSession(hostRequest("GET", "/account", nil, cookie)))
	emailForm := url.Values{"email": {"developer@example.test"}, "_csrf": {csrf}}
	r = hostRequest("POST", "/verify", emailForm, cookie)
	r.TLS = nil
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w = httptest.NewRecorder()
	Handler(w, r)
	if w.Code != 200 || !strings.HasPrefix(verificationURL, "https://m3o.test/verify?") {
		t.Fatalf("verification changed host: %d %q", w.Code, verificationURL)
	}
	parsed, _ := url.Parse(verificationURL)
	w = httptest.NewRecorder()
	Handler(w, hostRequest("GET", parsed.RequestURI(), nil, nil))
	if w.Code != 200 || auth.CheckCredentialAccess(id) != nil {
		t.Fatal("verification did not enable credentials")
	}
	// The top-up ledger is shared by checkout, webhooks and API credit access.
	if err := billing.AddCredits(id, 500, "topup", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := service.SpecFor(news.Spec.Name); !ok {
		if err := service.Register(news.Spec); err != nil {
			t.Fatal(err)
		}
	}
	tool.DeriveTools()
	w = httptest.NewRecorder()
	Handler(w, hostRequest("POST", "/account/tokens", url.Values{"name": {"New agent"}, "_csrf": {csrf}}, cookie))
	_, raw, ok := strings.Cut(w.Body.String(), `<pre id="new-token">`)
	if w.Code != 200 || !ok {
		t.Fatalf("token creation: %d %s", w.Code, w.Body.String())
	}
	raw, _, _ = strings.Cut(raw, "</pre>")
	call := httptest.NewRequest("POST", "http://m3o.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("Authorization", "Bearer "+raw)
	w = httptest.NewRecorder()
	Handler(w, call)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "news_") {
		t.Fatalf("MCP token failed: %d %s", w.Code, w.Body.String())
	}
	sess, err := auth.GetSession(api.CredentialRequest(call))
	if err != nil || billing.Balance(sess.Account) != 500 {
		t.Fatal("token is not using funded account")
	}
	if origin.URL(call) != "https://m3o.test" {
		t.Fatal("proxy scheme broke public endpoints")
	}
	// Fresh account credentials also work after the initial signup session.
	w = httptest.NewRecorder()
	Handler(w, hostRequest("POST", "/login", url.Values{"id": {id}, "secret": {"test-password"}}, nil))
	if w.Code != 303 {
		t.Fatal("new account cannot log in again")
	}
}
