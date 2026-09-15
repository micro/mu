package home

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootIsTheConversationBeforeAndAfterSignIn(t *testing.T) {
	const who = "v2_entry"
	if err := auth.Create(&auth.Account{ID: who, Approved: true}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(session.Token)
	for _, signedIn := range []bool{false, true} {
		for _, path := range []string{"/", "/?from=app", "/?new=1"} {
			req := httptest.NewRequest("GET", path, nil)
			if signedIn {
				req.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
			}
			w := httptest.NewRecorder()
			Index(w, req)
			if w.Code != 200 || w.Header().Get("Location") != "" {
				t.Fatalf("%s signedIn=%v: %d", path, signedIn, w.Code)
			}
			body := w.Body.String()
			if strings.Count(body, `id="root"`) != 1 {
				t.Fatal("expected one composer")
			}
			for _, obsolete := range []string{`id="home-cards"`, `id="home-agent"`, `mu-chat-continue`, `Continue in Assistant`} {
				if strings.Contains(body, obsolete) {
					t.Errorf("obsolete UI: %s", obsolete)
				}
			}
			if !signedIn && !strings.Contains(body, "A personal AI agent") {
				t.Fatal("missing tagline")
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("conversation must not be cached")
			}
		}
	}
}
