package home

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantReopensOwnedConversation(t *testing.T) {
	const owner = "asstcontinuity"
	if err := auth.Create(&auth.Account{ID: owner, Name: owner}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	mine := thread.Open(owner, thread.WebClient, "assistant-continuity")
	thread.Add(thread.Message{Account: owner, Thread: mine.ID, Text: "My preserved question"})
	other := thread.Open("another-reader", thread.WebClient, "assistant-private")
	for _, tc := range []struct {
		id   string
		want int
	}{{mine.ID, 200}, {other.ID, 404}, {"nonexistent", 404}} {
		r := httptest.NewRequest("GET", "/?session="+tc.id, nil)
		r.Header.Set("Accept", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Index(w, r)

		if w.Code != tc.want {
			t.Fatalf("%s: got %d want %d", tc.id, w.Code, tc.want)
		}
		if tc.want == 200 {
			if !strings.Contains(w.Body.String(), "My preserved question") || strings.Contains(w.Body.String(), `href="/?session=`) {
				t.Fatal("Saved transcript must reopen without history links")
			}
		}
	}
}
