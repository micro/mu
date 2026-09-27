package agent_test

import (
	"mu/agent"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/mail"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCheckinContinuesOwnedHistoryOnce(t *testing.T) {
	owner := "checkin-flow-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	source := thread.Open(owner, "mail", "checkin-fixture")
	thread.Add(thread.Message{Account: owner, Thread: source.ID, Role: thread.RolePerson, Text: "How&rsquo;s your day?", From: "agent@" + mail.ConfiguredDomain(), To: owner + "+checkin@example.test", Ref: "initial"})
	request := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/checkin?id="+url.QueryEscape(id), nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		w := httptest.NewRecorder()
		agent.CheckinHandler(w, r)
		return w
	}
	w := request(source.ID)
	if w.Code != 303 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	u, _ := url.Parse(w.Header().Get("Location"))
	id := u.Query().Get("session")
	messages := thread.Messages(owner, id, 100)
	if len(messages) != 1 || messages[0].Text != "How’s your day?" || messages[0].Role != thread.RoleAgent {
		t.Fatalf("bad import: %+v", messages)
	}
	request(source.ID)
	if len(thread.Messages(owner, id, 100)) != 1 {
		t.Fatal("duplicate history")
	}
	r := httptest.NewRequest("GET", w.Header().Get("Location"), nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	page := httptest.NewRecorder()
	agent.ConsoleHandler(page, r)
	if !strings.Contains(page.Body.String(), ">Check in</button>") || !strings.Contains(page.Body.String(), `id="command-form"`) {
		t.Fatal("missing live check-in composer")
	}
	foreign := thread.Open("other-checkin-owner", "mail", "private")
	if request(foreign.ID).Code != 404 {
		t.Fatal("foreign history exposed")
	}
}
