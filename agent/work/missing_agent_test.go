package work

import (
	"testing"
	"time"

	"mu/agent"
	"mu/internal/auth"
	"mu/service/tasks"
)

func TestMissingAgentBlocksQueuedWorkWithoutCallingDefault(t *testing.T) {
	const who = "missing_agent_work"
	if err := auth.Create(&auth.Account{ID: who, Name: who, Secret: "test"}); err != nil {
		t.Fatal(err)
	}
	a, _, err := agent.CreateAgent(who, "Temporary", agent.Hosted, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Create(who, "Private work", "Read mail", "agent", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.RemoveAgent(who, a.ID); err != nil {
		t.Fatal(err)
	}
	called := false
	runWithQuery(request{Account: who, ID: task.ID, Kind: tasks.Kind, Agent: a.ID, Prompt: task.Detail}, func(string, string, agent.QueryOpts) (string, error) {
		called = true
		return "must not execute", nil
	})
	got, err := tasks.Get(who, task.ID)
	if err != nil || called || got.Status != tasks.StatusBlocked {
		t.Fatalf("missing agent did not block: called=%v task=%+v err=%v", called, got, err)
	}
}
