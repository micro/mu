package work

import (
	"strings"
	"testing"
	"time"

	"mu/agent"
	"mu/service/tasks"
)

func TestTaskOutcomeDoesNotCompleteFromInspectionOrUnverifiedClaims(t *testing.T) {
	for _, tc := range []struct{ name, reply, status string }{
		{"inspection", "- Assistant by Asim\n- No problems found.\n- What can I help with?", tasks.StatusFailed},
		{"empty", "", tasks.StatusFailed},
		{"unverified", `{"status":"done","summary":"Added Copy","evidence":[]}`, tasks.StatusBlocked},
		{"blocked", `{"status":"blocked","summary":"Clipboard access was denied; the change is not verified"}`, tasks.StatusBlocked},
		{"read-only", `{"status":"done","summary":"Two events tomorrow","evidence":["Calendar returned two events for the requested date"]}`, tasks.StatusDone},
		{"unknown", `{"status":"maybe","summary":"Not sure"}`, tasks.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			who := t.Name()
			task, err := tasks.Create(who, "Do work", "", tasks.Agent, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			defer tasks.Remove(who, task.ID)
			runWithQuery(request{Account: who, Kind: tasks.Kind, ID: task.ID, Prompt: "Do work"}, func(_, prompt string, opts agent.QueryOpts) (string, error) {
				if !opts.RawReply || opts.OutputInstruction != outcomeInstruction || opts.System != "" {
					t.Fatal("task outcome can be replaced by a tool summary")
				}
				if strings.Contains(prompt, "completes this task automatically") {
					t.Fatal("prompt promises unconditional completion")
				}
				opts.OnStep(agent.Step{Tool: "shell_run", OK: false})
				return tc.reply, nil
			})
			got, err := tasks.Get(who, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.status || len(got.Steps) != 1 || got.Steps[0].OK {
				t.Fatalf("outcome lost: %+v", got)
			}
			if tc.name == "blocked" && !strings.Contains(got.Result, "Clipboard access was denied") {
				t.Fatal("lost actionable blocker")
			}
		})
	}
}
