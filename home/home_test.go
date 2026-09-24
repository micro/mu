package home

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHomeReadsDeliveredBriefFromLocalProjection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "home-brief-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	th := thread.Open(owner, "mail", "morning-brief")
	at := time.Date(2026, 9, 23, 7, 0, 0, 0, time.UTC)
	source := &thread.Source{Service: "mail", ID: "brief-source"}
	thread.Add(thread.Message{Account: owner, Thread: th.ID, To: owner + "+brief@example.test", Text: "Your saved morning information.", At: at, Source: source})
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Text: "A later reply", At: at.Add(time.Hour)})
	foreign := thread.Open("another-owner", "mail", "foreign-brief")
	thread.Add(thread.Message{Account: "another-owner", Thread: foreign.ID, To: owner + "+brief@example.test", Text: "Foreign brief", At: at.Add(2 * time.Hour)})
	r := httptest.NewRequest("GET", "/home", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()
	Handler(w, r)
	body := w.Body.String()
	for _, want := range []string{"Your saved morning information.", "23 September 2026 at 07:00 UTC", `href="/inbox?id=` + th.ID, `data-path="/agent/micro"`, `aria-label="Home"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, bad := range []string{"Foreign brief", "A later reply", `<a href="/home/apps">Apps</a>`} {
		if strings.Contains(body, bad) {
			t.Fatalf("unexpected %q", bad)
		}
	}
	thread.InvalidateSource(owner, "mail", "brief-source")
	if body, _, _ := latestBrief(owner); body != "" {
		t.Fatal("deleted brief remains visible")
	}
	w = httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/home", nil))
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Fatalf("home is not protected: %d", w.Code)
	}
}
