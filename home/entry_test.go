package home

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRootOffersAssistantAndSignedInOverview(t *testing.T) {
	const who = "overview_entry"
	if err := auth.Create(&auth.Account{ID: who, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	defer auth.EndSession(sess.Token)
	guest := httptest.NewRecorder()
	Index(guest, httptest.NewRequest("GET", "/", nil))
	if guest.Code != 200 || !strings.Contains(guest.Body.String(), "Type a command or ask a question.") || strings.Count(guest.Body.String(), `id="command-input"`) != 1 {
		t.Fatal("guest landing lost its assistant")
	}
	for _, path := range []string{"/", "/?new=1"} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Index(w, req)
		if w.Code != 200 || strings.Count(w.Body.String(), `id="command-input"`) != 1 || strings.Contains(w.Body.String(), `id="nav-container"`) {
			t.Fatalf("one command surface required: %s %d", path, w.Code)
		}
	}
}
