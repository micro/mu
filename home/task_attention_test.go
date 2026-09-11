package home

import (
	"strings"
	"testing"
	"time"

	"mu/service/tasks"
)

func TestTaskAttentionShowsFailedAndBlockedOwnedWork(t *testing.T) {
	who := "home-task-attention"
	defer tasks.DeleteAll(who)
	for _, state := range []string{tasks.StatusFailed, tasks.StatusBlocked} {
		task, err := tasks.Create(who, state, "", tasks.Agent, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Update(who, task.ID, "", "", state, "", "Review first"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tasks.Create(who, "<script>personal</script>", "", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(who, "Queued agent work", "", tasks.Agent, time.Time{}); err != nil {
		t.Fatal(err)
	}
	out := todoHTML(who)
	if strings.Contains(out, "<script>") || strings.Contains(out, "Queued agent work") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "&lt;script&gt;personal") {
		t.Fatal(out)
	}
	for _, part := range briefParts(who) {
		if strings.Contains(part, "retry") || strings.Contains(part, "blocked") || strings.Contains(part, "open.") {
			t.Fatal(part)
		}
	}
	for _, want := range []string{"/tasks?status=failed", "/tasks?status=blocked", "review before retrying", sectionRule("Todo")} {
		if !strings.Contains(out, want) {
			t.Errorf("attention summary missing %q: %s", want, out)
		}
	}
	if todoHTML("unrelated-owner") != "" || todoHTML("") != "" {
		t.Fatal("task attention crossed ownership")
	}
}
