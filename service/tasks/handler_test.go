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
	RunAgent = func(string, string) (string, error) {
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
