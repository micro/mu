package social

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The feed has an upper bound, and counting the messages under each thread does
// not cost a walk of every message per row.
//
// /social rendered every thread ever started, and asked replyCount for each one
// — and replyCount walks the whole message list, taking the lock to do it. Rows
// times messages, both of which only ever grow.
func TestTheFeedShowsOnePageOfThreads(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mutex.Lock()
	messages = nil
	for i := 0; i < feedPerPage*2; i++ {
		id := fmt.Sprintf("thread-%03d", i)
		messages = append(messages, &Message{
			ID:       id,
			Author:   "ann",
			AuthorID: "ann",
			Content:  "a thread about something",
			PostedAt: time.Now().Add(-time.Duration(i) * time.Minute),
		})
		// One reply each, so the counts are not all zero.
		messages = append(messages, &Message{
			ID:       id + "-reply",
			ReplyTo:  id,
			Author:   "bob",
			AuthorID: "bob",
			Content:  "a reply",
			PostedAt: time.Now(),
		})
	}
	mutex.Unlock()

	body := func(url string) string {
		w := httptest.NewRecorder()
		Handler(w, httptest.NewRequest("GET", url, nil))
		return w.Body.String()
	}

	first := body("/social")
	if shown := strings.Count(first, `class="headline"`); shown != feedPerPage {
		t.Fatalf("the feed shows %d threads, want %d", shown, feedPerPage)
	}
	if !strings.Contains(first, "Older") {
		t.Error("no way to reach the rest of the feed")
	}
	if !strings.Contains(first, "1 message") {
		t.Error("the message count is missing from the rows")
	}

	last := body("/social?page=2")
	if strings.Contains(last, "Older") {
		t.Error("the last page offers a page after it")
	}
	if !strings.Contains(last, "Newer") {
		t.Error("no way back from the last page")
	}
}
