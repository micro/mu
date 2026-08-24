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
func TestAConversationCarriesTheAssignDialog(t *testing.T) {
	r := httptest.NewRequest("GET", "/inbox?id=x", nil)
	body := assignDialog(r, "dlg-x", &thread.Thread{ID: "x"}, "")

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
	// One button, and it is the async one. The other ran the agent inside the
	// POST and made you wait at a dead page for a model call — a chat with the
	// streaming taken out, on the page that exists so you do not have to wait.
	if !strings.Contains(body, ">Assign</button>") {
		t.Errorf("there is no way to hand the conversation over:\n%s", body)
	}
	// And it is a dialog rather than a permanent box under the thread, which is
	// what "clutters the view" was about: a textarea, three pills and a caption
	// on every conversation whether or not you meant to hand it over.
	if !strings.Contains(body, "<dialog") {
		t.Errorf("the box is not in a dialog, so it is on the page always:\n%s", body)
	}
	if strings.Contains(body, ">Ask<") {
		t.Errorf("the synchronous button is back:\n%s", body)
	}
	// And the caption says the thing the page is for.
	if !strings.Contains(body, "close the tab") {
		t.Errorf("nothing says you can walk away:\n%s", body)
	}
}

// Somebody else's conversation is not a conversation. Scoped by account, so an
// id from elsewhere is not "forbidden" — there is no such thing here.
func TestYouCanOnlyActOnYourOwnConversation(t *testing.T) {
	reader(t, "act-me")
	theirs := thread.Open("act-somebody-else", "mail", "<x@example.com>")
	if theirs == nil {
		t.Fatal("no conversation")
	}

	form := url.Values{"id": {theirs.ID}, "ask": {"summarise this"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	action(w, r, "act-me")

	if w.Code != 404 {
		t.Errorf("answered %d", w.Code)
	}
	if taskOn(t, "act-me", theirs.ID) != nil || taskOn(t, "act-somebody-else", theirs.ID) != nil {
		t.Error("work was made out of somebody else's conversation")
	}
}

// taskOn is the task made for a conversation, or nil.
//
// Found by the conversation rather than by counting: this package has no
// TestMain, so its store is the real one and outlives a run.
func taskOn(t *testing.T, who, threadID string) *tasks.Task {
	t.Helper()
	for _, candidate := range tasks.List(who, "") {
		if candidate.Thread == threadID {
			return candidate
		}
	}
	return nil
}

// An empty instruction runs nothing and costs nothing. Somebody pressed the
// button.
func TestAnEmptyInstructionDoesNothing(t *testing.T) {
	reader(t, "act-empty")
	mine := thread.Open("act-empty", "mail", "<y@example.com>")
	form := url.Values{"id": {mine.ID}, "ask": {"   "}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	action(w, r, "act-empty")

	if taskOn(t, "act-empty", mine.ID) != nil {
		t.Error("an empty instruction made work")
	}
	if w.Code != 303 {
		t.Errorf("answered %d, want a redirect back", w.Code)
	}
}

// The POST hands the conversation over, and the thread says so.
//
// This used to assert the other half: an instruction ran the agent inside the
// request and the answer was on the thread when the page came back. That
// control is gone — it was a chat with the streaming taken out, on the page
// that exists so nobody has to wait — so what the POST does now is make work,
// and what somebody sees immediately is the agent saying it has been taken on.
func TestTheInstructionLandsOnTheConversation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// A fresh account per run. The record loads from disk at package init,
	// before a test can point HOME somewhere else, so a fixed id accumulates
	// across runs and the counts below drift.
	who := fmt.Sprintf("act-record-%d", time.Now().UnixNano())
	reader(t, who)
	mine := thread.Open(who, "mail", "<z@example.com>")
	thread.Add(thread.Message{Thread: mine.ID, Account: who, Text: "dinner on the 4th at 8"})

	said := ""
	AgentSaid(func(accountID, threadID, text string) { said = text })
	t.Cleanup(func() { AgentSaid(func(string, string, string) {}) })

	form := url.Values{"id": {mine.ID}, "ask": {"add this to my calendar"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	action(httptest.NewRecorder(), r, who)

	task := taskOn(t, who, mine.ID)
	if task == nil {
		t.Fatal("the POST made no work out of the conversation")
	}
	// The conversation travels with it, so the run does not start cold.
	for _, want := range []string{"dinner on the 4th at 8", "add this to my calendar"} {
		if !strings.Contains(task.Detail, want) {
			t.Errorf("the task lost %q:\n%s", want, task.Detail)
		}
	}
	if said == "" {
		t.Error("nothing was said on the conversation, so the press left no trace")
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

// The conversation is handed to the agent it is already with.
//
// Every hand-over made a task with no agent on it, so agent/work ran whichever
// agent the instance runs by default — whatever the thread was. A conversation
// that arrived at asim+research@ is research's, and answering it as the general
// agent is the wrong instruction and the wrong tool scope. It is also the one
// place work is actually given away, so having more than one agent bought
// nothing exactly where it should have counted.
func TestHandingOverKeepsTheConversationsAgent(t *testing.T) {
	const who = "act-agentful"
	reader(t, who)

	th := thread.Open(who, "mail", "<agentful@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}
	thread.SetAgent(who, th.ID, "research")

	form := url.Values{"id": {th.ID}, "ask": {"deal with this"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	action(httptest.NewRecorder(), r, who)

	task := taskOn(t, who, th.ID)
	if task == nil {
		t.Fatal("no task was made")
	}
	if task.Agent != "research" {
		t.Errorf("the task was handed to %q, want %q — the conversation's own "+
			"agent did not travel with the work", task.Agent, "research")
	}
}

// And a conversation with no agent stays with the default, which is what
// almost every thread is.
func TestHandingOverWithoutAnAgentIsStillTheDefault(t *testing.T) {
	const who = "act-agentless"
	reader(t, who)

	th := thread.Open(who, "mail", "<agentless@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}

	form := url.Values{"id": {th.ID}, "ask": {"deal with this"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	action(httptest.NewRecorder(), r, who)

	task := taskOn(t, who, th.ID)
	if task == nil {
		t.Fatal("no task was made")
	}
	if task.Agent != "" {
		t.Errorf("Agent = %q, want empty so the default answers", task.Agent)
	}
}
