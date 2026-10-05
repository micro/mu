package inbox

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSaveConversationAndFilterOwnership(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "save-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, _ := auth.CreateSession(owner)
	received := thread.Open(owner, "mail", "incoming")
	thread.Add(thread.Message{Account: owner, Thread: received.ID, From: "someone@example.com", Text: "Received only"})
	sent := thread.Open(owner, "mail", "outgoing")
	thread.Add(thread.Message{Account: owner, Thread: sent.ID, Text: "My message"})
	agentOnly := thread.Open(owner, thread.WebClient, "agent-only")
	thread.Add(thread.Message{Account: owner, Thread: agentOnly.ID, Role: thread.RoleAgent, Text: "Update"})
	foreign := thread.Open("save-other", "mail", "foreign")
	send := func(id, action string, csrf bool) *httptest.ResponseRecorder {
		v := url.Values{"id": {id}, "action": {action}}
		r := httptest.NewRequest("POST", "/inbox", strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		Handler(w, r)
		return w
	}
	if w := send(received.ID, "save", false); w.Code != 403 {
		t.Fatal("save bypassed CSRF")
	}
	if w := send(foreign.ID, "save", true); w.Code != 404 {
		t.Fatal("saved foreign conversation")
	}
	before := thread.Get(owner, received.ID)
	if w := send(received.ID, "save", true); w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	after := thread.Get(owner, received.ID)
	if !after.Saved || !after.Seen.Equal(before.Seen) || !after.Updated.Equal(before.Updated) {
		t.Fatal("save changed read or arrival state")
	}
	for filter, want := range map[string]string{"saved": received.ID, "sent": sent.ID} {
		r := httptest.NewRequest("GET", "/inbox?filter="+filter, nil)
		rows := filterThreads(r, inboxThreads(owner, "/inbox"))
		if len(rows) != 1 || rows[0].ID != want {
			t.Fatalf("%s: %+v", filter, rows)
		}
	}
	if w := send(received.ID, "unsave", true); w.Code != 303 || thread.Get(owner, received.ID).Saved {
		t.Fatal("unsave failed")
	}
}
