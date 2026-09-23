package events

import "testing"

func TestBriefSchedulePreservesIdentityAndPreferences(t *testing.T) {
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

func TestMorningAndEveningSchedulesAreIndependent(t *testing.T) {
	const owner = "brief_independent_test"
	defer DeleteAll(owner)
	if err := scheduleBrief(owner, "06:00", "Europe/London", "weekdays", "morning", false, false, false); err != nil {
		t.Fatal(err)
	}
	morning := Brief(owner)
	if err := scheduleBrief(owner, "20:00", "Europe/London", "daily", "evening", false, false, true); err != nil {
		t.Fatal(err)
	}
	evening := Brief(owner, "evening")
	if evening == nil || evening.ID == morning.ID {
		t.Fatal("evening replaced morning")
	}
	if err := ConfigureBrief(owner, false, false, "Europe/London", "evening"); err != nil {
		t.Fatal(err)
	}
	if Brief(owner).Paused || !Brief(owner, "evening").Paused || BriefWorldNews(Brief(owner)) {
		t.Fatal("evening toggle affected morning")
	}
	if Brief(owner).Repeat != "weekdays" || Brief(owner, "evening").Repeat != "daily" {
		t.Fatal("cadence changed")
	}
}
