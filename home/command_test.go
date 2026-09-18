package home

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
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
		ConsoleHandler(w, r)
		return w
	}
	w := request("/?session=" + chat.ID)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "My saved conversation") || !strings.Contains(body, `id="assistant-history"`) || !strings.Contains(body, `<textarea id="command-input"`) {
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
