package home

import (
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestOverviewSummarisesOnlyOwnedPersonalActions(t *testing.T) {
	const owner = "overview-action-owner"
	defer tasks.DeleteAll(owner)
	if _, err := tasks.Create(owner, "Private errand", "", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Create(owner, "Blocked agent request", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.Update(owner, task.ID, "", "", tasks.StatusBlocked, "", "Review needed"); err != nil {
		t.Fatal(err)
	}
	got := overviewHTML(owner)
	for _, want := range []string{"1 thing needs your attention", "1 thing to do"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
		for _, other := range []string{"", "overview-other-owner"} {
			if strings.Contains(overviewHTML(other), want) {
				t.Fatal("overview crossed account boundary")
			}
		}
	}
	if strings.Contains(got, "peek-row") || strings.Contains(got, "Private errand") {
		t.Fatal("overview became a duplicate task list")
	}
}
