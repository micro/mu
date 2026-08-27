package social

// Deleting a message deletes it, rather than posting another one.
//
// A browser form cannot issue a DELETE, so the corner menu says so with a
// _method=DELETE field on a POST. This handler read that field — below the
// method switch, where nothing could reach it, because "case POST" matches
// every POST there is. So the Delete control created a thread, the answer was
// a 303 like any successful post, and the message you asked to remove was
// still on the page with a new one above it.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

// mine puts one message in the feed, owned by a signed-in account, and hands
// back a request that carries that session.
func mine(t *testing.T) (id string, cookie *http.Cookie) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	const who = "social_deleter"
	if _, err := auth.GetAccount(who); err != nil {
		if err := auth.Create(&auth.Account{ID: who, Name: who, Created: time.Now()}); err != nil {
			t.Fatalf("could not create %s: %v", who, err)
		}
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatalf("no session: %v", err)
	}

	id = "msg_under_test"
	mutex.Lock()
	before := messages
	messages = []*Message{{ID: id, Author: who, AuthorID: who,
		Content: "A message that exists so it can be removed.", PostedAt: time.Now()}}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = before; mutex.Unlock() })

	return id, &http.Cookie{Name: "session", Value: sess.Token}
}

func count(id string) int {
	mutex.RLock()
	defer mutex.RUnlock()
	n := 0
	for _, m := range messages {
		if m.ID == id {
			n++
		}
	}
	return n
}

func TestDeletingAMessageRemovesIt(t *testing.T) {
	id, cookie := mine(t)

	r := httptest.NewRequest(http.MethodPost, "/social?id="+id,
		strings.NewReader("_method=DELETE"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Handler(rec, r)

	if rec.Code >= 400 {
		t.Fatalf("the delete was refused with %d: %s", rec.Code, rec.Body.String())
	}
	if n := count(id); n != 0 {
		t.Errorf("the message is still there after being deleted")
	}
	// The failure this replaced was not a no-op — the post landed on
	// handleCreateThread and added a message. Nothing new may appear.
	mutex.RLock()
	left := len(messages)
	mutex.RUnlock()
	if left != 0 {
		t.Errorf("deleting one message left %d in the feed, so it posted rather "+
			"than deleted", left)
	}
}

// And a POST that is genuinely a new message still is one.
func TestAPlainPostStillPosts(t *testing.T) {
	_, cookie := mine(t)

	r := httptest.NewRequest(http.MethodPost, "/social",
		strings.NewReader("content=Something+worth+saying+here."))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	rec := httptest.NewRecorder()
	Handler(rec, r)

	mutex.RLock()
	left := len(messages)
	mutex.RUnlock()
	if left < 2 {
		t.Errorf("an ordinary post no longer creates a message (feed has %d, "+
			"status %d)", left, rec.Code)
	}
}
