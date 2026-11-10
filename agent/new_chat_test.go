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
		Handler(w, r)
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

func TestNewGroupChatCreatesInvitationsAndOpensChat(t *testing.T) {
	const owner = "new-group-chat-owner"
	const peer = "new-group-chat-peer"
	for _, id := range []string{owner, peer} {
		auth.SetAccountForTest(&auth.Account{ID: id, Approved: true})
		defer auth.RemoveAccountForTest(id)
	}
	session, _ := auth.CreateSession(owner)
	send := func(people, assistant string, csrf bool) *httptest.ResponseRecorder {
		form := url.Values{"group": {"new"}, "name": {"Family"}, "description": {"Plans together"}, "people": {people}, "agent": {assistant}}
		r := httptest.NewRequest("POST", "/agent?view=new-chat", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		return w
	}
	if w := send(peer, "", false); w.Code != 403 {
		t.Fatal("CSRF accepted", w.Code)
	}
	if w := send("unknown-person-for-test", "", true); w.Code != 400 {
		t.Fatal("unknown invitee accepted", w.Code)
	}
	if w := send(peer, "foreign-agent-id", true); w.Code != 400 {
		t.Fatal("foreign agent accepted", w.Code)
	}
	for _, assistant := range []string{"", "none"} {
		w := send("@"+peer, assistant, true)
		if w.Code != 303 {
			t.Fatal(w.Code, w.Body.String())
		}
		target, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		room := target.Query().Get("id")
		g, c, ok := group.ConversationFor(room)
		if !ok || g.Name != "Family" || g.Description != "Plans together" || c.Agent != assistant {
			t.Fatal(g, c, ok)
		}
		defer group.Change(g.ID, owner, "delete", "", "")
		if group.Member(g.ID, peer) {
			t.Fatal("invitation granted membership before acceptance")
		}
		if _, ok := g.Invitations[peer]; !ok {
			t.Fatal("invitation missing")
		}
		if err := group.Change(g.ID, peer, "accept", "", ""); err != nil {
			t.Fatal(err)
		}
		if !group.Member(g.ID, peer) {
			t.Fatal("accepted invitation has no access")
		}
	}
}
