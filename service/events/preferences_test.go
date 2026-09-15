package events

import (
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/data"
	"strings"
	"testing"
	"time"
)

func TestBriefPreferencesPreserveScheduleAndSurviveReconcile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reset()
	t.Cleanup(reset)
	owner := "brief_preferences"
	if err := auth.Create(&auth.Account{ID: owner, Zone: "Europe/London"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(owner) })
	if err := ScheduleBrief(owner, "07:15", "Europe/London", "weekdays", "morning", false); err != nil {
		t.Fatal(err)
	}
	original := *Brief(owner)
	if !BriefWorldNews(&original) {
		t.Fatal("existing schedules lost default news preference")
	}
	if err := ConfigureBrief(owner, false, false, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	ensureDefaultBriefs()
	e := Brief(owner)
	if e.ID != original.ID || !e.Paused || BriefWorldNews(e) || e.Zone != original.Zone || e.Repeat != original.Repeat || !e.When.Equal(original.When) {
		t.Fatalf("preference changed timing or opt-out: %+v", e)
	}
	var stored []*Event
	if err := data.LoadJSON(storeKey, &stored); err != nil {
		t.Fatal(err)
	}
	reset()
	for _, s := range stored {
		events[s.ID] = s
	}
	ensureDefaultBriefs()
	e = Brief(owner)
	if e == nil || !e.Paused || BriefWorldNews(e) {
		t.Fatal("preferences lost after restart")
	}
	if err := ConfigureBrief(owner, true, false, "UTC"); err != nil {
		t.Fatal(err)
	}
	if len(List(owner)) != 1 || Brief(owner).ID != original.ID || BriefWorldNews(Brief(owner)) {
		t.Fatal("reenabling duplicated or reset the brief")
	}
	if Brief("another-owner") != nil {
		t.Fatal("cross-owner preference")
	}
}

func TestBriefToggleRestoresNamedTimezoneBeforeCatchup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reset()
	t.Cleanup(reset)
	loc, _ := time.LoadLocation("Europe/London")
	now := time.Now().In(loc)
	// Persist a winter occurrence, whose JSON numeric offset cannot carry DST.
	at := time.Date(now.Year()-1, time.December, 1, 6, 0, 0, 0, loc)
	if at.After(now) {
		at = at.AddDate(-1, 0, 0)
	}
	raw, _ := json.Marshal(Event{ID: "clock", Owner: "clock", Kind: "brief", When: at, Zone: "Europe/London", Repeat: "daily", Paused: true})
	var e Event
	json.Unmarshal(raw, &e)
	events[e.ID] = &e
	if err := ConfigureBrief("clock", true, false, ""); err != nil {
		t.Fatal(err)
	}
	next := Brief("clock").When.In(loc)
	if next.Hour() != 6 || next.Minute() != 0 || !next.After(now) {
		t.Fatalf("clock drifted after resume: %v", next)
	}
}

func TestBriefNewsPreferenceReachesScheduledWork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const owner = "brief_work_preferences"
	if err := auth.Create(&auth.Account{ID: owner}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { auth.DeleteAccount(owner) })
	for _, news := range []bool{true, false} {
		got := asked(t, func() {
			requestWork(&Event{ID: "brief", Owner: owner, Kind: "brief", Title: "Morning brief", Prompt: "Give me a brief for today", WorldNews: &news})
		})
		if len(got) != 1 {
			t.Fatal("scheduled work missing")
		}
		prompt, _ := got[0]["prompt"].(string)
		if news && !strings.Contains(prompt, "Include a short world news") || !news && !strings.Contains(prompt, "Do not fetch or include world news") {
			t.Fatalf("preference ignored: %q", prompt)
		}
	}
}
