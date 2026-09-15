package test

import (
	"mu/internal/api"
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFirstPartyCallsPreserveIdentityAndCSRF(t *testing.T) {
	registerAll(t)
	loadTools(t)
	const owner = "app_call_security"
	if err := auth.Create(&auth.Account{ID: owner, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	for _, tc := range []struct {
		name, path, method    string
		session, csrf, header bool
		want                  int
	}{
		{"public guest read", "blog/list", "POST", false, false, false, 200},
		{"private guest read", "notes/list", "POST", false, false, false, 401},
		{"session without csrf", "notes/list", "POST", true, false, false, 403},
		{"session with csrf", "notes/list", "POST", true, true, false, 200},
		{"header credentials use API", "notes/list", "POST", true, true, true, 403},
		{"no calls by GET", "blog/list", "GET", false, false, false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/client/call/"+tc.path, strings.NewReader("{}"))
			r.Header.Set("Content-Type", "application/json")
			if tc.session {
				r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
			}
			if tc.csrf {
				r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
			}
			if tc.header {
				r.Header.Set("Authorization", "Bearer fixture")
			}
			w := httptest.NewRecorder()
			api.AppCallHandler(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
