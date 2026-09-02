package tasks

// Assigning a task to the agent starts it.

import (
	"context"
	"strings"
	"testing"
	"time"

	"mu/internal/event"
	"mu/internal/service"
)

// The endpoint has said "assign it to the agent and it can pick the task up
// itself" since it was written, and nothing picked anything up: Run is what
// announces the work, and its only caller was a button on /inbox. So filing a
// task for the agent put a row in a list and waited for somebody to press a
// thing they had no reason to know about — which is what "does an agent pick
// that up?" was really asking.
func TestATaskAssignedToTheAgentIsStarted(t *testing.T) {
	setupTasks(t)

	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()

	ctx := service.WithAccount(context.Background(), "someone")
	var rsp TaskResponse
	if err := (Server{}).Create(ctx, &CreateRequest{
		Title: "book the flights", Assignee: Agent,
	}, &rsp); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-sub.Chan:
		if got, _ := e.Data["account"].(string); got != "someone" {
			t.Errorf("the work was announced for %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was announced, so the task sits in a list until\n" +
			"somebody finds the button — which is the state this fixes")
	}
}

// And a task for you is a task for you. Filing something on your own list must
// not spend a model call.
func TestATaskForYourselfIsNotStarted(t *testing.T) {
	setupTasks(t)

	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()

	ctx := service.WithAccount(context.Background(), "someone")
	var rsp TaskResponse
	if err := (Server{}).Create(ctx, &CreateRequest{Title: "call the plumber"}, &rsp); err != nil {
		t.Fatal(err)
	}

	select {
	case <-sub.Chan:
		t.Error("a task assigned to nobody started an agent run — every note to\n" +
			"self would cost a model call")
	case <-time.After(400 * time.Millisecond):
	}
}

// Not when the agent filed it.
//
// A model that creates a task for itself mid-run is describing what it is
// already doing. Starting a second run from inside the first is how one
// question becomes a loop, and Run's status guard does not catch it: each turn
// creates a *new* task, and a new task is always startable.
func TestTheAgentFilingWorkForItselfDoesNotStartAnotherRun(t *testing.T) {
	setupTasks(t)

	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()

	// The context the agent's tool wrapper builds — see injectAccount.
	ctx := service.WithAgentRun(service.WithAccount(context.Background(), "someone"))
	var rsp TaskResponse
	if err := (Server{}).Create(ctx, &CreateRequest{
		Title: "and then book the hotel", Assignee: Agent,
	}, &rsp); err != nil {
		t.Fatal(err)
	}

	select {
	case <-sub.Chan:
		t.Error("a task the agent filed during its own run started a second run")
	case <-time.After(400 * time.Millisecond):
	}
	// It is still on the list, which is the point of filing it.
	if !strings.Contains(Render(List("someone", "")), "book the hotel") {
		t.Error("the task was not recorded at all")
	}
}

func setupTasks(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}
