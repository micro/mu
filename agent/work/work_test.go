package work

// Where an answer goes when the work is finished.
//
// This is the half that moved. service/tasks and service/events each ran the
// agent themselves and decided what to do with the result; both now announce
// and stop, and putting the answer back is here. So is the rule events had and
// could only half keep: a run that could not happen is news the owner needs,
// because a standing instruction that goes quiet looks like one nobody set.
//
// The agent itself is not exercised — that needs a model, and what is worth
// testing is the dispatch: a task keeps its result, a failure reopens it, and a
// scheduled run says something either way.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/service/tasks"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-work-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// A finished task keeps what came of it, and what was done to get there.
func TestAFinishedTaskKeepsItsResult(t *testing.T) {
	const who = "work-done"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	task, err := tasks.Create(who, "Summarise the news", "Top five", "agent", time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	finishTask(request{Account: who, Kind: tasks.Kind, ID: task.ID, Title: task.Title},
		"Here is the summary.",
		[]tasks.Step{{Tool: "web_search", Detail: "latest AI news", OK: true, Seconds: 1.2}},
		nil)

	got, err := tasks.Get(who, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != tasks.StatusDone {
		t.Errorf("the task is %q, want done", got.Status)
	}
	if got.Result != "Here is the summary." {
		t.Errorf("the result was not stored: %q", got.Result)
	}
	if len(got.Steps) != 1 || got.Steps[0].Tool != "web_search" {
		t.Errorf("the steps were lost: %+v", got.Steps)
	}
}

// A failed run leaves the work to be done rather than marking it finished
// badly, and says why where somebody will see it.
func TestAFailedRunReopensTheTask(t *testing.T) {
	const who = "work-failed"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	task, err := tasks.Create(who, "Something hard", "", "agent", time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	finishTask(request{Account: who, Kind: tasks.Kind, ID: task.ID, Title: task.Title},
		"", nil, fmt.Errorf("the model is unavailable"))

	got, err := tasks.Get(who, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != tasks.StatusTodo {
		t.Errorf("a failed run left the task %q, want todo", got.Status)
	}
	if !strings.Contains(got.Result, "unavailable") {
		t.Errorf("the reason is not on the task: %q", got.Result)
	}
}

// Work off the bus is read whole, and a message that is not a work request is
// not one — which is what a subscriber on the wrong topic gets.
func TestAWorkRequestIsReadWhole(t *testing.T) {
	got, ok := requestFrom(map[string]interface{}{
		"account": "asim", "kind": "task", "id": "t1",
		"title": "Brief me", "prompt": "brief me on the news",
	})
	if !ok {
		t.Fatal("a complete request was refused")
	}
	if got.Account != "asim" || got.Kind != "task" || got.ID != "t1" ||
		got.Title != "Brief me" || got.Prompt != "brief me on the news" {
		t.Errorf("the request flattened to %+v", got)
	}

	// Nothing to run, or nobody to run it for, is not a request.
	for _, bad := range []map[string]interface{}{
		{"account": "asim", "kind": "task"},
		{"prompt": "do a thing"},
		{},
	} {
		if _, ok := requestFrom(bad); ok {
			t.Errorf("%v was accepted as work", bad)
		}
	}
}
