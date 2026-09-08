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
	out := taskAttention(who)
	for _, want := range []string{"/tasks?status=failed", "/tasks?status=blocked", "review before retrying"} {
		if !strings.Contains(out, want) {
			t.Errorf("attention summary missing %q: %s", want, out)
		}
	}
	if taskAttention("unrelated-owner") != "" || taskAttention("") != "" {
		t.Fatal("task attention crossed ownership")
	}
}
