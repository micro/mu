package agent_test

import (
	"mu/agent"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAssistantHomeOwnsWebThreads(t *testing.T) {
	const owner = "home_workspace_test"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	chat := thread.Open(owner, thread.WebClient, "home-chat-test")
	thread.Name(owner, chat.ID, "My saved conversation")
	mail := thread.Open(owner, "mail", "home-mail-test")
	thread.Name(owner, mail.ID, "Private mail")
	other := thread.Open("someone_else", thread.WebClient, "other-chat")
	thread.Name("someone_else", other.ID, "Foreign conversation")
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		w := httptest.NewRecorder()
		agent.ConsoleHandler(w, r)
		return w
	}
	w := request("/agent/micro?session=" + chat.ID)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "My saved conversation") || strings.Contains(body, `id="assistant-history"`) || !strings.Contains(body, `<textarea id="command-input"`) {
		t.Fatal("missing workspace")
	}
	if strings.Contains(body, "Private mail") || strings.Contains(body, "Foreign conversation") {
		t.Fatal("history mixed account or channel")
	}
	if strings.Index(body, `id="responses"`) > strings.Index(body, `id="command-form"`) {
		t.Fatal("composer is above transcript")
	}
	if request("/?session="+other.ID).Code != 404 {
		t.Fatal("foreign thread exposed")
	}
	w = request("/?session=" + mail.ID)
	if w.Code != 303 || w.Header().Get("Location") != "/inbox?id="+mail.ID {
		t.Fatal("mail rendered as assistant chat")
	}
}

func TestThreadTitleOwnershipAndCSRF(t *testing.T) {
	const owner = "title_owner_test"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	own := thread.Open(owner, thread.WebClient, "title-own")
	foreign := thread.Open("other_title_owner", thread.WebClient, "title-foreign")
	mail := thread.Open(owner, "mail", "title-mail")
	get := httptest.NewRequest("GET", "/", nil)
	get.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
	token := auth.CSRFToken(get)
	for _, tc := range []struct {
		id, title, csrf string
		want            int
	}{
		{own.ID, "My project", token, 200}, {foreign.ID, "Private", token, 404}, {mail.ID, "Mail", token, 404},
		{own.ID, "Blocked", "", 403}, {own.ID, "", token, 400}, {own.ID, strings.Repeat("x", 161), token, 400},
	} {
		values := url.Values{"action": {"rename-thread"}, "thread": {tc.id}, "title": {tc.title}, "_csrf": {tc.csrf}}
		r := httptest.NewRequest("POST", "/", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		w := httptest.NewRecorder()
		agent.ConsoleHandler(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body.String())
		}
	}
	if thread.Get(owner, own.ID).Subject != "My project" {
		t.Fatal("title not preserved")
	}
}

func TestAgentPageKeepsAddressWithoutHistory(t *testing.T) {
	const owner = "agent_history_owner"
	if err := auth.Create(&auth.Account{ID: owner, Admin: true}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := agent.CreateAgent(owner, "Malten", agent.Hosted, "Help", "A specialist", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	own := thread.Open(owner, thread.WebClient, "malten-thread")
	thread.SetAgent(owner, own.ID, a.ID)
	thread.Name(owner, own.ID, "Malten only")
	other := thread.Open(owner, thread.WebClient, "micro-thread")
	thread.Name(owner, other.ID, "Micro only")
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		w := httptest.NewRecorder()
		agent.ConsoleHandler(w, r)
		return w
	}
	path := agent.Path(owner, a.ID)
	w := get(path)
	body := w.Body.String()
	if w.Code != 200 || strings.Contains(body, "Malten only") || strings.Contains(body, "Micro only") || !strings.Contains(body, `data-path="`+path+`"`) || !strings.Contains(body, "A specialist") {
		t.Fatal("agent page lost identity or mixed history")
	}
	w = get("/")
	if strings.Contains(w.Body.String(), "Malten only") || strings.Contains(w.Body.String(), "Micro only") {
		t.Fatal("home history mixed agents")
	}
	w = get("/?session=" + own.ID)
	if w.Code != 303 || w.Header().Get("Location") != path+"?session="+own.ID {
		t.Fatal("old thread link lost agent address")
	}
	w = get("/agent/micro?session=" + own.ID)
	if w.Code != 303 || w.Header().Get("Location") != path+"?session="+own.ID {
		t.Fatal("wrong agent thread accepted")
	}
}
