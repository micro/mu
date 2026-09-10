package events

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestPreviewCombinesOwnedAndExternalEventsAndLimitsRows(t *testing.T) {
	const owner = "previewowner"
	defer DeleteAll(owner)
	defer DeleteAll("previewother")
	now := time.Now()
	Create(owner, "Local reminder", now.Add(2*time.Hour), "")
	Create("previewother", "Someone else's reminder", now.Add(time.Hour), "")
	old := ExternalEntries
	defer func() { ExternalEntries = old }()
	ExternalEntries = func(got string, from, to time.Time, limit int) []External {
		if got != owner {
			t.Errorf("wrong calendar owner %q", got)
		}
		return []External{
			{Title: "Earlier <meeting>", Start: now.Add(time.Hour)},
			{Title: "Later meeting", Start: now.Add(3 * time.Hour)},
			{Title: "Not in preview", Start: now.Add(4 * time.Hour)},
		}
	}
	body := Preview(owner, ExternalEvents(owner, now, now.Add(30*24*time.Hour), PreviewLimit))
	for _, want := range []string{"Earlier &lt;meeting&gt;", "Local reminder", "Later meeting", "Go to events"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "Someone else's") || strings.Contains(body, "Not in preview") {
		t.Fatal("wrong events in preview")
	}
	if strings.Index(body, "Earlier &lt;") > strings.Index(body, "Local reminder") {
		t.Fatal("events not chronological")
	}
	if Preview("", nil) != "" {
		t.Fatal("guest preview exposed")
	}
}

func TestCalendarSelectionRequiresSessionAndCSRF(t *testing.T) {
	const owner = "calendarform"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		signed bool
		token  string
		code   int
	}{{false, "", 303}, {true, "", 403}, {true, "bad", 403}} {
		form := url.Values{"action": {"calendars"}, "calendars": {"private-calendar"}, "_csrf": {tc.token}, "owner": {"someoneelse"}}
		req := httptest.NewRequest("POST", "/events", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if tc.signed {
			req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		rec := httptest.NewRecorder()
		Handler(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("status %d want %d", rec.Code, tc.code)
		}
	}
}
