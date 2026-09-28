package inbox

import (
	"mu/internal/thread"
	"net/http/httptest"
	"testing"
)

func TestResourcePathKeepsMailboxesAndOwnerIsolation(t *testing.T) {
	owner := "inbox-resource"
	defer thread.Forget(owner)
	th := thread.Open(owner, thread.WebClient, "test")
	for _, path := range []string{"/inbox/" + th.ID, "/inbox?id=" + th.ID} {
		r, ok := inboxResource(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil), owner)
		if !ok || r.URL.Path != "/inbox" || r.URL.Query().Get("id") != th.ID {
			t.Fatal("not normalized", path)
		}
	}
	r, ok := inboxResource(httptest.NewRecorder(), httptest.NewRequest("GET", "/inbox/research", nil), owner)
	if !ok || r.URL.Path != "/inbox/research" || r.URL.Query().Get("id") != "" {
		t.Fatal("mailbox changed")
	}
	w := httptest.NewRecorder()
	_, ok = inboxResource(w, httptest.NewRequest("GET", "/inbox/"+th.ID+"?id=another", nil), owner)
	if ok || w.Code != 400 {
		t.Fatal("conflicting IDs accepted")
	}
	r, ok = inboxResource(httptest.NewRecorder(), httptest.NewRequest("GET", "/inbox/"+th.ID, nil), "another-owner")
	if !ok || r.URL.Query().Get("id") != th.ID || thread.Get("another-owner", r.URL.Query().Get("id")) != nil {
		t.Fatal("foreign conversation exposed")
	}
}
