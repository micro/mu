package agent

import (
	"mu/internal/auth"
	"mu/service/events"
	"mu/service/prayer"
	"strings"
	"testing"
	"time"
)

func TestEveningReflectionMigrationPreservesConsentAndTime(t *testing.T) {
	const owner = "reflection-migration"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	defer events.DeleteAll(owner)
	when := time.Now().Add(time.Hour)
	for _, paused := range []bool{false, true} {
		old := events.Event{ID: owner, Owner: owner, Kind: "research", Title: "Evening Reading: Islam", Paused: paused, When: when, Zone: "Europe/London", Repeat: "daily", Sequence: 7, MaxCredits: 6}
		if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[old.ID] = &old; return nil }); err != nil {
			t.Fatal(err)
		}
		configureScheduledAdvances()
		got := Research(owner)
		if got.Title != "Evening reflection" || got.Paused != paused || !got.When.Equal(when) || got.Zone != old.Zone || got.Repeat != old.Repeat || got.Sequence != 8 || got.MaxCredits != 6 {
			t.Fatalf("migration changed schedule: %+v", got)
		}
		configureScheduledAdvances()
		if Research(owner).Sequence != 8 {
			t.Fatal("migration was not idempotent")
		}
	}
}

func TestEveningReflectionPreservesReminder(t *testing.T) {
	const message = "An existing daily reflection, unchanged."
	text, err := renderEveningReflection(&prayer.ReminderData{Message: message}, "2026-10-11")
	if err != nil || !strings.HasPrefix(text, message+"\n\n") || !strings.Contains(text, "date=2026-10-11") {
		t.Fatalf("%q %v", text, err)
	}
	for _, reminder := range []*prayer.ReminderData{nil, {}, {Message: " "}} {
		if _, err := renderEveningReflection(reminder, "2026-10-11"); err == nil {
			t.Fatal("missing reminder treated as delivered")
		}
	}
}
