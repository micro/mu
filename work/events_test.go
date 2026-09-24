package work

import (
	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
	"mu/service/tasks"
	"testing"
	"time"
)

func TestWorkReceiptPreventsToolReplay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	r := request{EventID: "event-1", Account: "owner", Kind: tasks.Kind, ID: "task", Prompt: "do something"}
	calls := 0
	execute := func(request) { calls++ }
	if err := consumeRequest(r, execute); err != nil {
		t.Fatal(err)
	}
	if err := consumeRequest(r, execute); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("executed %d times", calls)
	}
	r.EventID = "event-2"
	if err := persist.Write("work/events/event-2.json", []byte(`"started"`)); err != nil {
		t.Fatal(err)
	}
	if err := consumeRequest(r, execute); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("replayed interrupted tool work")
	}
	r.EventID = "event-3"
	if err := consumeRequest(r, execute); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("suppressed a new explicit attempt")
	}
}

func TestWorkResolvesOwnedTaskAndRejectsStaleAttempt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := service.Register(tasks.Spec); err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Create("event-owner", "requested work", "details", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Run("event-owner", task.ID); err != nil {
		t.Fatal(err)
	}
	task, err = tasks.Get("event-owner", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	e := event.Record{ID: "event", Type: event.TaskStarted, Account: "event-owner", Resource: task.ID, Version: task.Attempts[len(task.Attempts)-1].ID}
	r, err := requestFor(e)
	if err != nil || r.Prompt != "requested work\n\ndetails" {
		t.Fatalf("%+v %v", r, err)
	}
	e.Version = "stale"
	r, err = requestFor(e)
	if err != nil || r.Account != "" {
		t.Fatalf("stale attempt accepted: %+v %v", r, err)
	}
	e.Account = "other"
	r, _ = requestFor(e)
	if r.Prompt != "" {
		t.Fatal("cross-account work")
	}
}
