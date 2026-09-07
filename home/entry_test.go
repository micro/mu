package home

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExpiredInstalledSessionOpensLogin(t *testing.T) {
	w := httptest.NewRecorder()
	Index(w, httptest.NewRequest("GET", "/?from=app", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("PWA entry: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestSignedInEntryUsesHomeAndPreservesOldChatLinks(t *testing.T) {
	const who = "home_entry_redirect"
	if e := auth.Create(&auth.Account{ID: who}); e != nil {
		t.Fatal(e)
	}
	sess, e := auth.CreateSession(who)
	if e != nil {
		t.Fatal(e)
	}
	defer auth.EndSession(sess.Token)
	for _, tc := range []struct{ path, want string }{
		{"/", "/home"}, {"/?from=app", "/home"}, {"/?session=old-thread", "/agent/micro?session=old-thread"}, {"/?new=1", "/agent/micro?new=1"},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Index(w, r)
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != tc.want {
			t.Errorf("%s: %d %s", tc.path, w.Code, w.Header().Get("Location"))
		}
	}
}
