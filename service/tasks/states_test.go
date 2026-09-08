package tasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"mu/internal/service"
	"mu/internal/userdb"
)

func TestNextSkipsWorkThatIsNotReady(t *testing.T) {
	setupTasks(t)
	for _, state := range []string{StatusFailed, StatusBlocked, StatusDoing, StatusDone} {
		task, err := Create("alice", state, "", Agent, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Update("alice", task.ID, "", "", state, "", "Previous result"); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := CreateOn("alice", "source", "specialist", "Delivery pending", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordOutcome("alice", pending.ID, StatusTodo, "Saved", "Saved", "specialist", nil); err != nil {
		t.Fatal(err)
	}
	if got := Next("alice"); got != nil {
		t.Fatalf("picked work requiring review or delivery: %+v", got)
	}
	ready, err := Create("alice", "Ready", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := Next("alice"); got == nil || got.ID != ready.ID {
		t.Fatalf("ready work was not picked: %+v", got)
	}
}

func TestReadyWorkAndOutcomeFiltersSurviveALongTaskHistory(t *testing.T) {
	setupTasks(t)
	ready, err := Create("alice", "Ready", "", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{StatusFailed, StatusBlocked} {
		if _, err := userdb.Create(ns, "alice", collection, map[string]any{
			"title": state, "status": state, "assignee": Agent,
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	// More recent personal tasks must not crowd owned state filters or pickup.
	for i := 0; i < userdb.MaxListLimit+1; i++ {
		if _, err := userdb.Create(ns, "alice", collection, map[string]any{
			"title": "Personal", "status": StatusTodo, "assignee": Me,
		}, false); err != nil {
			t.Fatal(err)
		}
	}
	if got := Next("alice"); got == nil || got.ID != ready.ID {
		t.Errorf("task history hid ready work: %+v", got)
	}
	for _, state := range []string{StatusFailed, StatusBlocked} {
		if got := List("alice", state); len(got) != 1 || got[0].Status != state {
			t.Errorf("task history hid %s work: %+v", state, got)
		}
	}
	if Next("") != nil || Next("bob") != nil {
		t.Fatal("ready work crossed ownership")
	}
}

func TestOutcomeStatesPersistFilterAndOfferExplicitRetry(t *testing.T) {
	setupTasks(t)
	for _, state := range []string{StatusFailed, StatusBlocked} {
		task, err := CreateOn("alice", "source", "specialist", state, "Context", Agent, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		var rsp TaskResponse
		if err := (Server{}).Update(service.WithAccount(context.Background(), "alice"), &UpdateRequest{ID: task.ID, Status: state, Result: "Review prior actions"}, &rsp); err != nil {
			t.Fatal(err)
		}
		got := mustGet(t, "alice", task.ID)
		if got.Status != state || !got.Open() || Running(got) || got.Thread != "source" || got.Agent != "specialist" {
			t.Fatalf("state or identity lost: %+v", got)
		}
		var list ListResponse
		if err := (Server{}).List(service.WithAccount(context.Background(), "alice"), &ListRequest{Status: state}, &list); err != nil || len(list.Items) != 1 || list.Items[0].ID != task.ID {
			t.Fatalf("state filter failed: %+v, %v", list, err)
		}
		row := taskRow(got, "csrf", "Specialist")
		if !strings.Contains(row, `data-task-status="`+state+`"`) || !strings.Contains(row, ">Retry</button>") || strings.Contains(row, "working…") {
			t.Fatalf("state not actionable: %s", row)
		}
		if err := Run("alice", task.ID); err != nil {
			t.Fatalf("explicit retry failed: %v", err)
		}
		running := mustGet(t, "alice", task.ID)
		if running.Status != StatusDoing || strings.Contains(taskRow(running, "csrf"), "/run\"") {
			t.Fatal("retry did not become working or still offers another run")
		}
	}
}
