package agent

import (
	"mu/internal/auth"
	"mu/service/events"
	"strings"
	"testing"
	"time"
)

func TestBriefSchedulePreservesIdentityAndPreferences(t *testing.T) {
	old := auth.SubscriptionTier
	auth.SubscriptionTier = func(string) string { return "starter" }
	t.Cleanup(func() { auth.SubscriptionTier = old })
	const owner = "brief_schedule_preferences_test"
	if err := scheduleBrief(owner, "07:30", "Europe/London", "weekdays", "morning", false, false, false); err != nil {
		t.Fatal(err)
	}
	first := Brief(owner)
	if first == nil || BriefWorldNews(first) {
		t.Fatal("world news preference not saved")
	}
	if err := ScheduleBrief(owner, "08:30", "Europe/London", "daily", "morning", true); err != nil {
		t.Fatal(err)
	}
	second := Brief(owner)
	if second.ID != first.ID || !second.Paused || BriefWorldNews(second) || second.Repeat != "daily" {
		t.Fatal("schedule edit lost identity or preferences")
	}
	if err := scheduleBrief(owner, "09:30", "Europe/London", "weekdays", "morning", false, false, true); err != nil {
		t.Fatal(err)
	}
	third := Brief(owner)
	if third.ID != first.ID || third.Paused || !BriefWorldNews(third) {
		t.Fatal("combined settings were not saved")
	}
	if err := scheduleBrief(owner, "invalid", "Europe/London", "daily", "morning", true, false, false); err == nil {
		t.Fatal("invalid schedule accepted")
	}
	final := Brief(owner)
	if final.ID != third.ID || final.Paused != third.Paused || !BriefWorldNews(final) {
		t.Fatal("invalid edit changed settings")
	}
}

func TestEveningBriefRetired(t *testing.T) {
	const owner = "retired_brief_test"
	defer events.DeleteAll(owner)
	if err := ScheduleBrief(owner, "06:00", "Europe/London", "weekdays", "morning", false); err != nil {
		t.Fatal(err)
	}
	morning := Brief(owner)
	if err := ScheduleBrief(owner, "20:00", "Europe/London", "daily", "evening", false); err == nil {
		t.Fatal("evening schedule accepted")
	}
	if err := ConfigureBrief(owner, true, true, "Europe/London", "evening"); err == nil {
		t.Fatal("evening preference accepted")
	}
	for _, title := range []string{"Evening brief", "Evening Debrief", "Daily brief"} {
		old := &events.Event{ID: owner + title, Owner: owner, Kind: "brief", Title: title, Prompt: "Give me a brief for tomorrow", When: time.Now().Add(-time.Hour), Repeat: "daily"}
		if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[old.ID] = old; return nil }); err != nil {
			t.Fatal(err)
		}
		allowed, err := reserveScheduled(old, old.When, time.Now())
		if err != nil || allowed {
			t.Fatalf("retired brief ran: %v", err)
		}
	}
	if len(events.List(owner)) != 1 {
		t.Fatal("retired schedules retained")
	}
	after := Brief(owner)
	if after == nil || after.ID != morning.ID || after.Paused || after.Repeat != morning.Repeat || !after.When.Equal(morning.When) {
		t.Fatal("morning schedule changed")
	}
	if strings.Contains(strings.ToLower(briefScheduleHTML(owner)), "evening") {
		t.Fatal("evening option remains")
	}
}

func TestExplicitBriefTimeChangeAllowsAnotherOccurrence(t *testing.T) {
	oldTier := auth.SubscriptionTier
	auth.SubscriptionTier = func(string) string { return "starter" }
	defer func() { auth.SubscriptionTier = oldTier }()
	owner := "brief_reschedule_regression"
	defer events.DeleteAll(owner)
	if err := ScheduleBrief(owner, "06:00", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	first := Brief(owner)
	if err := events.EditOwned(owner, func(records map[string]*events.Event) error {
		records[first.ID].LastBrief = time.Now().UTC()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := ScheduleBrief(owner, "06:00", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	if Brief(owner).LastBrief.IsZero() {
		t.Fatal("unchanged settings bypassed cadence")
	}
	if err := ScheduleBrief(owner, "07:15", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	changed := Brief(owner)
	if !changed.LastBrief.IsZero() {
		t.Fatal("explicit reschedule remains blocked by previous delivery")
	}
	loc, _ := time.LoadLocation("Europe/London")
	if changed.ID != first.ID || changed.When.In(loc).Format("15:04") != "07:15" {
		t.Fatal("new schedule not preserved")
	}
}

func TestBriefCadenceGuardPreservesChosenClock(t *testing.T) {
	oldTier := auth.SubscriptionTier
	auth.SubscriptionTier = func(string) string { return "starter" }
	defer func() { auth.SubscriptionTier = oldTier }()
	owner := "brief_clock_regression"
	defer events.DeleteAll(owner)
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	source := &events.Event{ID: owner, Owner: owner, Kind: "brief", Title: "Morning brief", When: due, LastBrief: now.Add(-time.Hour), Zone: "UTC", Repeat: "daily"}
	if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[owner] = source; return nil }); err != nil {
		t.Fatal(err)
	}
	if allowed, err := reserveScheduled(source, due, now); err != nil || allowed {
		t.Fatalf("cadence was not enforced: %v", err)
	}
	after := Brief(owner)
	if after.When.Format("15:04") != due.Format("15:04") {
		t.Fatalf("clock reverted: %v -> %v", due, after.When)
	}
	if !after.When.After(now) {
		t.Fatal("schedule did not advance")
	}
}
