package work

import (
	"strings"
	"testing"
	"time"

	"mu/agent"
	"mu/internal/thread"
	"mu/service/tasks"
)

func TestChatHandoffCarriesDestinationAndTask(t *testing.T) {
	who := t.Name()
	th := thread.Open(who, thread.ChatClient, "private-room-123")
	r := request{Account: who, Thread: th.ID, Kind: tasks.Kind, ID: "task-123", Prompt: "reply to this"}
	prompt := workPrompt(r)
	for _, want := range []string{"private-room-123", "task-123", "chat Send", "A draft or summary is not a request to send", "reply to this"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("missing %q in %s", want, prompt)
		}
	}
	r.Account = "someone-else"
	if strings.Contains(workPrompt(r), "private-room-123") {
		t.Fatal("leaked another account's room")
	}
}

func TestPanickedWorkClearsWorkingAndReportsFailure(t *testing.T) {
	who := t.Name()
	th := thread.Open(who, thread.ChatClient, "test-room")
	task, err := tasks.CreateOn(who, th.ID, "malten", "reply", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Update(who, task.ID, "", "", tasks.StatusDoing, "", ""); err != nil {
		t.Fatal(err)
	}
	runWithQuery(request{Account: who, Thread: th.ID, Kind: tasks.Kind, ID: task.ID, Agent: "malten", Prompt: "reply"}, func(string, string, agent.QueryOpts) (string, error) { panic("test panic") })
	got, err := tasks.Get(who, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tasks.Running(got) || !strings.Contains(got.Result, "stopped unexpectedly") {
		t.Fatalf("task still working: %+v", got)
	}
	msgs := thread.Messages(who, th.ID, 10)
	if len(msgs) != 1 || msgs[0].From != "malten" || !strings.Contains(msgs[0].Text, "stopped unexpectedly") {
		t.Fatalf("missing named failure: %+v", msgs)
	}
}

func TestSuccessfulWorkCompletesAndNamesReply(t *testing.T) {
	who := t.Name()
	th := thread.Open(who, thread.ChatClient, "test-room")
	task, err := tasks.CreateOn(who, th.ID, "malten", "reply", "", tasks.Agent, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	runWithQuery(request{Account: who, Thread: th.ID, Kind: tasks.Kind, ID: task.ID, Agent: "malten", Prompt: "reply"}, func(string, string, agent.QueryOpts) (string, error) {
		return `{"status":"done","summary":"The reply was sent.","evidence":["chat Send confirmed delivery"]}`, nil
	})
	got, err := tasks.Get(who, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != tasks.StatusDone || !strings.HasPrefix(got.Result, "The reply was sent.") {
		t.Fatalf("incomplete: %+v", got)
	}
	msgs := thread.Messages(who, th.ID, 10)
	if len(msgs) != 1 || msgs[0].From != "malten" {
		t.Fatalf("missing author: %+v", msgs)
	}
}
