package inbox

// The agent, on the thing you are reading.

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/tasks"
)

// reader is an account that exists, which the quota check needs before it will
// let a run happen.
//
// Idempotent, because accounts are stored on disk and a second run of the suite
// on the same machine finds the one the first run made. A test that fails only
// the second time it is run is a worse thing to own than this line.
func reader(t *testing.T, id string) {
	t.Helper()
	if _, err := auth.GetAccount(id); err == nil {
		return
	}
	if err := auth.Create(&auth.Account{ID: id, Name: id, Created: time.Now()}); err != nil {
		t.Fatalf("could not create %s: %v", id, err)
	}
}

// The box is on the conversation, because that is where the work is. The whole
// move is that the details are already in the messages above it.
func TestAConversationCarriesTheAskBox(t *testing.T) {
	Act = func(accountID, threadID, ask string) error { return nil }
	t.Cleanup(func() { Act = nil })

	r := httptest.NewRequest("GET", "/inbox?id=x", nil)
	body := askBox(r, "x", "")

	for _, want := range []string{`method="post"`, `action="/inbox"`,
		`name="id" value="x"`, `name="_csrf"`, `name="ask"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the box is missing %s:\n%s", want, body)
		}
	}
	// And it says what it is not, because a box under a message looks like a
	// reply and is not one.
	if !strings.Contains(body, "not a reply") {
		t.Error("nothing says this is not a reply")
	}
	// Two buttons, and the difference is whether you are waiting: Ask answers
	// now, Hand over makes a task and you can close the tab.
	if !strings.Contains(body, `name="hand"`) {
		t.Errorf("there is no way to hand the conversation over:\n%s", body)
	}
}

// With no agent wired in there is no box. An instruction nothing can carry out
// is a control that does nothing.
func TestNoAgentMeansNoBox(t *testing.T) {
	Act = nil
	if got := askBox(httptest.NewRequest("GET", "/inbox?id=x", nil), "x", ""); got != "" {
		t.Errorf("a box was drawn with nothing behind it: %s", got)
	}
}

// Somebody else's conversation is not a conversation. Scoped by account, so an
// id from elsewhere is not "forbidden" — there is no such thing here.
func TestYouCanOnlyActOnYourOwnConversation(t *testing.T) {
	var ran string
	Act = func(accountID, threadID, ask string) error {
		ran = threadID
		return nil
	}
	t.Cleanup(func() { Act = nil })

	theirs := thread.Open("act-somebody-else", "mail", "<x@example.com>")
	if theirs == nil {
		t.Fatal("no conversation")
	}

	form := url.Values{"id": {theirs.ID}, "ask": {"summarise this"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	action(w, r, "act-me")

	if ran != "" {
		t.Errorf("the agent was run on somebody else's conversation (%s)", ran)
	}
	if w.Code != 404 {
		t.Errorf("answered %d", w.Code)
	}
}

// An empty instruction runs nothing and costs nothing. Somebody pressed the
// button.
func TestAnEmptyInstructionDoesNothing(t *testing.T) {
	var ran bool
	Act = func(accountID, threadID, ask string) error { ran = true; return nil }
	t.Cleanup(func() { Act = nil })

	mine := thread.Open("act-empty", "mail", "<y@example.com>")
	form := url.Values{"id": {mine.ID}, "ask": {"   "}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	action(w, r, "act-empty")

	if ran {
		t.Error("an empty instruction was run")
	}
	if w.Code != 303 {
		t.Errorf("answered %d, want a redirect back", w.Code)
	}
}

// The instruction and what came of it are recorded on the conversation they are
// about, so the thread reads as what arrived, what was asked, and what was done.
func TestTheInstructionLandsOnTheConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// A fresh account per run. The record loads from disk at package init,
	// before a test can point HOME somewhere else, so a fixed id accumulates
	// across runs and the counts below drift.
	who := fmt.Sprintf("act-record-%d", time.Now().UnixNano())
	reader(t, who)
	mine := thread.Open(who, "mail", "<z@example.com>")
	thread.Add(thread.Message{Thread: mine.ID, Account: who, Text: "dinner on the 4th at 8"})

	Act = func(accountID, threadID, ask string) error {
		// What the wiring does: the agent's own door, on this conversation.
		thread.Add(thread.Message{Thread: threadID, Account: accountID, Text: ask})
		thread.Add(thread.Message{Thread: threadID, Account: accountID,
			Role: thread.RoleAgent, Text: "Added — Dinner, 4th, 20:00."})
		return nil
	}
	t.Cleanup(func() { Act = nil })

	form := url.Values{"id": {mine.ID}, "ask": {"add this to my calendar"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	action(httptest.NewRecorder(), r, who)

	msgs := thread.Messages(who, mine.ID, 10)
	if len(msgs) != 3 {
		t.Fatalf("the conversation holds %d messages, want what arrived, the ask and the answer", len(msgs))
	}
	if !strings.Contains(msgs[1].Text, "calendar") || msgs[2].Role != thread.RoleAgent {
		t.Errorf("the record reads wrong: %+v", msgs)
	}
}

// Handing a conversation over makes a task that answers back to it.
//
// The difference between the two buttons is not how long it takes — it is
// whether you are waiting. Ask runs now; Hand over makes work and you can close
// the tab, and the answer arrives on the conversation you were reading.
func TestHandingOverMakesATaskOnTheConversation(t *testing.T) {
	const who = "inbox-hand"
	reader(t, who)

	said := ""
	AgentSaid(func(accountID, threadID, text string) { said = text })
	t.Cleanup(func() { AgentSaid(func(string, string, string) {}) })

	th := thread.Open(who, "mail", "<hand@example.com>")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Name(who, th.ID, "The quarterly numbers")
	agentSaidNothing := thread.Add(thread.Message{
		Thread: th.ID, Account: who, Role: thread.RolePerson,
		Text: "Can you pull together the quarterly numbers?", From: "them@example.com",
	})
	_ = agentSaidNothing

	if err := hand(who, th, "Pull the numbers and summarise them"); err != nil {
		t.Fatal(err)
	}

	// Found by the conversation rather than by counting: this package has no
	// TestMain, so its store is the real one and outlives a run.
	var got *tasks.Task
	for _, candidate := range tasks.List(who, "") {
		if candidate.Thread == th.ID {
			got = candidate
			break
		}
	}
	if got == nil {
		t.Fatal("no task was made for the conversation")
	}
	if got.Assignee != tasks.Agent {
		t.Errorf("the task was not given to the agent: %q", got.Assignee)
	}
	// The conversation travels with it, or the run starts cold — which is why
	// work handed off in one sentence comes back worse than the same request
	// made in a conversation that already has the context.
	for _, want := range []string{"The quarterly numbers", "quarterly numbers?",
		"Pull the numbers and summarise them"} {
		if !strings.Contains(got.Detail, want) {
			t.Errorf("the task lost %q from the conversation:\n%s", want, got.Detail)
		}
	}
	// And it says so, because a task made silently is a task nobody knows was
	// made — and because deciding this was work rather than a question is a
	// claim about what somebody meant.
	if !strings.Contains(said, "Taking that on") {
		t.Errorf("nothing was said on the conversation: %q", said)
	}
}
