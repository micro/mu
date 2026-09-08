package tasks

import (
	"strings"
	"testing"
)

func TestTaskIdentityContextAndLegacyFailure(t *testing.T) {
	task := &Task{ID: "task", Owner: "alice", Agent: "opaque-agent-id", Assignee: Agent, Thread: "thread", Title: "Reply", Detail: "## Conversation\n\nFirst paragraph.\n\n- One\n- Two\n\n<img src=x onerror=alert(1)>", Result: "Last run failed: agent: ai generate failed after 3 attempt(s) (timeout): context deadline exceeded; inspect run history with micro inspect agent"}
	row := taskRow(task, "csrf", "Malten")
	for _, want := range []string{"Malten", `<details class="task-context">`, `<h2`, `<li>`, "too long"} {
		if !strings.Contains(row, want) {
			t.Errorf("missing %s: %s", want, row)
		}
	}
	for _, bad := range []string{"opaque-agent-id", "onerror=", "micro inspect", `class="task-context" open`} {
		if strings.Contains(row, bad) {
			t.Errorf("leaked %s", bad)
		}
	}
	if row := taskRow(task, "csrf", `<script>alert(1)</script>`); strings.Contains(row, "<script>") {
		t.Fatal("agent name injected markup")
	}
}
