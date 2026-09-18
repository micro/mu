package account

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAppPasswordIsProtocolOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    []string
		want bool
	}{
		{"mail", []string{"read", "write", "protocol:mail"}, true},
		{"chat", []string{"read", "write", "protocol:chat"}, true},
		{"both", []string{"read", "write", "protocol:mail", "protocol:chat"}, true},
		{"legacy", []string{"read", "write"}, false},
		{"all", []string{"all", "protocol:mail"}, false},
		{"api", []string{"read", "write", "protocol:mail", "api:agent"}, false},
		{"service", []string{"read", "service:mail"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := appPassword(&auth.Token{Permissions: tc.p}); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestAppPasswordRequestGuards(t *testing.T) {
	const owner = "password_test_owner"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Owner", Admin: true, Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	request := func(values url.Values, csrf bool) *http.Request {
		r := httptest.NewRequest("POST", "/account/app-password", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	values := url.Values{"name": {"My phone"}, "client": {"mail"}}
	w := httptest.NewRecorder()
	AppPasswordHandler(w, request(values, false))
	if w.Code != http.StatusForbidden || len(auth.ListTokens(owner)) != 0 {
		t.Fatal("missing CSRF created a credential")
	}
	values.Set("client", "api")
	w = httptest.NewRecorder()
	AppPasswordHandler(w, request(values, true))
	if w.Code != http.StatusBadRequest || len(auth.ListTokens(owner)) != 0 {
		t.Fatal("app-password flow granted API access")
	}
	values.Set("client", "mail")
	w = httptest.NewRecorder()
	AppPasswordHandler(w, request(values, true))
	tokens := auth.ListTokens(owner)
	if w.Code != http.StatusOK || len(tokens) != 1 || !appPassword(tokens[0]) {
		t.Fatalf("mail connection failed: status %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("password response may be cached")
	}
	id := tokens[0].ID
	w = httptest.NewRecorder()
	AppPasswordHandler(w, request(url.Values{"disconnect": {id}}, true))
	if w.Code != http.StatusSeeOther || len(auth.ListTokens(owner)) != 0 {
		t.Fatal("disconnect failed")
	}
}
