package exec

import (
	"mu/internal/event"
	"mu/internal/persist"
	"mu/internal/service"
	"mu/service/events"
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

func TestBuiltinScheduleWithoutPromptRuns(t *testing.T) {
	const owner = "empty-digest-owner"
	defer events.DeleteAll(owner)
	due := time.Now().Add(-time.Second)
	schedule := &events.Event{ID: "empty-digest", Owner: owner, Kind: "research", Title: "Evening Digest", Sequence: 1, When: due, Advance: events.Advance{Minutes: 15, Recipient: "agent"}}
	if err := events.EditOwned(owner, func(records map[string]*events.Event) error { records[schedule.ID] = schedule; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{event.ScheduleAdvance, event.ScheduleDue} {
		r, err := requestFor(event.Record{ID: typ + "-empty-digest", Type: typ, Account: owner, Resource: schedule.ID, Version: "1", Data: map[string]interface{}{"when": due, "recipient": "agent"}})
		if err != nil || !r.Builtin || !r.Prepared || r.Prompt != "" {
			t.Fatalf("built-in request lost: %+v %v", r, err)
		}
		calls := 0
		if err := consumeRequest(r, func(request) { calls++ }); err != nil {
			t.Fatal(err)
		}
		if calls != 1 {
			t.Fatal("empty built-in prompt skipped")
		}
	}
	ordinary := request{EventID: "ordinary-empty", Account: owner, Kind: events.Kind}
	if err := consumeRequest(ordinary, func(request) { t.Fatal("ordinary event started agent work") }); err != nil {
		t.Fatal(err)
	}
}
