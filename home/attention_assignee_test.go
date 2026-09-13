package home

import (
	"mu/service/tasks"
	"testing"
	"time"
)

func TestAttentionDoesNotEscalateAgentFailures(t *testing.T) {
	owner := "attention_assignee"
	for _, assignee := range []string{"agent", tasks.Me} {
		task, err := tasks.Create(owner, "Blocked work", "", assignee, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Update(owner, task.ID, "", "", tasks.StatusBlocked, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	items := attentionItems(owner, time.Now())
	if len(items) != 1 {
		t.Fatalf("got %d attention items, want only personally assigned work", len(items))
	}
}
