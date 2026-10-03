package agent

import (
	"fmt"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMomentScheduleIsOptionalOwnedAndDurable(t *testing.T) {
	const owner = "moment-owner"
	defer events.DeleteAll(owner)
	auth.SetAccountForTest(&auth.Account{ID: owner, Name: "Reader", Zone: "Europe/London"})
	defer auth.RemoveAccountForTest(owner)
	if Moment(owner) != nil {
		t.Fatal("automatically enrolled")
	}
	if err := scheduleCheckin(owner, "09:00", "Europe/London", "daily", false); err != nil {
		t.Fatal(err)
	}
	checkin := Checkin(owner)
	if err := scheduleInvitation(owner, "moment", "14:00", "Europe/London", "weekdays", false); err != nil {
		t.Fatal(err)
	}
	e := Moment(owner)
	if e == nil || e.When.In(mustLondon(t)).Hour() != 14 || e.Advance.Recipient != "agent" {
		t.Fatal("wrong schedule", e)
	}
	if Moment("other") != nil || Checkin(owner).ID != checkin.ID {
		t.Fatal("schedule ownership or independence lost")
	}
	var stored []*events.Event
	if err := data.LoadJSON("events.json", &stored); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range stored {
		if item.ID == e.ID && item.Kind == "moment" {
			found = true
		}
	}
	if !found {
		t.Fatal("schedule not persisted")
	}
	handled, answer, err := RunScheduled(owner, e.ID, fmt.Sprint(e.Sequence), time.Now())
	if err != nil || !handled || answer == "" {
		t.Fatalf("deterministic delivery failed: %v %s", err, answer)
	}
	_, tag, topic := ScheduledDelivery(owner, e.ID, answer)
	if tag != "moment" || topic != "moment" {
		t.Fatal("wrong delivery routing")
	}
	if allowed, err := reserveScheduled(e, time.Now().Add(-2*time.Hour), time.Now()); err != nil || allowed {
		t.Fatal("missed reminder replayed")
	}
	if err := scheduleInvitation(owner, "moment", "15:00", "Europe/London", "daily", true); err != nil {
		t.Fatal(err)
	}
	if !Moment(owner).Paused || Moment(owner).ID != e.ID {
		t.Fatal("pause lost identity")
	}
	if allowed, err := reserveScheduled(e, time.Now(), time.Now()); err != nil || allowed {
		t.Fatal("old revision accepted")
	}
	for _, tc := range []struct{ clock, zone, repeat string }{{"bad", "Europe/London", "daily"}, {"14:00", "bad", "daily"}, {"14:00", "Europe/London", "hourly"}} {
		if err := scheduleInvitation(owner, "moment", tc.clock, tc.zone, tc.repeat, false); err == nil {
			t.Fatal("invalid setting accepted")
		}
	}
}

func TestMomentSettingsRequireCSRF(t *testing.T) {
	const owner = "moment-settings-owner"
	defer events.DeleteAll(owner)
	auth.SetAccountForTest(&auth.Account{ID: owner})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, valid := range []bool{false, true} {
		values := url.Values{"action": {"moment-schedule"}, "state": {"active"}, "clock": {"14:00"}, "zone": {"Europe/London"}, "repeat": {"daily"}}
		r := httptest.NewRequest("POST", "/agents?view=scheduled", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		if valid {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		w := httptest.NewRecorder()
		scheduledHandler(w, r)
		if !valid && (w.Code != 403 || Moment(owner) != nil) {
			t.Fatal("missing CSRF accepted")
		}
		if valid && (w.Code != 303 || Moment(owner) == nil) {
			t.Fatalf("schedule failed: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestPurposeAppliesToEveryPromptPath(t *testing.T) {
	for _, opts := range []QueryOpts{{}, {System: "Specialist instructions"}, {System: includedBriefInstruction, NoTools: true}, {System: eveningReadingInstruction, NoTools: true}} {
		if !strings.Contains(nativeSystem(opts), purpose) {
			t.Fatal("purpose missing from prompt path")
		}
	}
}
