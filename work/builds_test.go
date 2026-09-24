package work

import (
	"mu/internal/result"
	"mu/internal/thread"
	"mu/service/apps"
	"testing"
	"time"
)

func TestAppBuildDeliveryReplayAndIsolation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "build-delivery-owner"
	th := thread.Open(owner, thread.WebClient, "build-delivery")
	b := apps.BuildStatusResponse{ID: "delivery", State: "complete", Thread: th.ID, Updated: time.Now(), Item: &result.Item{Kind: "app", ID: "checklist", Title: "Checklist", URL: "/apps/checklist"}}
	for i := 0; i < 2; i++ {
		if err := cacheAppBuild(owner, b); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(thread.Messages(owner, th.ID, 0)); got != 1 {
		t.Fatalf("got %d deliveries", got)
	}
	stale := b
	stale.State = "queued"
	stale.Updated = b.Updated.Add(-time.Hour)
	if err := cacheAppBuild(owner, stale); err != nil {
		t.Fatal(err)
	}
	if got := buildsFor(owner)[0].State; got != "complete" {
		t.Fatalf("stale state: %s", got)
	}
	b.ID = "foreign"
	if err := cacheAppBuild("another-owner", b); err != nil {
		t.Fatal(err)
	}
	if got := len(thread.Messages(owner, th.ID, 0)); got != 1 {
		t.Fatalf("foreign delivery: %d", got)
	}
}
