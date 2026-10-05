package home

import (
	"mu/agent/work"
	"mu/internal/auth"
	"mu/service/apps"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/notes"
	"mu/service/tasks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHomeDestinationsAllowOrdinaryAccounts(t *testing.T) {
	t.Setenv("ADMIN", "home_destination_operator")
	const owner = "home_destination_reader"
	if err := auth.Create(&auth.Account{ID: owner, Admin: false}); err != nil {
		t.Fatal(err)
	}
	acc, _ := auth.GetAccount(owner)
	if acc.Admin {
		t.Fatal("fixture must not be an administrator")
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Create(owner, "Review report", "", tasks.Me, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tasks.Remove(owner, task.ID) })
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/work", work.Handler}, {"/work?id=" + task.ID, work.Handler},
		{"/events", events.Handler}, {"/notes", notes.Handler},
		{"/files", files.Handler}, {"/docs", docs.Handler}, {"/apps", apps.Handler},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.path, nil)
			r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("ordinary user received %d: %s", w.Code, w.Body.String())
			}
			body := w.Body.String()
			if !strings.Contains(body, "/mu.css") {
				t.Fatal("missing shared stylesheet")
			}
			if strings.HasPrefix(tc.path, "/work") && (!strings.Contains(body, `class="metadata-row"`) || strings.Contains(body, `class="metadata"`)) {
				t.Fatal("work metadata lacks shared spacing")
			}
		})
	}
}
