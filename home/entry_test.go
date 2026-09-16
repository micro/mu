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
	if guest.Code != 200 || !strings.Contains(guest.Body.String(), "A personal assistant") || strings.Count(guest.Body.String(), `id="mu-chat-input"`) != 1 {
		t.Fatal("guest landing lost its assistant")
	}
	for _, tc := range []struct{ path, want string }{{"/", "/home"}, {"/?new=1", "/agent/micro?new=1"}, {"/?session=previous", "/agent/micro?session=previous"}} {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Index(w, req)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != tc.want {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Header().Get("Location"))
		}
	}
}
