package tasks

import (
	"os"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/event"
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

// Running is what makes this more than a to-do list: the work is given away.
//
// It used to be given away by calling a RunAgent hook this package declared,
// which is a service running an agent. It announces now, and what happens to
// the answer is agent/work's — including a failed run reopening the task,
// which is tested there.
func TestRunAsksForTheWork(t *testing.T) {
	account(t, "alice")
	clear("alice")
	defer clear("alice")

	sub := event.Subscribe(event.WorkForAgent)
	defer sub.Close()

	task, _ := Create("alice", "Summarise the news", "Top five", "agent", time.Time{})
	if err := Run("alice", task.ID); err != nil {
		t.Fatal(err)
	}

	select {
	case e := <-sub.Chan:
		// Title and detail both reach the agent: the title is the instruction,
		// the detail is the context.
		prompt, _ := e.Data["prompt"].(string)
		if !strings.Contains(prompt, "Summarise the news") || !strings.Contains(prompt, "Top five") {
			t.Errorf("the agent was asked %q", prompt)
		}
		// The kind and the id are how the answer finds its way back.
		if e.Data["kind"] != Kind || e.Data["id"] != task.ID {
			t.Errorf("the request does not name this task: %v", e.Data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the agent was never asked")
	}

	// And it is marked before it is announced, so the page says what is
	// happening the moment the press returns.
	if got, _ := Get("alice", task.ID); got.Status != StatusDoing {
		t.Errorf("the task is %q, want doing", got.Status)
	}
}

// A second press does not start a second run.
//
// The status is the lock, and it is the one a reader can see. There was an
// in-memory map beside it doing the same job, which meant two answers to "is
// this running" and only one survived a restart — a task left doing by a crash
// could never be run again.
func TestATaskAlreadyWithTheAgentIsNotRunTwice(t *testing.T) {
	account(t, "alice")
	clear("alice")
	defer clear("alice")

	task, _ := Create("alice", "Once", "", "agent", time.Time{})
	if err := Run("alice", task.ID); err != nil {
		t.Fatal(err)
	}
	if err := Run("alice", task.ID); err == nil {
		t.Error("a second run was accepted while the first was still going")
	}
}

func TestRunRefusesWhatCannotBeRun(t *testing.T) {
	account(t, "alice")
	clear("alice")
	defer clear("alice")

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

// An update keeps what it was not asked to change.
//
// Update rebuilds the record from scratch and carries forward only the fields
// it names, so a field added to Task and not added here is one that any edit
// silently deletes — its own comment says so, and the agent field was added
// without it. The symptom was specific and easy to miss: Run reads the task,
// then moves it to "doing" through Update, then announces. The announcement was
// right because it came off the in-memory copy; the stored task lost its agent
// the instant work started, so /tasks could not say who was doing it.
func TestAnUpdateDoesNotDropWhatItWasNotGiven(t *testing.T) {
	const owner = "tasks-carry"
	made, err := CreateOn(owner, "thread-77", "research", "Do the thing", "with detail", Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// The update Run makes: a status and nothing else.
	after, err := Update(owner, made.ID, "", "", StatusDoing, Agent, "")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ what, want, got string }{
		{"agent", "research", after.Agent},
		{"thread", "thread-77", after.Thread},
		{"detail", "with detail", after.Detail},
		{"title", "Do the thing", after.Title},
	} {
		if c.got != c.want {
			t.Errorf("after an update that only set the status, %s = %q, want %q",
				c.what, c.got, c.want)
		}
	}

	// And from the store, not just the returned value.
	reloaded, err := Get(owner, made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Agent != "research" {
		t.Errorf("reloaded agent = %q, want research", reloaded.Agent)
	}
}
