package inbox

// Telling the agent what to do about the thing you are reading.
//
// The inbox is where the work arrives and where the agent already is, and until
// now those were two facts that never met: you could read a conversation here
// and you could talk to an agent on another page, and moving from one to the
// other meant retyping what the message said. The interesting move — the one
// worth copying — is a box on the message itself: "add that to my calendar",
// and it does, because the details are in the messages above and the tools are
// already there.
//
// It is not a reply. Nothing typed here is sent to anybody: the conversation is
// what arrived, and this is the owner standing over it giving an instruction.
// Keeping those apart is the whole of InboxPrompt — an agent that confuses them
// answers the sender about calendars.
//
// The instruction and the answer are recorded on the same conversation, because
// that is what they are about. A month later the thread reads as what arrived,
// what you asked for, and what was done — which is a better record than the mail
// alone, and it is the thing an agent is handed the next time.

import (
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/tasks"
)

// Act runs the agent on a conversation the reader is looking at, and is filled
// in by the server because this package may not import agent/ — see doc.go.
//
// Nil on a build with no agent, which is what hides the box: an instruction
// nothing can carry out is a control that does nothing.
var Act func(accountID, threadID, ask string) error

// askLimit bounds one instruction. Long enough for a sentence with a date in
// it, short enough that a pasted document is not a prompt.
const askLimit = 2000

// action handles the POST from the box under a conversation.
func action(w http.ResponseWriter, r *http.Request, accountID string) {
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	ask := strings.TrimSpace(r.FormValue("ask"))
	if len(ask) > askLimit {
		ask = ask[:askLimit]
	}

	back := "/inbox"
	if id != "" {
		back = "/inbox?id=" + url.QueryEscape(id)
	}

	switch {
	case Act == nil, id == "", ask == "":
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	// Whose conversation it is, before anything is charged or run. Scoped to
	// the reader, so somebody else's id is not a conversation.
	t := thread.Get(accountID, id)
	if t == nil {
		app.NotFound(w, r, "no conversation here with that id")
		return
	}

	// Handing it over rather than waiting for it. See hand.
	if r.FormValue("hand") != "" {
		if err := hand(accountID, t, ask); err != nil {
			app.Log("inbox", "handing a conversation to an agent failed: %v", err)
			http.Redirect(w, r, back+"&problem="+url.QueryEscape(err.Error()), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	if err := Act(accountID, id, ask); err != nil {
		app.Log("inbox", "acting on a conversation failed: %v", err)
		http.Redirect(w, r, back+"&problem="+url.QueryEscape("that one did not work. Try asking a different way."), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, back, http.StatusSeeOther)
}

// hand turns the conversation into work and gives it away.
//
// Two buttons on one box, and the difference is not how long it takes — it is
// whether you are waiting. Ask runs now and the answer is here when the page
// comes back; Hand over makes a task, and you can close the tab.
//
// No hook and no agent. tasks.Run announces that work was asked for and
// agent/work subscribes, so starting work needs nothing from agent/ — which is
// what inverting those three hooks bought. The import is the plain one this
// package's doc already argues for with service/mail: a service is neither
// internal/ nor a consumer of tools, so there is no cycle and no hook to
// justify.
//
// The thread goes on the task, so the answer comes back to the conversation
// somebody is actually reading rather than onto a page they have no reason to
// open. See tasks.Task.Thread.
//
// And the conversation goes in the detail. A task run starts cold — a title and
// a line, no history — which is exactly why work handed off in one sentence
// comes back worse than the same request in a conversation that already has the
// context. This carries what was said, so it does not.
func hand(accountID string, t *thread.Thread, ask string) error {
	title := ask
	if len(title) > handTitle {
		title = strings.TrimSpace(title[:handTitle]) + "…"
	}

	var detail strings.Builder
	if subject := strings.TrimSpace(t.Subject); subject != "" {
		detail.WriteString("This is about a conversation titled " + subject + ".\n\n")
	}
	msgs := thread.Messages(accountID, t.ID, handContext)
	if len(msgs) > 0 {
		detail.WriteString("What was said, oldest first:\n\n")
		for _, m := range msgs {
			who := "They"
			if m.Role == thread.RoleAgent {
				who = "The agent"
			} else if strings.TrimSpace(m.From) == "" || m.From == accountID {
				who = "The owner"
			}
			// Without the quoted tail. Six messages each carrying the two before
			// them is the same conversation three times over in one prompt, and
			// the agent is being handed all six anyway. See quoted.go.
			text, _ := unquoted(m.Text)
			detail.WriteString(who + ": " + strings.TrimSpace(text) + "\n\n")
		}
	}
	detail.WriteString("What they have asked for: " + ask)

	task, err := tasks.CreateOn(accountID, t.ID, title, detail.String(), tasks.Agent, time.Time{})
	if err != nil {
		return err
	}
	// Said on the conversation, because a task made silently is a task nobody
	// knows was made — and because inferring that somebody wanted work rather
	// than an answer is a claim about what they meant, which they should be
	// able to see and correct.
	agentSaid(accountID, t.ID, "Taking that on — I will answer here when it is done.")
	return tasks.Run(accountID, task.ID)
}

// handTitle bounds a task's title, and handContext how much of the conversation
// travels with it. Six messages is three exchanges, which is what the mail
// agent is reminded of for the same reason.
const (
	handTitle   = 70
	handContext = 6
)

// agentSaid records a line from the agent on a conversation, filled in by the
// server because this package may not import agent/.
//
// One line rather than the whole of agent.Answered: what is needed here is
// "say this on that thread", and the agent owns how anything it says is
// written down.
var agentSaid = func(accountID, threadID, text string) {}

// AgentSaid is how the server hands that over. See agentSaid.
func AgentSaid(f func(accountID, threadID, text string)) {
	if f != nil {
		agentSaid = f
	}
}

// askBox is the control itself: somewhere to type, and three things that are
// true of any message.
//
// The suggestions exist because an empty box on a page nobody has seen before
// is a question with no hint of what the answers look like.
//
// "Add this to my calendar" was one of them and has gone. It read as a claim
// about the message — the thing Gmail does when it has found a date — and it
// was on every conversation whether or not there was anything to put in a
// calendar, including a newsletter. A suggestion that is wrong about what you
// are looking at is worse than no suggestion, because the first one somebody
// tries teaches them what the box is for.
//
// What is left is true of anything: summarise it, tell me what to do, draft a
// reply. Making them depend on the content would mean reading the content,
// which is a model call to decide what to offer before anybody has asked for
// anything.
func askBox(r *http.Request, threadID, replyWho string) string {
	if Act == nil {
		return ""
	}
	canReply := replyWho != ""
	var b strings.Builder
	b.WriteString(`<form class="ib-ask" method="post" action="/inbox">`)
	b.WriteString(`<input type="hidden" name="id" value="` + html.EscapeString(threadID) + `">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">`)
	if problem := strings.TrimSpace(r.URL.Query().Get("problem")); problem != "" {
		b.WriteString(`<p class="ib-ask-problem">` + html.EscapeString(problem) + `</p>`)
	}
	b.WriteString(`<textarea name="ask" rows="2" maxlength="` + strconv.Itoa(askLimit) + `" ` +
		`placeholder="Tell the agent what to do about this"></textarea>`)
	// Pressed once, and it says so.
	//
	// This posts, runs a model for as long as a model takes, and redirects back
	// to a page that looks exactly like the one it left. Nothing moved, nothing
	// spun, and the button was still there to press — so the honest reading of
	// the screen was that the press had not registered. Somebody asked their
	// inbox to turn a sender down politely, pressed Ask twice on that reading,
	// and paid for two runs.
	//
	// Inline rather than in mu.js because the whole behaviour is three
	// assignments on one form and it belongs where the form is. The submit is
	// not cancelled — disabling a submit button in its own handler would stop
	// the POST — so the click goes through and the second one has nothing to
	// click.
	// Two buttons, and the difference is whether you are waiting.
	//
	// Ask runs now and the answer is here when the page comes back. Hand over
	// makes a task and you can close the tab — which is the move this page is
	// for, because the inbox is where you decide what happens to things rather
	// than where you sit through them.
	//
	// The disable-on-click is on both: the run takes tens of seconds with
	// nothing on the screen, and somebody who read that as a press that had not
	// registered pressed again and paid for two.
	press := `onclick="var f=this.form;setTimeout(function(){f.querySelectorAll('button').forEach(` +
		`function(b){b.disabled=true});this.textContent='Working…'}.bind(this),0)"`
	b.WriteString(`<div class="ib-ask-row"><button type="submit" ` + press + `>Ask</button>`)
	b.WriteString(`<button type="submit" name="hand" value="1" class="pill" ` + press +
		`>Hand over</button>`)
	for _, s := range []string{
		"Summarise this",
		"Draft a reply",
		"What should I do about this?",
	} {
		// Fills the box rather than submitting, so the suggestion is a starting
		// point somebody can change — which is what makes it a suggestion.
		b.WriteString(`<button type="button" class="pill" onclick="this.form.ask.value='` +
			html.EscapeString(s) + `';this.form.ask.focus()">` + html.EscapeString(s) + `</button>`)
	}
	// What the two buttons do, and what neither of them is.
	//
	// A box under a message looks like a reply, so it says it is not one. That
	// sentence used to be the whole caption — "This is not a reply" — on a page
	// with no reply button anywhere, which read as a statement that replying was
	// not possible. It is a distinction now rather than a refusal.
	note := `Ask and the answer comes back here. Hand over makes it a task and ` +
		`the agent answers here when it is done. It is not a reply — nothing ` +
		`typed here is sent to anybody.`
	if canReply {
		note = `Ask and the answer comes back here. Hand over makes it a task and ` +
			`the agent answers here when it is done. It is not a reply — use ` +
			`Reply above to answer ` + html.EscapeString(replyWho) + ` yourself.`
	}
	b.WriteString(`</div><p class="ib-ask-note">` + note + `</p>`)
	b.WriteString(`</form>`)
	return b.String()
}
