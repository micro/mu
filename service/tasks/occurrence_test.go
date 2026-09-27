package tasks

import (
	"fmt"
	"testing"
	"time"
)

func TestRecurringOccurrencesGroupAndStayOutOfOrdinaryQueue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "occurrence-owner"
	due := time.Now().UTC()
	var first *Task
	for i := 0; i < 32; i++ {
		o := Occurrence{Key: fmt.Sprintf("run-%d", i), Schedule: "brief", Revision: "1", State: "ready"}
		task, err := RecordOccurrence(owner, "Morning brief", due.AddDate(0, 0, i), o, "Saved report")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = task
		}
	}
	o := Occurrence{Key: "run-0", Schedule: "brief", Revision: "1", State: "done", MessageID: "<brief-1@example.test>"}
	again, err := RecordOccurrence(owner, "Morning brief", due, o, "Saved report")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatal("repeated callback created another task")
	}
	if len(Occurrences(owner, "brief")) != 32 {
		t.Fatal("wrong run history")
	}
	if len(LatestOccurrences(owner)) != 1 {
		t.Fatal("daily runs flooded groups")
	}
	if len(List(owner, "")) != 0 || Next(owner) != nil {
		t.Fatal("scheduled occurrences entered ordinary queue")
	}
	if len(Occurrences("other-owner", "brief")) != 0 {
		t.Fatal("foreign history exposed")
	}
	if err := Run(owner, first.ID); err == nil {
		t.Fatal("scheduled run executed through generic retry")
	}
	saved, err := Get(owner, first.ID)
	if err != nil || saved.Result != "Saved report" || saved.Occurrence.MessageID == "" {
		t.Fatal("result or message reference lost")
	}
}
