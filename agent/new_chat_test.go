package agent

import (
	"mu/internal/auth"
	"mu/internal/group"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewSharedChatAccess(t *testing.T) {
	owner := "new-shared-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	session, _ := auth.CreateSession(owner)
	g, err := group.Create(owner, "Plans", false)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := group.Create("new-shared-other", "Private", false)
	if err != nil {
		t.Fatal(err)
	}
	send := func(id string, csrf bool) *httptest.ResponseRecorder {
		form := url.Values{"group": {id}, "name": {"Weekend"}}
		r := httptest.NewRequest("POST", "/agent?view=new-chat", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		ConsoleHandler(w, r)
		return w
	}
	if w := send(g.ID, false); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if w := send(foreign.ID, true); w.Code != 404 {
		t.Fatal("foreign group accepted", w.Code)
	}
	if w := send(g.ID, true); w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/chat?id="+group.RoomPrefix+g.ID+"_") {
		t.Fatal(w.Code, w.Body.String())
	}
}
