package tasks

import (
	"testing"
	"time"
)

func TestClaimStopAndRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "claim-stop-retry"
	task, err := Create(owner, "Run", "", Me, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(owner, task.ID); err != nil {
		t.Fatal(err)
	}
	task, _ = Get(owner, task.ID)
	attempt := task.Attempts[len(task.Attempts)-1].ID
	if _, _, err := Claim("another-owner", task.ID, attempt); err == nil {
		t.Fatal("cross-account claim")
	}
	if _, _, err := Claim(owner, task.ID, "stale"); err == nil {
		t.Fatal("stale claim")
	}
	stopped, release, err := Claim(owner, task.ID, attempt)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, _, err := Claim(owner, task.ID, attempt); err == nil {
		t.Fatal("duplicate claim")
	}
	if err := Stop(owner, task.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker not notified")
	}
	if err := Run(owner, task.ID); err == nil {
		t.Fatal("retry overlapped stopping worker")
	}
	release()
	if err := Run(owner, task.ID); err != nil {
		t.Fatal(err)
	}
	task, _ = Get(owner, task.ID)
	next := task.Attempts[len(task.Attempts)-1].ID
	if next == attempt {
		t.Fatal("attempt reused")
	}
	if _, _, err := Claim(owner, task.ID, attempt); err == nil {
		t.Fatal("old attempt claimed retry")
	}
	_, done, err := Claim(owner, task.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	release() // an old release must not clear the new claim
	if _, _, err := Claim(owner, task.ID, next); err == nil {
		t.Fatal("old release cleared new claim")
	}
}

func TestPreparationActivity(t *testing.T) {
	release := BeginPreparation("owner", "occurrence")
	if !Preparing("owner", "occurrence") || Preparing("other", "occurrence") {
		t.Fatal("incorrect activity ownership")
	}
	release()
	if Preparing("owner", "occurrence") {
		t.Fatal("activity persisted after release")
	}
}
