package agent

import (
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestActivityIsScopedAndReflectsWork(t *testing.T) {
	const owner = "activity-owner"
	flowMu.Lock()
	flowStore["activity-test"] = &Flow{ID: "activity-test", AccountID: owner, Agent: "", Status: "running", CreatedAt: time.Now()}
	flowMu.Unlock()
	t.Cleanup(func() { flowMu.Lock(); delete(flowStore, "activity-test"); flowMu.Unlock() })
	if got := activity(owner, DefaultPlatformAgent); got != "Working" {
		t.Fatalf("default activity: %s", got)
	}
	if got := activity("activity-other", DefaultPlatformAgent); strings.Contains(got, "Working") {
		t.Fatal("another account's activity leaked")
	}
	if got := activity(owner, "specialist"); strings.Contains(got, "Working") {
		t.Fatal("another agent's activity leaked")
	}
	task, err := tasks.CreateOn(owner, "", "", "Needs a decision", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.Update(owner, task.ID, "", "", tasks.StatusBlocked, "", "", nil); err != nil {
		t.Fatal(err)
	}
	if got := activity(owner, ""); !strings.Contains(got, "Working") || !strings.Contains(got, "Needs input") {
		t.Fatalf("simultaneous work and blocked task lost: %s", got)
	}
}
