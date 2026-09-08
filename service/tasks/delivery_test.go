package tasks

import (
	"fmt"
	"testing"
	"time"

	"mu/internal/userdb"
)

func TestOutcomeAndDeliveryStayTogether(t *testing.T) {
	setupTasks(t)
	task, err := CreateOn("alice", "source", "specialist", "Do work", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := RecordOutcome("alice", task.ID, StatusDone, "Done", "The result", "specialist", []Step{{Tool: "news_list", OK: true}})
	if err != nil {
		t.Fatal(err)
	}
	pending := PendingDeliveries("alice", "")
	if len(pending) != 1 || pending[0].Status != StatusDone || pending[0].Result != "Done" || pending[0].Delivery.Text != "The result" || len(pending[0].Steps) != 1 {
		t.Fatalf("incomplete persisted outcome: %+v", pending)
	}
	if len(PendingDeliveries("bob", "")) != 0 || len(PendingDeliveries("", "")) != 0 {
		t.Fatal("delivery crossed accounts")
	}
	if _, err := Update("alice", task.ID, "Edited", "", "", "", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get("alice", task.ID); got.Delivery == nil || got.Delivery.ID != outcome.Delivery.ID {
		t.Fatal("editing dropped the pending reply")
	}
	if err := Run("alice", task.ID); err == nil {
		t.Fatal("pending delivery permitted another execution")
	}
	if err := AcknowledgeDelivery("alice", task.ID, "older-delivery"); err == nil {
		t.Fatal("stale acknowledgement cleared a newer outcome")
	}
	if err := AcknowledgeDelivery("bob", task.ID, outcome.Delivery.ID); err == nil {
		t.Fatal("another owner acknowledged the outcome")
	}
	if err := AcknowledgeDelivery("alice", task.ID, outcome.Delivery.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := Get("alice", task.ID)
	if got.Delivery != nil || got.Result != "Done" || got.Status != StatusDone || len(PendingDeliveries("alice", "")) != 0 {
		t.Fatal("acknowledgement lost the result or left it pending")
	}
}

func TestFailedOutcomeCanRunAgainOnlyAfterDelivery(t *testing.T) {
	setupTasks(t)
	task, err := CreateOn("alice", "source", "specialist", "Try work", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := RecordOutcome("alice", task.ID, StatusFailed, "Failed", "Could not finish", "specialist", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run("alice", task.ID); err == nil {
		t.Fatal("retry could overwrite an undelivered failure")
	}
	if err := AcknowledgeDelivery("alice", task.ID, failed.Delivery.ID); err != nil {
		t.Fatal(err)
	}
	if err := Run("alice", task.ID); err != nil {
		t.Fatalf("explicit retry rejected: %v", err)
	}
}

func TestPendingDeliveryCursorAdvancesPastUndeliverableFirstBatch(t *testing.T) {
	setupTasks(t)
	for i := 0; i < userdb.MaxListLimit+1; i++ {
		id := fmt.Sprintf("reply-%03d", i)
		if _, err := userdb.Create(ns, "alice", collection, map[string]interface{}{
			"title": "Result", "status": StatusDone, "assignee": Agent,
			"delivery":         encodeDelivery(&Delivery{ID: id, Text: "Answer"}),
			"delivery_pending": true, "delivery_id": id,
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	first := PendingDeliveries("alice", "")
	if len(first) != userdb.MaxListLimit {
		t.Fatalf("first batch size: %d", len(first))
	}
	// No acknowledgements: all first-page destinations may be unavailable.
	next := PendingDeliveries("alice", first[len(first)-1].Delivery.ID)
	if len(next) != 1 || next[0].Delivery.ID != "reply-200" {
		t.Fatalf("later pending outcome was starved: %+v", next)
	}
}
