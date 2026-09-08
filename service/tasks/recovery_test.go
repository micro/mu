package tasks

import (
	"strings"
	"testing"
	"time"

	"mu/internal/event"
	"mu/internal/userdb"
)

func TestRecoveryPreservesWorkWithoutReplayingIt(t *testing.T) {
	setupTasks(t)
	task, err := CreateOn("alice", "source", "specialist", "Send reply", "context", Agent, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	steps := []Step{{Tool: "mail_send", OK: true}}
	if _, err := Update("alice", task.ID, "", "", StatusDoing, "", "A reply was sent", steps); err != nil {
		t.Fatal(err)
	}
	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()
	if err := recoverInterrupted("bob"); err != nil {
		t.Fatal(err)
	}
	before, _ := Get("alice", task.ID)
	if before.Status != StatusDoing {
		t.Fatal("recovery crossed accounts")
	}
	if err := recoverInterrupted("alice"); err != nil {
		t.Fatal(err)
	}
	got, err := Get("alice", task.ID)
	if err != nil || got.Status != StatusBlocked || got.Thread != "source" || got.Agent != "specialist" || len(got.Steps) != 1 || !strings.Contains(got.Result, "A reply was sent") || !strings.Contains(got.Result, "interrupted") {
		t.Fatalf("recovery lost the work record: %+v, %v", got, err)
	}
	if err := recoverInterrupted("alice"); err != nil {
		t.Fatal(err)
	}
	again, _ := Get("alice", task.ID)
	if again.Result != got.Result {
		t.Fatal("repeated startup duplicated the recovery notice")
	}
	select {
	case <-sub.Chan:
		t.Fatal("recovery replayed potentially completed actions")
	default:
	}
}

func TestRecoveryDrainsAllPagesAndLeavesOtherStatesAlone(t *testing.T) {
	setupTasks(t)
	for i := 0; i < userdb.MaxListLimit+1; i++ {
		if _, err := userdb.Create(ns, "alice", collection, map[string]interface{}{
			"title": "interrupted", "status": StatusDoing, "assignee": Agent,
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []string{StatusTodo, StatusDoing, StatusDone} {
		if _, err := userdb.Create(ns, "bob", collection, map[string]interface{}{
			"title": "personal", "status": state, "assignee": "me",
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := recoverInterrupted("alice"); err != nil {
		t.Fatal(err)
	}
	left, err := userdb.List(ns, "alice", collection, "mine", map[string]interface{}{"status": StatusDoing}, "", "", 0)
	if err != nil || len(left) != 0 {
		t.Fatalf("interrupted tasks beyond first page remain: %d, %v", len(left), err)
	}
	if err := recoverInterrupted("bob"); err != nil {
		t.Fatal(err)
	}
	if len(List("bob", StatusDoing)) != 1 || len(List("bob", StatusTodo)) != 1 || len(List("bob", StatusDone)) != 1 {
		t.Fatal("recovery changed personal tasks")
	}
}
