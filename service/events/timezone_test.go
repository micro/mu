package events

import (
	"mu/internal/auth"
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
