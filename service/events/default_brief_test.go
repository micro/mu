package events

import (
	"encoding/json"
	"mu/internal/auth"
	"testing"
	"time"
)

func TestDefaultBriefEnrollmentAndOptOut(t *testing.T) {
	reset()
	defer reset()
	for _, a := range []*auth.Account{
		{ID: "default_london", Zone: "Europe/London"},
		{ID: "default_tokyo", Zone: "Asia/Tokyo"},
		{ID: "default_unknown"},
		{ID: "default_badzone", Zone: "bad/zone"},
		{ID: "default_agent", Zone: "Europe/London", Agent: true},
		{ID: "default_banned", Zone: "Europe/London", Banned: true},
		{ID: "default_unclaimed", Zone: "Europe/London", Unclaimed: true},
	} {
		if err := auth.Create(a); err != nil {
			t.Fatal(err)
		}
		id := a.ID
		t.Cleanup(func() { auth.DeleteAccount(id) })
	}
	ensureDefaultBriefs()
	ensureDefaultBriefs()
	for _, id := range []string{"default_london", "default_tokyo"} {
		e := Brief(id)
		if e == nil {
			t.Fatalf("missing %s", id)
		}
		loc, _ := time.LoadLocation(e.Zone)
		if !e.Builtin || e.When.In(loc).Hour() != 6 || e.When.In(loc).Minute() != 0 || !e.When.After(time.Now()) || e.Prompt != "Give me a brief for today" || len(List(id)) != 1 {
			t.Fatalf("bad default: %+v", e)
		}
	}
	for _, id := range []string{"default_unknown", "default_badzone", "default_agent", "default_banned", "default_unclaimed"} {
		if Brief(id) != nil {
			t.Fatalf("ineligible %s", id)
		}
	}
	e := Brief("default_london")
	if err := Remove(e.Owner, e.ID); err != nil {
		t.Fatal(err)
	}
	// Persisted paused state is what startup reads; reconciliation must retain it.
	raw, _ := json.Marshal(Brief(e.Owner))
	var restored Event
	json.Unmarshal(raw, &restored)
	mu.Lock()
	events[e.ID] = &restored
	mu.Unlock()
	ensureDefaultBriefs()
	if !Brief(e.Owner).Paused || len(List(e.Owner)) != 1 {
		t.Fatal("opt-out was lost")
	}
	if err := ScheduleBrief("default_tokyo", "08:30", "Asia/Tokyo", "weekdays", "morning", false); err != nil {
		t.Fatal(err)
	}
	ensureDefaultBriefs()
	if Brief("default_tokyo").When.Hour() != 8 || Brief("default_tokyo").Repeat != "weekdays" {
		t.Fatal("custom schedule overwritten")
	}
	a, _ := auth.GetAccount("default_unknown")
	a.Zone = "America/New_York"
	auth.UpdateAccount(a)
	ensureDefaultBriefs()
	if Brief(a.ID) == nil {
		t.Fatal("timezone update did not enroll")
	}
}

func TestLegacyEveningDefaultBecomesMorningWithoutReenabling(t *testing.T) {
	for _, paused := range []bool{false, true} {
		owner := "legacy_enabled"
		if paused {
			owner = "legacy_paused"
		}
		auth.Create(&auth.Account{ID: owner, Zone: "Europe/London"})
		defer DeleteAll(owner)
		if err := ScheduleBrief(owner, "20:00", "Europe/London", "daily", "evening", paused); err != nil {
			t.Fatal(err)
		}
		e := Brief(owner)
		mu.Lock()
		events[e.ID].Title = "Daily brief"
		mu.Unlock()
		ensureDefaultBriefs()
		got := Brief(owner)
		loc, _ := time.LoadLocation(got.Zone)
		if got.When.In(loc).Hour() != 6 || got.Title != "Morning brief" || got.Prompt != "Give me a brief for today" || got.Paused != paused {
			t.Fatalf("wrong migration: %+v", got)
		}
	}
}
