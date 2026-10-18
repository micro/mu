package server

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLandingHomeAndLegacyConversationRoutes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "landing-route-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	th := thread.Open(owner, thread.WebClient, "legacy-root")
	foreign := thread.Open("another-owner", thread.WebClient, "foreign-root")
	for _, tc := range []struct {
		path     string
		signed   bool
		code     int
		location string
	}{
		{"/", false, 200, ""}, {"/", true, 303, "/home"},
		{"/?session=" + th.ID, true, 303, "/agent/micro?session=" + th.ID},
		{"/?continue=" + th.ID, true, 303, "/agent/micro?session=" + th.ID},
		{"/?session=" + foreign.ID, true, 404, ""},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.signed {
			r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		}
		w := httptest.NewRecorder()
		IndexHandler(w, r)
		if w.Code != tc.code || w.Header().Get("Location") != tc.location {
			t.Fatalf("%s: %d %q", tc.path, w.Code, w.Header().Get("Location"))
		}
		if !tc.signed && (!strings.Contains(w.Body.String(), "data-demo-form") || !strings.Contains(w.Body.String(), `href="/pricing"`)) {
			t.Fatal("landing lacks demo or pricing navigation")
		}
	}
}

func TestLandingOnlyOffersChat(t *testing.T) {
	body := landingHTML()
	for _, removed := range []string{"Today’s brief", "A short preview", "Activate your assistant", "From $12", "landing-scheduled", "landing-activation"} {
		if strings.Contains(body, removed) {
			t.Fatalf("landing still contains %q", removed)
		}
	}
	if !strings.Contains(body, "data-demo-form") {
		t.Fatal("chat removed")
	}
}
