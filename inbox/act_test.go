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
	body := assignDialog(r, "dlg_x", &thread.Thread{ID: "x"}, "")

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
	reader(t, "act_me")
	theirs := thread.Open("act_somebody_else", "mail", "<x@example.com>")
	if theirs == nil {
		t.Fatal("no conversation")
	}

	form := url.Values{"id": {theirs.ID}, "ask": {"summarise this"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	action(w, r, "act_me")

	if w.Code != 404 {
		t.Errorf("answered %d", w.Code)
	}
	if taskOn(t, "act_me", theirs.ID) != nil || taskOn(t, "act_somebody_else", theirs.ID) != nil {
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
	reader(t, "act_empty")
	mine := thread.Open("act_empty", "mail", "<y@example.com>")
	form := url.Values{"id": {mine.ID}, "ask": {"   "}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	action(w, r, "act_empty")

	if taskOn(t, "act_empty", mine.ID) != nil {
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
	who := fmt.Sprintf("act_rec_%d", time.Now().UnixNano()%100000000)
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
	const who = "inbox_hand"
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

	if err := hand(who, th, "Pull the numbers and summarise them", ""); err != nil {
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
	const who = "act_agentful"
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
	const who = "act_agentless"
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

// Picking an agent in the dialog sends the work to that one.
//
// Before the picker there was no question asked: the conversation's own agent
// was used, and that is only ever set once an agent has answered on the thread
// — which for mail to a bare address is never. So an account with four agents
// handed everything to the default one and nothing on the page said so.
func TestThePickerDecidesWhoGetsTheWork(t *testing.T) {
	const who = "act_picker"
	reader(t, who)
	Agents = func(owner string) []Agent {
		return []Agent{{ID: "research", Name: "Research", Tag: "research"},
			{ID: "money", Name: "Money", Tag: "money"}}
	}
	t.Cleanup(func() { Agents = nil })

	th := thread.Open(who, "mail", "<picker@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}

	form := url.Values{"id": {th.ID}, "ask": {"deal with this"}, "agent": {"money"}}
	r := httptest.NewRequest("POST", "/inbox", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	action(httptest.NewRecorder(), r, who)

	task := taskOn(t, who, th.ID)
	if task == nil {
		t.Fatal("no task was made")
	}
	if task.Agent != "money" {
		t.Errorf("the work went to %q, want money — the picker was ignored", task.Agent)
	}
}

// An agent id that is not one of yours is not an agent.
//
// The field is posted by whoever submits the form. Nothing downstream could
// reach somebody else's agent with it — AskAs is account-scoped — but a task
// should not be stored carrying a value that means nothing, and falling back
// to the default is what agent/work does with an unknown name anyway.
func TestAForgedAgentFallsBackToTheDefault(t *testing.T) {
	const who = "act_forged"
	reader(t, who)
	Agents = func(owner string) []Agent {
		return []Agent{{ID: "research", Name: "Research", Tag: "research"}}
	}
	t.Cleanup(func() { Agents = nil })

	th := thread.Open(who, "mail", "<forged@example.com>")
	if th == nil {
		t.Fatal("no conversation")
	}

	form := url.Values{"id": {th.ID}, "ask": {"deal with this"},
		"agent": {"somebody_elses_agent"}}
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

// The dialog offers a choice only when there is one to make.
func TestThePickerIsOnlyDrawnWhenThereAreAgents(t *testing.T) {
	r := httptest.NewRequest("GET", "/inbox?id=x", nil)
	th := &thread.Thread{ID: "x"}

	Agents = nil
	if got := assignDialog(r, "nobody", th, ""); strings.Contains(got, `name="agent"`) {
		t.Error("a picker with nothing in it is furniture that teaches nothing")
	}

	Agents = func(owner string) []Agent {
		return []Agent{{ID: "research", Name: "Research", Tag: "research"}}
	}
	t.Cleanup(func() { Agents = nil })
	got := assignDialog(r, "somebody", th, "")
	if !strings.Contains(got, `name="agent"`) {
		t.Error("no way to choose which agent gets it")
	}
	if !strings.Contains(got, `>Research</option>`) {
		t.Errorf("the roster is not offered:\n%s", got)
	}
}
