package agent

import (
	"mu/internal/auth"
	"mu/internal/group"
	"mu/service/events"
	"testing"
)

func TestGroupBriefOwnedOnceAndRequiresGroupPlan(t *testing.T) {
	g, _ := group.Create("brief-group-owner", "Morning team", false)
	defer group.Change(g.ID, g.Owner, "delete", "", "")
	old := auth.SubscriptionTier
	defer func() { auth.SubscriptionTier = old }()
	auth.SubscriptionTier = func(string) string { return "free" }
	if err := ConfigureGroupBrief(g.ID, "08:00", "Europe/London"); err == nil {
		t.Fatal("free group gained a scheduled brief")
	}
	auth.SubscriptionTier = func(id string) string {
		if id == group.Scope(g.ID) {
			return "starter"
		}
		return "free"
	}
	if err := ConfigureGroupBrief(g.ID, "08:00", "Europe/London"); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureGroupBrief(g.ID, "09:00", "Europe/London"); err != nil {
		t.Fatal(err)
	}
	schedules := events.List(group.Scope(g.ID))
	if len(schedules) != 1 || schedules[0].Kind != "group_brief" || schedules[0].When.Hour() != 9 {
		t.Fatalf("duplicate or invalid shared brief: %+v", schedules)
	}
	if len(events.List(g.Owner)) != 0 {
		t.Fatal("brief belongs to creator personally")
	}
	if err := ConfigureGroupBrief(g.ID, "09:00", "invalid/zone"); err == nil {
		t.Fatal("invalid timezone accepted")
	}
}
