package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func resetCSRFTestState(t *testing.T) (*Account, *Session) {
	t.Helper()

	mutex.Lock()
	oldAccounts := accounts
	oldSessions := sessions
	oldTokens := tokens
	accounts = map[string]*Account{}
	sessions = map[string]*Session{}
	tokens = map[string]*Token{}
	mutex.Unlock()

	acc := &Account{ID: "csrf-user", Name: "csrf_user"}
	sess := &Session{
		ID:      "11111111-1111-1111-1111-111111111111",
		Type:    "account",
		Token:   "MTExMTExMTEtMTExMS0xMTExLTExMTEtMTExMTExMTExMTEx",
		Account: acc.ID,
		Created: time.Now(),
	}

	mutex.Lock()
	accounts[acc.ID] = acc
	sessions[sess.ID] = sess
	mutex.Unlock()

	t.Cleanup(func() {
		mutex.Lock()
		accounts = oldAccounts
		sessions = oldSessions
		tokens = oldTokens
		mutex.Unlock()
	})

	return acc, sess
}

func requestWithSession(t *testing.T, sess *Session) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	return req
}

func TestCSRFTokenRequiresSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if got := CSRFToken(req); got != "" {
		t.Fatalf("CSRFToken without a session = %q, want empty string", got)
	}
}

func TestValidCSRFAllowsUnauthenticatedAndMissingToken(t *testing.T) {
	_, sess := resetCSRFTestState(t)

	if !ValidCSRF(httptest.NewRequest(http.MethodPost, "/", nil)) {
		t.Fatal("ValidCSRF rejected an unauthenticated request")
	}
	if !ValidCSRF(requestWithSession(t, sess)) {
		t.Fatal("ValidCSRF rejected an authenticated request with no submitted token")
	}
}

func TestValidCSRFValidatesHeaderAndFormTokens(t *testing.T) {
	_, sess := resetCSRFTestState(t)
	req := requestWithSession(t, sess)
	token := CSRFToken(req)
	if token == "" {
		t.Fatal("CSRFToken returned empty token for authenticated request")
	}

	headerReq := requestWithSession(t, sess)
	headerReq.Header.Set("X-CSRF-Token", token)
	if !ValidCSRF(headerReq) {
		t.Fatal("ValidCSRF rejected a valid header token")
	}

	formReq := requestWithSession(t, sess)
	formReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formReq.Body = http.NoBody
	formReq.Form = map[string][]string{"_csrf": {token}}
	if !ValidCSRF(formReq) {
		t.Fatal("ValidCSRF rejected a valid form token")
	}

	invalidReq := requestWithSession(t, sess)
	invalidReq.Header.Set("X-CSRF-Token", "invalid")
	if ValidCSRF(invalidReq) {
		t.Fatal("ValidCSRF accepted an invalid header token")
	}
}

func TestSetCSRFCookieUsesSecureForDirectTLS(t *testing.T) {
	_, sess := resetCSRFTestState(t)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	rec := httptest.NewRecorder()

	SetCSRFCookie(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("SetCSRFCookie wrote %d cookies, want 1", len(cookies))
	}
	if !cookies[0].Secure {
		t.Fatal("SetCSRFCookie did not mark cookie secure for direct TLS")
	}
}

func TestSetCSRFCookieUsesSecureForwardedProto(t *testing.T) {
	_, sess := resetCSRFTestState(t)
	req := requestWithSession(t, sess)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	SetCSRFCookie(rec, req)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("SetCSRFCookie wrote %d cookies, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "csrf_token" || cookie.Value == "" {
		t.Fatalf("SetCSRFCookie wrote unexpected cookie: %#v", cookie)
	}
	if !cookie.Secure {
		t.Fatal("SetCSRFCookie did not mark cookie secure for forwarded HTTPS")
	}
	if cookie.HttpOnly {
		t.Fatal("SetCSRFCookie marked cookie HttpOnly; JavaScript must be able to read it")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SetCSRFCookie SameSite = %v, want Strict", cookie.SameSite)
	}
}
