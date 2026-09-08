package work

import (
	"testing"
	"time"

	"mu/agent"
	"mu/internal/thread"
	"mu/service/tasks"
)

func TestDeliveryRetriesAfterRecordingWithoutRepeatingTheMessage(t *testing.T) {
	const who = "delivery-retry"
	th := thread.Open(who, thread.ChatClient, "private-source")
	task, err := tasks.CreateOn(who, th.ID, "specialist", "Research", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := tasks.RecordOutcome(who, task.ID, tasks.StatusDone, "Saved answer", "Saved answer", "specialist", nil)
	if err != nil {
		t.Fatal(err)
	}
	// The first process wrote the message but stopped before acknowledging it.
	if err := agent.AnsweredOnce(who, th.ID, pending.Delivery.Text, pending.Delivery.From, "task-result:"+pending.Delivery.ID); err != nil {
		t.Fatal(err)
	}
	// A fresh worker reads only persisted pending work, never calls a model.
	loaded := tasks.PendingDeliveries(who, "")
	if len(loaded) != 1 {
		t.Fatal("pending result was not persisted")
	}
	if err := deliverTask(loaded[0]); err != nil {
		t.Fatal(err)
	}
	if err := deliverTask(pending); err != nil {
		t.Fatal(err)
	}
	msgs := thread.Messages(who, th.ID, 0)
	if len(msgs) != 1 || msgs[0].Text != "Saved answer" || msgs[0].From != "specialist" {
		t.Fatalf("duplicate or incorrect answer: %+v", msgs)
	}
	if len(tasks.PendingDeliveries(who, "")) != 0 {
		t.Fatal("persisted message was not acknowledged")
	}
}

func TestUnavailableConversationKeepsResultPending(t *testing.T) {
	const who = "delivery-missing"
	task, err := tasks.CreateOn(who, "unavailable", "specialist", "Research", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := tasks.RecordOutcome(who, task.ID, tasks.StatusDone, "Saved answer", "Saved answer", "specialist", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := deliverTask(pending); err == nil {
		t.Fatal("missing conversation was reported as delivered")
	}
	got, _ := tasks.Get(who, task.ID)
	if got.Delivery == nil || got.Status != tasks.StatusDone || got.Result != "Saved answer" {
		t.Fatal("delivery failure lost the completed result")
	}
}

func TestReplayedWorkDeliversTheSavedResultWithoutCallingTheModel(t *testing.T) {
	const who = "delivery-replayed-work"
	th := thread.Open(who, thread.ChatClient, "source")
	task, err := tasks.CreateOn(who, th.ID, "specialist", "Research", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.RecordOutcome(who, task.ID, tasks.StatusDone, "Already done", "Already done", "specialist", nil); err != nil {
		t.Fatal(err)
	}
	calls := 0
	query := func(string, string, agent.QueryOpts) (string, error) {
		calls++
		return "Unwanted second run", nil
	}
	r := request{Account: who, ID: task.ID, Kind: tasks.Kind, Thread: th.ID, Prompt: "Research"}
	runWithQuery(r, query)
	runWithQuery(r, query)
	if calls != 0 || len(thread.Messages(who, th.ID, 0)) != 1 || len(tasks.PendingDeliveries(who, "")) != 0 {
		t.Fatal("replayed work repeated execution or lost the delivery")
	}
}

func TestFailedAndBlockedWorkWaitsForExplicitRetry(t *testing.T) {
	for _, state := range []string{tasks.StatusFailed, tasks.StatusBlocked} {
		who := "review-" + state
		task, err := tasks.Create(who, "Do work", "", tasks.Agent, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Update(who, task.ID, "", "", state, "", "Review first"); err != nil {
			t.Fatal(err)
		}
		calls := 0
		query := func(string, string, agent.QueryOpts) (string, error) {
			calls++
			return "Completed after review", nil
		}
		r := request{Account: who, ID: task.ID, Kind: tasks.Kind, Prompt: "Do work"}
		runWithQuery(r, query)
		if calls != 0 {
			t.Fatal("stale request reran work requiring review")
		}
		if err := tasks.Run(who, task.ID); err != nil {
			t.Fatal(err)
		}
		runWithQuery(r, query)
		got, _ := tasks.Get(who, task.ID)
		if calls != 1 || got.Status != tasks.StatusDone {
			t.Fatalf("explicit retry did not finish: calls=%d, task=%+v", calls, got)
		}
	}
}
