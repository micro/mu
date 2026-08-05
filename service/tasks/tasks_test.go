package tasks

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-tasks-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// account makes owner a real account. Running a task checks the wallet, which
// starts by looking the account up — so a made-up owner cannot run anything,
// which is correct in production and has to be set up in a test.
func account(t *testing.T, owner string) {
	t.Helper()
	auth.Create(&auth.Account{ID: owner, Name: owner, Secret: "test-secret"})
}

func clear(owner string) {
	for _, t := range List(owner, "") {
		Remove(owner, t.ID)
	}
}

// A task needs enough to be actionable and nothing more.
func TestCreateNeedsATitle(t *testing.T) {
	clear("alice")
	defer clear("alice")

	if _, err := Create("alice", "  ", "detail", "", time.Time{}); err == nil {
		t.Error("a task with no title was created")
	}
	if _, err := Create("", "Something", "", "", time.Time{}); err == nil {
		t.Error("a signed-out caller created a task")
	}

	task, err := Create("alice", "Book the flights", "LHR to JFK, week of the 12th", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusTodo {
		t.Errorf("a new task starts as %q, want %q", task.Status, StatusTodo)
	}
	if task.Assignee != Me {
		t.Errorf("a new task is assigned to %q; handing work to the agent should be something you said", task.Assignee)
	}
}

// Handing work over is the whole point of the assignee, and it has to be
// deliberate — anything unrecognised stays yours.
func TestAssignment(t *testing.T) {
	clear("alice")
	defer clear("alice")

	mine, _ := Create("alice", "Mine", "", "", time.Time{})
	theirs, _ := Create("alice", "Theirs", "", "agent", time.Time{})
	odd, _ := Create("alice", "Odd", "", "sarah", time.Time{})

	if mine.Assignee != Me || odd.Assignee != Me {
		t.Errorf("assignment defaulted wrongly: %q, %q", mine.Assignee, odd.Assignee)
	}
	if theirs.Assignee != Agent {
		t.Errorf("assigning to the agent failed: %q", theirs.Assignee)
	}
}

// Next is what an agent asks when it wants to know what to do. It must be the
// oldest open task assigned to it, and nothing else.
func TestNextIsTheOldestOpenAgentTask(t *testing.T) {
	clear("alice")
	defer clear("alice")

	at := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	now = func() time.Time { return at }
	defer func() { now = time.Now }()

	Create("alice", "Mine, not the agent's", "", "", time.Time{})
	at = at.Add(time.Minute)
	first, _ := Create("alice", "First for the agent", "", "agent", time.Time{})
	at = at.Add(time.Minute)
	Create("alice", "Second for the agent", "", "agent", time.Time{})

	got := Next("alice")
	if got == nil || got.ID != first.ID {
		t.Fatalf("Next returned %+v, want the first agent task", got)
	}

	// Finished work is not next.
	Update("alice", first.ID, "", "", StatusDone, "", "did it")
	got = Next("alice")
	if got == nil || got.Title != "Second for the agent" {
		t.Fatalf("after finishing the first, Next returned %+v", got)
	}
}

// An agent finishing a task sends a status and a result. It should not have to
// restate the title it was given.
func TestUpdateLeavesUntouchedFieldsAlone(t *testing.T) {
	clear("alice")
	defer clear("alice")

	task, _ := Create("alice", "Summarise the news", "Top five stories", "agent", time.Time{})
	updated, err := Update("alice", task.ID, "", "", StatusDone, "", "Five stories, mailed.")
	if err != nil {
		t.Fatal(err)
	}

	if updated.Title != "Summarise the news" || updated.Detail != "Top five stories" {
		t.Errorf("an update lost fields it was not given: %+v", updated)
	}
	if updated.Status != StatusDone || updated.Result != "Five stories, mailed." {
		t.Errorf("the update did not take: %+v", updated)
	}
	if updated.Assignee != Agent {
		t.Errorf("the assignee changed on its own: %q", updated.Assignee)
	}
	if !updated.Updated.After(updated.Created) && updated.Updated.IsZero() {
		t.Error("updated time was not set")
	}
}

func TestUpdateRefusesAnUnknownStatus(t *testing.T) {
	clear("alice")
	defer clear("alice")

	task, _ := Create("alice", "A task", "", "", time.Time{})
	if _, err := Update("alice", task.ID, "", "", "finished", "", ""); err == nil {
		t.Error("an invented status was accepted")
	}
}

// A list of work is read from the top: what is open, oldest first.
func TestOpenTasksComeFirst(t *testing.T) {
	clear("alice")
	defer clear("alice")

	at := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	now = func() time.Time { return at }
	defer func() { now = time.Now }()

	old, _ := Create("alice", "Old", "", "", time.Time{})
	at = at.Add(time.Hour)
	Create("alice", "New", "", "", time.Time{})
	Update("alice", old.ID, "", "", StatusDone, "", "")

	list := List("alice", "")
	if len(list) != 2 {
		t.Fatalf("got %d tasks", len(list))
	}
	if list[0].Title != "New" || list[1].Title != "Old" {
		t.Errorf("finished work is not last: %s, %s", list[0].Title, list[1].Title)
	}

	if open := List("alice", StatusTodo); len(open) != 1 || open[0].Title != "New" {
		t.Errorf("the todo filter returned %+v", open)
	}
}

// A task list is one person's. Nobody else reads it or changes it.
func TestTasksArePrivateToTheirOwner(t *testing.T) {
	clear("alice")
	clear("bob")
	defer clear("alice")

	task, _ := Create("alice", "Alice's task", "", "", time.Time{})

	if len(List("bob", "")) != 0 {
		t.Error("bob sees alice's tasks")
	}
	if _, err := Get("bob", task.ID); err == nil {
		t.Error("bob read alice's task")
	}
	if _, err := Update("bob", task.ID, "", "", StatusDone, "", "nope"); err == nil {
		t.Error("bob changed alice's task")
	}
	if Next("bob") != nil {
		t.Error("bob's next task is one of alice's")
	}
	if list := List("alice", ""); len(list) != 1 || list[0].Status != StatusTodo {
		t.Errorf("alice's task was altered: %+v", list)
	}
}

// A model reads this. It has to say what to do, what state it is in, and what
// came of it — including the id, or nothing can be updated.
func TestRenderIsReadableByAModel(t *testing.T) {
	clear("alice")
	defer clear("alice")

	task, _ := Create("alice", "Check the flights", "LHR to JFK", "agent",
		time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC))
	Update("alice", task.ID, "", "", StatusDone, "", "Booked BA117")

	out := Render(List("alice", ""))
	for _, want := range []string{"[done]", "Check the flights", "assigned to the agent",
		"due 2026-08-09", "id: " + task.ID, "LHR to JFK", "result: Booked BA117"} {
		if !strings.Contains(out, want) {
			t.Errorf("Render is missing %q:\n%s", want, out)
		}
	}

	if got := Render(nil); got != "No tasks." {
		t.Errorf("an empty list renders as %q", got)
	}
}

// Running is what makes this more than a to-do list: the agent does the work
// and the answer lands back on the task.
func TestRunHandsTheTaskToTheAgent(t *testing.T) {
	account(t, "alice")
	clear("alice")
	defer clear("alice")

	done := make(chan string, 1)
	RunAgent = func(accountID, prompt string, onStep func(Step)) (string, error) {
		done <- prompt
		return "Here is the summary.", nil
	}
	defer func() { RunAgent = nil }()

	task, _ := Create("alice", "Summarise the news", "Top five", "agent", time.Time{})
	if err := Run("alice", task.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case prompt := <-done:
		// Title and detail both reach the agent: the title is the instruction,
		// the detail is the context.
		if !strings.Contains(prompt, "Summarise the news") || !strings.Contains(prompt, "Top five") {
			t.Errorf("the agent was asked %q", prompt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the agent was never asked")
	}

	// The goroutine writes the result; wait for it to land.
	for i := 0; i < 100; i++ {
		if got, _ := Get("alice", task.ID); got.Status == StatusDone {
			if got.Result != "Here is the summary." {
				t.Errorf("the result was not stored: %q", got.Result)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the task never finished")
}

// A failed run leaves the work to be done rather than marking it finished
// badly, and says why where someone will see it.
func TestAFailedRunReopensTheTask(t *testing.T) {
	account(t, "alice")
	clear("alice")
	defer clear("alice")

	RunAgent = func(accountID, prompt string, onStep func(Step)) (string, error) {
		return "", fmt.Errorf("the model is unavailable")
	}
	defer func() { RunAgent = nil }()

	task, _ := Create("alice", "Something hard", "", "agent", time.Time{})
	if err := Run("alice", task.ID); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 100; i++ {
		got, _ := Get("alice", task.ID)
		if got.Status == StatusTodo && got.Result != "" {
			if !strings.Contains(got.Result, "unavailable") {
				t.Errorf("the reason was not recorded: %q", got.Result)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a failed run did not leave the task to be done")
}

func TestRunRefusesWhatCannotBeRun(t *testing.T) {
	account(t, "alice")
	clear("alice")
	defer clear("alice")

	RunAgent = func(string, string, func(Step)) (string, error) { return "ok", nil }
	defer func() { RunAgent = nil }()

	task, _ := Create("alice", "Already done", "", "agent", time.Time{})
	Update("alice", task.ID, "", "", StatusDone, "", "done")
	if err := Run("alice", task.ID); err == nil {
		t.Error("a finished task was run again")
	}
	if err := Run("alice", "no-such-id"); err == nil {
		t.Error("a task that does not exist was run")
	}
	if err := Run("bob", task.ID); err == nil {
		t.Error("another account ran this task")
	}
}

// Two tasks added in the same second still have an order. Stored to the second,
// they compared equal and the list showed whichever the store returned first.
func TestTasksAddedTogetherKeepTheirOrder(t *testing.T) {
	clear("ordering")
	defer clear("ordering")

	first, _ := Create("ordering", "First", "", "", time.Time{})
	second, _ := Create("ordering", "Second", "", "", time.Time{})
	if !first.Created.Before(second.Created) {
		t.Fatalf("timestamps do not distinguish two quick creates: %v vs %v",
			first.Created, second.Created)
	}

	list := List("ordering", "")
	if len(list) != 2 || list[0].Title != "First" {
		t.Errorf("open tasks are not oldest first: %+v", list)
	}
}
