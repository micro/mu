package home

import (
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/tasks"
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
		if strings.Count(body, "My little app") != 1 || !strings.Contains(body, "home-conversations") {
			t.Fatal("recent agent conversation missing")
		}
		if strings.Contains(body, "home-todo-title") {
			t.Fatal("removed Todo card rendered on Home")
		}
		if path == "/home" {
			for _, want := range []string{`data-path="/agent/micro"`, `href="/services"`, `aria-label="Main navigation"`} {
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

func TestActivityRetainsFailedScheduledRuns(t *testing.T) {
	owner := "home-failed-schedule"
	run, err := tasks.RecordOccurrence(owner, "Evening Digest", time.Now(), tasks.Occurrence{Key: "failed-evening", Schedule: "evening", State: "done", Failure: "Could not deliver"}, "")
	if err != nil {
		t.Fatal(err)
	}
	rows := recentActivity(owner)
	if len(rows) != 1 || rows[0].Status != tasks.StatusFailed || !strings.Contains(rows[0].URL, run.ID) {
		t.Fatalf("missing scheduled failure: %+v", rows)
	}
	if err := tasks.ArchiveTask(owner, run.ID, true); err != nil {
		t.Fatal(err)
	}
	if len(recentActivity(owner)) != 0 {
		t.Fatal("dismissed failure remains in activity")
	}
	notice := thread.Open(owner, "mail", "failure-notice")
	thread.Add(thread.Message{Account: owner, Thread: notice.ID, Role: thread.RoleAgent, Text: "Could not deliver", Ref: "failed-digest-notice"})
	if err := tasks.ArchiveTask(owner, run.ID, false); err != nil {
		t.Fatal(err)
	}
	_, err = tasks.RecordOccurrence(owner, "Evening Digest", time.Now(), tasks.Occurrence{Key: "failed-evening", Schedule: "evening", State: "done", Failure: "Could not deliver", MessageID: "failed-digest-notice"}, "")
	if err != nil {
		t.Fatal(err)
	}
	rows = recentActivity(owner)
	if len(rows) != 1 || rows[0].ID != notice.ID || rows[0].Status != tasks.StatusFailed {
		t.Fatalf("failure duplicates notification: %+v", rows)
	}

}
