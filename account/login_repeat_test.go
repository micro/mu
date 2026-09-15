package account

import (
	"mu/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSignedInLoginReturnsToDestination(t *testing.T) {
	const who = "repeat_login"
	if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "secret"}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/login?redirect=%2Finbox", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	w := httptest.NewRecorder()
	Login(w, r)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/inbox" {
		t.Fatalf("got %d %s", w.Code, w.Header().Get("Location"))
	}
	form := accountFormValues(`<form></form>`, r)
	if !strings.Contains(form, `name="_csrf"`) || !strings.Contains(form, auth.CSRFToken(r)) {
		t.Fatal("authenticated form omits CSRF token")
	}
}
