package inbox

import (
	"mu/internal/thread"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPriorityShowsOneCommunicationAndHandledTimestampDoesNotHideNewMessages(t *testing.T) {
	const owner = "priority_reader"
	first := arrived(t, owner, "mail", "first", "", "first@example.com", "First arrival")
	second := arrived(t, owner, "mail", "second", "", "second@example.com", "Second arrival")
	said(t, owner, thread.WebClient, "chat", "", "A web conversation")
	render := func() string {
		w := httptest.NewRecorder()
		priority(w, httptest.NewRequest("GET", "/inbox", nil), owner)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("private priority can be cached")
		}
		return w.Body.String()
	}
	body := render()
	if !strings.Contains(body, "Second arrival") || strings.Contains(body, "First arrival") || strings.Contains(body, "A web conversation") {
		t.Fatal("priority is a mixed feed")
	}
	reviewed := thread.Get(owner, second.ID).Updated
	thread.HandleAt(owner, second.ID, reviewed)
	if !strings.Contains(render(), "First arrival") {
		t.Fatal("Done did not reveal next communication")
	}
	thread.Add(thread.Message{Account: owner, Thread: second.ID, From: "second@example.com", Text: "A new reply"})
	thread.HandleAt(owner, second.ID, reviewed)
	if !strings.Contains(render(), "A new reply") {
		t.Fatal("stale Done hid new communication")
	}
	if thread.Get("other", first.ID) != nil {
		t.Fatal("cross-account thread visible")
	}
}
