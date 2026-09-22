package home

import (
	"fmt"
	"mu/internal/auth"
	"mu/internal/notes"
	"mu/internal/thread"
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHomeIndexOwnershipAndDestinations(t *testing.T) {
	owner := "home_index_owner"
	foreign := "home_index_foreign"
	notes.Add(foreign, "Foreign secret", "Private body")
	t.Cleanup(func() { notes.Delete(foreign, "Foreign secret"); events.DeleteAll(owner); events.DeleteAll(foreign) })
	for i := 0; i < 8; i++ {
		title := fmt.Sprintf("Note %d <private>", i)
		notes.Add(owner, title, "Body must not appear")
		t.Cleanup(func() { notes.Delete(owner, title) })
	}
	when := time.Date(2035, 1, 2, 12, 0, 0, 0, time.UTC)
	e, err := events.Create(owner, "Appointment <test>", when, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = events.Create(foreign, "Foreign appointment", when, ""); err != nil {
		t.Fatal(err)
	}
	body := homeIndex(owner, "America/New_York")
	if strings.Contains(body, "Foreign") || strings.Contains(body, "Body must not appear") || strings.Contains(body, "<private>") {
		t.Fatal("leaked or unescaped content")
	}
	if !strings.Contains(body, "/events?id="+e.ID) || !strings.Contains(body, "07:00 EST") {
		t.Fatal("event destination or account timezone missing")
	}
	if strings.Count(body, `href="/notes?id=`) != 3 {
		t.Fatal("recent list not limited")
	}
	items := recentItems(owner)
	for i := 1; i < len(items); i++ {
		if items[i].updated.After(items[i-1].updated) {
			t.Fatal("recent order reversed")
		}
	}
	if homeIndex("", "") != "" {
		t.Fatal("anonymous index rendered")
	}
}

func TestHomeIndexOnlyOnStartingHome(t *testing.T) {
	owner := "home_index_session"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	chat := thread.Open(owner, thread.WebClient, "home-index-chat")
	for _, tc := range []struct {
		path           string
		signedIn, want bool
	}{
		{"/", true, true}, {"/?new=1", true, true}, {"/?session=" + chat.ID, true, false}, {"/", false, false},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.signedIn {
			r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		}
		w := httptest.NewRecorder()
		ConsoleHandler(w, r)
		if got := strings.Contains(w.Body.String(), `class="home-index"`); got != tc.want {
			t.Fatalf("%s signedIn=%v: index=%v", tc.path, tc.signedIn, got)
		}
	}
}
