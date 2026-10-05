package work

import (
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestDismissedRunLeavesScheduledAttention(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "dismiss-scheduled-attention"
	o := tasks.Occurrence{Key: "failed-run", Schedule: "research", Revision: "1", State: "done", Failure: "budget"}
	run, err := tasks.RecordOccurrence(owner, "Research", time.Now(), o, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ScheduledAttention(owner)) != 1 {
		t.Fatal("missing failed run")
	}
	if err := tasks.ArchiveTask(owner, run.ID, true); err != nil {
		t.Fatal(err)
	}
	if len(ScheduledAttention(owner)) != 0 {
		t.Fatal("dismissed run still needs attention")
	}
	run, err = tasks.Get(owner, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	body := occurrenceDetail(run, "csrf-test")
	if !strings.Contains(body, "Restore") || !strings.Contains(body, "budget") {
		t.Fatal("dismissed run lost restore control or failure details")
	}
}

func TestEmptyOccurrenceDoesNotClaimDelivery(t *testing.T) {
	legacy := &tasks.Task{Occurrence: &tasks.Occurrence{State: "done", Delivery: "delivered"}}
	_, label := occurrenceStatus(legacy, nil)
	if strings.Contains(label, "Delivered") || occurrenceDelivery(legacy) != "not sent" {
		t.Fatalf("misleading legacy status: %s", label)
	}
}
