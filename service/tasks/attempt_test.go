package tasks

import (
	"testing"
	"time"
)

func TestAttemptsSurviveRetryAndRejectLateProgress(t *testing.T) {
	const owner = "attempt_history"
	task, err := Create(owner, "Do the work", "", Me, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err = Run(owner, task.ID); err != nil {
		t.Fatal(err)
	}
	task, _ = Get(owner, task.ID)
	first := task.Attempts[0].ID
	steps := []Step{{ID: "tool-one", Tool: "shell_run", Output: "exit 1", Error: "missing binary", Status: "failed"}}
	if err = Progress(owner, task.ID, first, steps, "invalid outcome", "not verified", "revision123"); err != nil {
		t.Fatal(err)
	}
	if _, err = RecordOutcome(owner, task.ID, StatusFailed, "Not verified", "", "", steps); err != nil {
		t.Fatal(err)
	}
	if err = Run(owner, task.ID); err != nil {
		t.Fatal(err)
	}
	task, _ = Get(owner, task.ID)
	if len(task.Attempts) != 2 || task.Attempts[0].Report != "invalid outcome" || task.Attempts[0].Version != "revision123" || task.Attempts[0].Finished.IsZero() {
		t.Fatalf("lost failed attempt: %+v", task.Attempts)
	}
	if err = Progress(owner, task.ID, first, nil, "late", "", ""); err == nil {
		t.Fatal("old attempt overwrote retry")
	}
	if err = ArchiveTask(owner, task.ID, true); err == nil {
		t.Fatal("running work archived")
	}
	if err = Stop(owner, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = RecordOutcome(owner, task.ID, StatusDone, "late success", "", "", nil); err == nil {
		t.Fatal("late result replaced stop")
	}
	task, _ = Get(owner, task.ID)
	if task.Status != StatusCanceled || len(task.Attempts[0].Steps) != 1 {
		t.Fatal("stop lost diagnostics")
	}
	if err = ArchiveTask(owner, task.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = Remove(owner, task.ID); err != nil {
		t.Fatal(err)
	}
}
