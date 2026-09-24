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

func TestHomeOverviewIsPrivateAndDoesNotRepeatDailyBrief(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "home-overview-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true, Pinned: []string{}})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	th := thread.Open(owner, "mail", "morning-brief")
	thread.Add(thread.Message{Account: owner, Thread: th.ID, To: owner + "+brief@example.test", Text: "Today’s scheduled brief. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information. Detailed news and market information.  The end of the daily brief.", At: time.Now()})
	chat := thread.Open(owner, thread.WebClient, "project")
	thread.Name(owner, chat.ID, "My little app")
	thread.MarkSeen(owner, chat.ID)
	foreign := thread.Open("home-other-owner", thread.WebClient, "foreign")
	thread.Name("home-other-owner", foreign.ID, "Foreign secret")
	thread.MarkSeen("home-other-owner", foreign.ID)
	for _, path := range []string{"/home", "/home?view=overview"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Handler(w, r)
		body := w.Body.String()
		if w.Code != 200 || strings.Contains(body, "The end of the daily brief.") || strings.Contains(body, "Foreign secret") {
			t.Fatalf("incorrect overview at %s: %d", path, w.Code)
		}
		if path == "/home" {
			for _, want := range []string{`data-path="/agent/micro"`, `Continue: My little app`, `href="/docs"`, `href="/services"`, `aria-label="Main navigation"`} {
				if !strings.Contains(body, want) {
					t.Fatalf("missing %q", want)
				}
			}
		}
	}
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "/home?view=overview", nil))
	if w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
		t.Fatal("private overview unprotected")
	}
}
