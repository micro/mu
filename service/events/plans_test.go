package events

import (
	"mu/internal/auth"
	"testing"
)

func TestBriefCadenceAndPlanEntitlement(t *testing.T) {
	old := auth.SubscriptionTier
	t.Cleanup(func() { auth.SubscriptionTier = old })
	tier := "free"
	auth.SubscriptionTier = func(string) string { return tier }
	owner := "benefit-cadence"
	defer DeleteAll(owner)
	if err := ScheduleBrief(owner, "08:00", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	if Brief(owner).Repeat != "weekly" {
		t.Fatal("free scheduled daily")
	}
	if err := setBriefPlan(owner, true); err == nil {
		t.Fatal("free enabled planning")
	}
	tier = "starter"
	if err := ScheduleBrief(owner, "08:00", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	if Brief(owner).Repeat != "daily" {
		t.Fatal("starter did not get daily")
	}
	if err := setBriefPlan(owner, true); err == nil {
		t.Fatal("starter enabled planning")
	}
	tier = "pro"
	if err := setBriefPlan(owner, true); err != nil || !Brief(owner).Plan {
		t.Fatal("pro plan not saved")
	}
}
