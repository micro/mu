package agent

import (
	"mu/internal/auth"
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCustomScheduleCreateAndCancelOwnOnly(t *testing.T) {
	owner := "custom-schedule-test"
	other := "custom-schedule-other"
	for _, id := range []string{owner, other} {
		auth.SetAccountForTest(&auth.Account{ID: id, Approved: true, Zone: "UTC"})
		defer auth.RemoveAccountForTest(id)
		defer events.DeleteAll(id)
	}
	send := func(id string, values url.Values, csrf bool) *httptest.ResponseRecorder {
		session, err := auth.CreateSession(id)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("POST", "/agents?view=scheduled", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		if csrf {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		RosterHandler(w, r)
		return w
	}
	values := url.Values{"action": {"create-schedule"}, "title": {"Topic update"}, "prompt": {"Research my topic"}, "when": {"2030-07-22T10:00"}, "zone": {"Europe/London"}, "repeat": {"weekly"}}
	if w := send(owner, values, false); w.Code != 403 {
		t.Fatalf("missing CSRF: %d", w.Code)
	}
	if len(events.List(owner)) != 0 {
		t.Fatal("invalid request created schedule")
	}
	if w := send(owner, values, true); w.Code != 303 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	schedules := events.List(owner)
	if len(schedules) != 1 {
		t.Fatalf("schedules: %d", len(schedules))
	}
	e := schedules[0]
	if e.Owner != owner || e.Zone != "Europe/London" || e.When.UTC().Hour() != 9 || e.Prompt != "Research my topic" || e.Repeat != "weekly" {
		t.Fatalf("wrong saved settings: %+v", e)
	}
	cancel := url.Values{"action": {"cancel-schedule"}, "id": {e.ID}}
	if w := send(other, cancel, true); w.Code != 404 {
		t.Fatalf("foreign cancel: %d", w.Code)
	}
	if len(events.List(owner)) != 1 {
		t.Fatal("foreign cancellation changed schedule")
	}
	if w := send(owner, cancel, true); w.Code != 303 {
		t.Fatalf("owner cancel: %d", w.Code)
	}
	if len(events.List(owner)) != 0 {
		t.Fatal("schedule not cancelled")
	}
}
