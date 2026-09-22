package events

import (
	"context"
	"mu/internal/auth"
	"mu/internal/service"
	"testing"
	"time"
)

func TestStandingScheduleKeepsLocalTimeAcrossDST(t *testing.T) {
	const owner = "evening_zone_test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Zone: "Europe/London"})
	defer auth.RemoveAccountForTest(owner)
	when, _ := time.Parse(time.RFC3339, "2026-10-24T20:00:00+01:00")
	e, err := CreateStanding(owner, "Evening debrief", when, "", 0, "daily", "Review today")
	if err != nil {
		t.Fatal(err)
	}
	defer Cancel(owner, e.ID)
	rescheduleLocked(e, when.Add(time.Hour))
	if e.Zone != "Europe/London" || e.When.Format(time.RFC3339) != "2026-10-25T20:00:00Z" {
		t.Fatalf("wrong next evening: %+v", e)
	}
}

func TestEnablingRepeatRetainsTimezone(t *testing.T) {
	const owner = "repeat_update_zone"
	auth.SetAccountForTest(&auth.Account{ID: owner, Zone: "Europe/London"})
	defer auth.RemoveAccountForTest(owner)
	e, err := CreateStanding(owner, "Debrief", time.Now().Add(48*time.Hour), "", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Cancel(owner, e.ID)
	repeat := "daily"
	var out UpdateResponse
	if err := (Server{}).Update(service.WithAccount(context.Background(), owner), &UpdateRequest{ID: e.ID, Repeat: &repeat}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Item.Zone != "Europe/London" {
		t.Fatal("repeat update lost local timezone")
	}
}
