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
		if !tc.signed && (!strings.Contains(w.Body.String(), "data-open-assistant") || !strings.Contains(w.Body.String(), `href="/pricing"`)) {
			t.Fatal("landing lacks assistant entry or pricing navigation")
		}
	}
}

func TestLandingOffersClients(t *testing.T) {
	body := landingHTML()
	for _, removed := range []string{"Today’s brief", "A short preview", "Activate your assistant", "From $12", "landing-scheduled", "landing-activation"} {
		if strings.Contains(body, removed) {
			t.Fatalf("landing still contains %q", removed)
		}
	}
	for _, link := range []string{`href="/app/"`, `href="/download#android"`, `href="/download"`, `href="/install"`, `href="/developers"`} {
		if !strings.Contains(body, link) {
			t.Fatalf("missing entry %s", link)
		}
	}
	if strings.Contains(body, "data-demo-form") {
		t.Fatal("landing still opens an anonymous demo")
	}
}

func TestRootProtocolDispatch(t *testing.T) {
	for _, tc := range []struct {
		method, protocol string
		code             int
	}{
		{"GET", "mucp/1", 200}, {"GET", "mucp/99", 400}, {"GET", "", 400},
		{"POST", "mucp/99", 400}, {"POST", "mucp/1", 401},
	} {
		r := httptest.NewRequest(tc.method, "/", strings.NewReader("{}"))
		r.Header.Set("Micro-Protocol", tc.protocol)
		r.Header.Set("Micro-Service", "assistant")
		r.Header.Set("Micro-Endpoint", "Assistant.ListAgents")
		r.Header.Set("Micro-Id", "root-request-12345678")
		r.Header.Set("Content-Type", "application/json")
		if !productClientRequest(r) {
			t.Fatal("root protocol missed credential normalization")
		}
		w := httptest.NewRecorder()
		IndexHandler(w, r)
		if w.Code != tc.code || !strings.Contains(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Location") != "" {
			t.Fatalf("%s %q: %d %s", tc.method, tc.protocol, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	if productClientRequest(r) {
		t.Fatal("ordinary root POST gained protocol token access")
	}
}
