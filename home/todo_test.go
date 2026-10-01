package home

import (
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestTodoUsesOutstandingOwnedWork(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "todo-owner"
	makeTask := func(account, title, assignee, status string) *tasks.Task {
		task, err := tasks.Create(account, title, "", assignee, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if status != tasks.StatusTodo {
			task, err = tasks.Update(account, task.ID, "", "", status, "", "")
			if err != nil {
				t.Fatal(err)
			}
		}
		return task
	}
	mine := makeTask(owner, "Review <proposal>", tasks.Me, tasks.StatusTodo)
	makeTask(owner, "Blocked job", tasks.Agent, tasks.StatusBlocked)
	makeTask(owner, "Failed job", tasks.Agent, tasks.StatusFailed)
	makeTask(owner, "Agent queue", tasks.Agent, tasks.StatusTodo)
	makeTask(owner, "Completed job", tasks.Me, tasks.StatusDone)
	makeTask("foreign-todo-owner", "Foreign secret", tasks.Me, tasks.StatusTodo)
	body := todoHTML(owner)
	for _, want := range []string{"Review &lt;proposal&gt;", "/work?id=" + mine.ID, "Blocked job", "Failed job"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, absent := range []string{"Agent queue", "Completed job", "Foreign secret", "caught up"} {
		if strings.Contains(body, absent) {
			t.Fatalf("unexpected %q", absent)
		}
	}
	if _, err := tasks.Update(owner, mine.ID, "", "", tasks.StatusDone, "", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(todoHTML(owner), mine.ID) {
		t.Fatal("completed action remained on Home")
	}
	if todoHTML("empty-todo-owner") != "" {
		t.Fatal("empty Todo should be hidden")
	}
}
