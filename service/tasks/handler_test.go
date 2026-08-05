package tasks

import (
	"strings"
	"testing"
	"time"
)

// A model writes markdown. Shown raw, a list of findings is a wall of asterisks
// and hashes, which reads as a bug in the thing that produced it.
func TestAResultIsRenderedNotDumped(t *testing.T) {
	clear("md")
	defer clear("md")

	task, _ := Create("md", "Summarise", "", "agent", time.Time{})
	Update("md", task.ID, "", "", StatusDone, "", "## Findings\n\n- one\n- two")

	row := taskRow(mustGet(t, "md", task.ID), "csrf")
	if !strings.Contains(row, "<h2") || !strings.Contains(row, "<li>") {
		t.Errorf("the result was not rendered as markdown:\n%s", row)
	}
}

// The result came out of a model that had just read news articles and web
// pages, so any HTML in it is HTML somebody else wrote.
func TestAResultCannotInjectHTML(t *testing.T) {
	clear("md2")
	defer clear("md2")

	task, _ := Create("md2", "Summarise", "", "agent", time.Time{})
	Update("md2", task.ID, "", "", StatusDone, "", "Found this: <img src=x onerror=alert(1)>")

	row := taskRow(mustGet(t, "md2", task.ID), "csrf")
	if strings.Contains(row, "onerror=alert(1)") {
		t.Errorf("raw HTML from a result reached the page:\n%s", row)
	}
}

func mustGet(t *testing.T, owner, id string) *Task {
	t.Helper()
	got, err := Get(owner, id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A run takes seconds to a minute. While one is in flight the row has to say
// so, and the page has to notice when it lands — it was static, so the only way
// to find out was to reload and guess.
func TestARunningTaskSaysSoAndThePageWatchesForIt(t *testing.T) {
	account(t, "watch")
	clear("watch")
	defer clear("watch")

	release := make(chan struct{})
	started := make(chan struct{})
	RunAgent = func(string, string, func(Step)) (string, error) {
		close(started)
		<-release
		return "done", nil
	}
	defer func() { RunAgent = nil }()

	task, _ := Create("watch", "Slow thing", "", "agent", time.Time{})
	if err := Run("watch", task.ID); err != nil {
		t.Fatal(err)
	}
	<-started

	if !Running(task.ID) {
		t.Fatal("a task with the agent does not report as running")
	}
	row := taskRow(mustGet(t, "watch", task.ID), "csrf")
	if !strings.Contains(row, "task-running") {
		t.Errorf("a running task does not say it is working:\n%s", row)
	}

	// And the watcher has to actually reload rather than poll forever.
	if !strings.Contains(taskPollJS, "location.reload") {
		t.Error("the poll never refreshes the page")
	}
	if !strings.Contains(taskPollJS, "'doing'") {
		t.Error("the poll does not look at whether anything is still running")
	}

	close(release)
}

// A finished task shows what the agent did, not only what it concluded. A
// paragraph on its own gives no way to tell research from invention.
func TestARunRecordsAndShowsItsSteps(t *testing.T) {
	account(t, "steps")
	clear("steps")
	defer clear("steps")

	RunAgent = func(_, _ string, onStep func(Step)) (string, error) {
		onStep(Step{Tool: "web_search", Detail: "latest AI news", OK: true, Seconds: 1.2})
		onStep(Step{Tool: "mail_send", OK: false, Seconds: 0.3})
		return "Here is the summary.", nil
	}
	defer func() { RunAgent = nil }()

	task, _ := Create("steps", "Summarise", "", "agent", time.Time{})
	if err := Run("steps", task.ID); err != nil {
		t.Fatal(err)
	}

	var got *Task
	for i := 0; i < 100; i++ {
		got = mustGet(t, "steps", task.ID)
		if got.Status == StatusDone {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("recorded %d steps, want 2: %+v", len(got.Steps), got.Steps)
	}
	if got.Steps[0].Tool != "web_search" || got.Steps[0].Detail != "latest AI news" {
		t.Errorf("the first step lost its detail: %+v", got.Steps[0])
	}
	if got.Steps[1].OK {
		t.Error("a failed tool was recorded as having worked")
	}

	row := taskRow(got, "csrf")
	for _, want := range []string{"2 steps", "web_search", "latest AI news", "task-step failed"} {
		if !strings.Contains(row, want) {
			t.Errorf("the steps list is missing %q:\n%s", want, row)
		}
	}
}

// An ordinary edit is not a reason to forget what the agent did.
func TestEditingATaskKeepsItsSteps(t *testing.T) {
	clear("keep")
	defer clear("keep")

	task, _ := Create("keep", "Thing", "", "agent", time.Time{})
	Update("keep", task.ID, "", "", StatusDone, "", "answer",
		[]Step{{Tool: "news_list", OK: true, Seconds: 0.4}})

	after, err := Update("keep", task.ID, "", "", StatusTodo, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Steps) != 1 || after.Steps[0].Tool != "news_list" {
		t.Errorf("reopening the task dropped its steps: %+v", after.Steps)
	}
}

// The detail beside a tool name is the argument that carries meaning, and only
// that — a summary line is not a place to spill the caller's data.
func TestStepDetailPicksTheQuery(t *testing.T) {
	if got := StepDetail(map[string]any{"lat": 51.5, "query": "AI news"}); got != "AI news" {
		t.Errorf("StepDetail = %q, want the query", got)
	}
	if got := StepDetail(map[string]any{"lat": 51.5, "lon": -0.12}); got != "" {
		t.Errorf("StepDetail = %q, want nothing worth showing", got)
	}
	long := strings.Repeat("x", 200)
	if got := StepDetail(map[string]any{"q": long}); len([]rune(got)) > 61 {
		t.Errorf("StepDetail returned %d runes; a glance line must stay short", len([]rune(got)))
	}
}
