package events

import (
	"mu/internal/auth"
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
	defer DeleteAll(owner)
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
	mu.Lock()
	for _, title := range []string{"Evening brief", "Evening Debrief", "Daily brief"} {
		id := owner + title
		events[id] = &Event{ID: id, Owner: owner, Kind: "brief", Title: title, Prompt: "Give me a brief for tomorrow", When: time.Now().Add(-time.Hour), Repeat: "daily"}
	}
	mu.Unlock()
	if len(List(owner)) != 1 {
		t.Fatal("retired briefs visible")
	}
	fireDue()
	mu.RLock()
	for _, title := range []string{"Evening brief", "Evening Debrief", "Daily brief"} {
		if events[owner+title] != nil {
			t.Error("retired brief retained", title)
		}
	}
	mu.RUnlock()
	after := Brief(owner)
	if after == nil || after.ID != morning.ID || after.Paused || after.Repeat != morning.Repeat || !after.When.Equal(morning.When) {
		t.Fatal("morning schedule changed")
	}
	if strings.Contains(strings.ToLower(briefScheduleHTML(owner)), "evening") {
		t.Fatal("evening option remains")
	}
}
