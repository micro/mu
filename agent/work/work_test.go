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
	"mu/internal/thread"
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

// Work that came out of a conversation answers back into it.
//
// Somebody writes in and asks for something that takes an hour. The task is
// where the work lives; the thread is where they asked, and where they are
// looking. Both get the answer — a result on a page nobody opened is a result
// nobody read.
func TestWorkFromAConversationAnswersBackToIt(t *testing.T) {
	const who = "work-thread"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	th := thread.Open(who, "mail", "<ask@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	answered(request{Account: who, Kind: tasks.Kind, ID: "t1", Thread: th.ID},
		"Here is what I found.", nil)

	msgs := thread.Messages(who, th.ID, 10)
	if len(msgs) != 1 {
		t.Fatalf("%d messages on the conversation, want the answer", len(msgs))
	}
	if msgs[0].Text != "Here is what I found." {
		t.Errorf("the conversation got %q", msgs[0].Text)
	}
	if msgs[0].Role != thread.RoleAgent {
		t.Errorf("the answer was recorded as %q", msgs[0].Role)
	}
}

// A failure goes back too. Silence is indistinguishable from work nobody
// picked up, which is the rule service/events had and could only half keep.
func TestAFailureGoesBackToTheConversation(t *testing.T) {
	const who = "work-thread-failed"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	th := thread.Open(who, "mail", "<broken@example.com>")
	answered(request{Account: who, Kind: tasks.Kind, ID: "t2", Thread: th.ID},
		"", fmt.Errorf("the model is unavailable"))

	msgs := thread.Messages(who, th.ID, 10)
	if len(msgs) != 1 || !strings.Contains(msgs[0].Text, "unavailable") {
		t.Errorf("the conversation was not told why: %+v", msgs)
	}
}

// Work nobody asked for in a conversation has nowhere to go back to, and that
// is not a failure — a task written on the page, a schedule falling due.
func TestWorkWithNoConversationAnswersNowhere(t *testing.T) {
	const who = "work-no-thread"
	auth.Create(&auth.Account{ID: who, Name: who, Secret: "test-secret"}) //nolint:errcheck

	answered(request{Account: who, Kind: tasks.Kind, ID: "t3"}, "Done.", nil)

	if got := thread.List(who, 10); len(got) != 0 {
		t.Errorf("a conversation was invented for work nobody asked for: %+v", got)
	}
}
