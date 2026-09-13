package work

import (
	"context"
	"mu/agent"
	"mu/service/tasks"
	"strings"
	"testing"
	"time"
)

func TestTimeoutPreservesCompletedSteps(t *testing.T) {
	const who = "timeout_steps"
	namedAgent(t, who)
	task, err := tasks.CreateOn(who, "", "malten", "Reply", "Context", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Remove(who, task.ID)
	runWithQuery(request{Account: who, Kind: tasks.Kind, ID: task.ID, Agent: "malten", Prompt: "Reply"}, func(_, _ string, opts agent.QueryOpts) (string, error) {
		opts.OnStep(agent.Step{Tool: "web_search", OK: true})
		return "", context.DeadlineExceeded
	})
	got, err := tasks.Get(who, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != tasks.StatusFailed || len(got.Steps) != 1 || got.Steps[0].Tool != "web_search" || !strings.Contains(got.Result, "too long") {
		t.Fatalf("failure lost work: %+v", got)
	}
}
