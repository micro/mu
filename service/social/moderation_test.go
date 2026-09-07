package social

import (
	"mu/internal/flag"
	"mu/internal/snapshot"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAbusiveCachedThreadIsHiddenAcrossReaders(t *testing.T) {
	mine(t)
	if err := flag.AdminFlag("social", "unsafe-cached", "system:harmful"); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	old := messages
	messages = []*Message{
		{ID: "unsafe-cached", Author: "Imported", AuthorID: "_system", Content: "Bring it on you little fuckers", PostedAt: time.Now()},
		{ID: "ordinary-cached", Author: "Reader", AuthorID: "reader", Content: "An ordinary update", PostedAt: time.Now()},
	}
	mutex.Unlock()
	t.Cleanup(func() { mutex.Lock(); messages = old; mutex.Unlock() })
	if got := FeedText(20); strings.Contains(got, "fuckers") || !strings.Contains(got, "ordinary") {
		t.Fatal(got)
	}
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/social", nil))
	if strings.Contains(w.Body.String(), "fuckers") {
		t.Fatal("feed rendered abuse")
	}
	for _, accept := range []string{"text/html", "application/json"} {
		r := httptest.NewRequest("GET", "/social/thread?id=unsafe-cached", nil)
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		ThreadHandler(w, r)
		if w.Code != 404 || strings.Contains(w.Body.String(), "fuckers") {
			t.Fatalf("direct thread leaked for %s", accept)
		}
	}
	if err := flag.Approve("social", "unsafe-cached"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(FeedText(20), "fuckers") {
		t.Fatal("operator approval did not restore the post")
	}
}

func TestModerationRefreshesTheHomeCard(t *testing.T) {
	oldSnap := cardSnap
	cardSnap = snapshot.New("social-moderation-test")
	t.Cleanup(func() { cardSnap = oldSnap })
	id, _ := mine(t)
	flag.RegisterDeleter("social", moderationStore{})
	moderationStore{}.RefreshCache()
	if !strings.Contains(CardHTML(), "A message that exists") {
		t.Fatal("initial card missing message")
	}
	if err := flag.AdminFlag("social", id, "operator"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for strings.Contains(CardHTML(), "A message that exists") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if strings.Contains(CardHTML(), "A message that exists") {
		t.Fatal("hidden message remained in cached card")
	}
	if err := flag.Approve("social", id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(CardHTML(), "A message that exists") {
		t.Fatal("approval did not restore cached card")
	}
}
