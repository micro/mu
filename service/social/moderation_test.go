package social

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAbusiveCachedThreadIsHiddenAcrossReaders(t *testing.T) {
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
}
