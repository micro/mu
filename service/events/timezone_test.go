package events

import (
	"context"
	"encoding/json"
	"mu/internal/auth"
	"mu/internal/service"
	"strings"
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

func TestBookingRequiresOffsetAndRetainsLocalConfirmation(t *testing.T) {
	const owner = "booking_zone_test"
	auth.SetAccountForTest(&auth.Account{ID: owner, Zone: "Europe/London"})
	defer auth.RemoveAccountForTest(owner)
	ctx := service.WithAccount(context.Background(), owner)
	var out CreateResponse
	if err := (Server{}).Create(ctx, &CreateRequest{Title: "Run", When: "2030-07-22T10:00:00"}, &out); err == nil {
		t.Fatal("accepted ambiguous local time")
	}
	if err := (Server{}).Create(ctx, &CreateRequest{Title: "Run", When: "2030-07-22T10:00:00+01:00"}, &out); err != nil {
		t.Fatal(err)
	}
	defer Cancel(owner, out.Item.ID)
	raw, _ := json.Marshal(out.Item)
	var restored Event
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.LocalTime().Hour() != 10 || !strings.Contains(Describe(&restored), "10:00 BST") {
		t.Fatalf("wrong local time: %s", Describe(&restored))
	}
	if !strings.Contains(ICS(&restored, ""), "DTSTART:20300722T090000Z") {
		t.Fatal("calendar instant changed")
	}
	when := "2030-07-22T09:00:00Z"
	var updated UpdateResponse
	if err := (Server{}).Update(ctx, &UpdateRequest{ID: restored.ID, When: &when}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Item.Sequence != restored.Sequence {
		t.Fatal("unchanged instant published a new revision")
	}
	when = "2030-07-22T11:00:00+01:00"
	if err := (Server{}).Update(ctx, &UpdateRequest{ID: restored.ID, When: &when}, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Item.Sequence != restored.Sequence+1 || updated.Item.LocalTime().Hour() != 11 {
		t.Fatal("reschedule lost local time or revision")
	}
}
