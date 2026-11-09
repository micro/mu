package agent_test

import (
	"mu/agent"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFocusedAssistantOwnershipAndNavigation(t *testing.T) {
	const owner = "focused-client-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	own := thread.Open(owner, thread.WebClient, "focused-own")
	thread.Name(owner, own.ID, "My focused chat")
	other := thread.Open("focused-other", thread.WebClient, "focused-private")
	thread.Name("focused-other", other.ID, "Other account secret")
	mail := thread.Open(owner, "mail", "focused-mail")
	thread.Name(owner, mail.ID, "Mail must stay in inbox")
	get := func(path string, signed bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if signed {
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		w := httptest.NewRecorder()
		agent.AssistantHandler(w, r)
		return w
	}
	for _, path := range []string{"/assistant?view=agents", "/assistant?view=chats", "/assistant?view=account", "/assistant/micro?new=1", "/assistant/micro?session=" + own.ID} {
		w := get(path, true)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		for _, absent := range []string{`class="runtime-navigation"`, `id="mobile-navigation"`, "Other account secret", "Mail must stay in inbox"} {
			if strings.Contains(w.Body.String(), absent) {
				t.Fatalf("%s exposed %s", path, absent)
			}
		}
		if w := get(path, false); w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/login?redirect=") {
			t.Fatalf("login return missing: %s", path)
		}
	}
	if w := get("/assistant/micro?session="+other.ID, true); w.Code != 404 {
		t.Fatal("foreign chat accessible")
	}
	w := get("/assistant/micro?session="+own.ID, true)
	for _, want := range []string{`href="/assistant" class="brand"`, `data-path="/assistant/micro"`, `data-voice-controls`, `href="/assistant/micro?new=1"`, "My focused chat"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %s", want)
		}
	}
	w = get("/assistant?view=chats", true)
	if !strings.Contains(w.Body.String(), "/assistant/micro?session="+own.ID) {
		t.Fatal("history leaves client")
	}
	w = get("/assistant?session="+own.ID, true)
	if w.Header().Get("Location") != "/assistant/micro?session="+own.ID {
		t.Fatal("entry lost thread")
	}
	for _, path := range []string{"/assistant?view=unknown", "/assistant/no-such-agent", "/assistant/micro?view=chats"} {
		if get(path, true).Code != 404 {
			t.Fatalf("unexpected view accepted %s", path)
		}
	}
	r := httptest.NewRequest("POST", "/assistant/micro", strings.NewReader("prompt=test"))
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w = httptest.NewRecorder()
	agent.AssistantHandler(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatal("client route accepted execution")
	}
}
