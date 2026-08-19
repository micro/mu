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

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/thread"
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
	if thread.Get(accountID, id) == nil {
		app.NotFound(w, r, "no conversation here with that id")
		return
	}

	// Charged like any other agent run, checked before the model is asked so a
	// run that cannot be paid for does not spend one first.
	if ok, _, _, err := quota.CheckQuota(accountID, quota.OpAgentQuery); err != nil || !ok {
		reason := "there are not enough credits for that"
		if err != nil {
			reason = err.Error()
		}
		http.Redirect(w, r, back+"&problem="+url.QueryEscape(reason), http.StatusSeeOther)
		return
	}

	if err := Act(accountID, id, ask); err != nil {
		app.Log("inbox", "acting on a conversation failed: %v", err)
		http.Redirect(w, r, back+"&problem="+url.QueryEscape("that one did not work. Try asking a different way."), http.StatusSeeOther)
		return
	}
	quota.ConsumeQuota(accountID, quota.OpAgentQuery) //nolint:errcheck

	http.Redirect(w, r, back, http.StatusSeeOther)
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
	b.WriteString(`<div class="ib-ask-row"><button type="submit" ` +
		`onclick="var f=this.form;setTimeout(function(){f.querySelectorAll('button').forEach(` +
		`function(b){b.disabled=true});f.querySelector('button[type=submit]').textContent='Working…'},0)"` +
		`>Ask</button>`)
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
	// What this box is not, and — where there is one — where the other thing is.
	//
	// "This is not a reply" was the whole caption, and on a page with no reply
	// button anywhere it read as a statement that replying was not possible. It
	// is a distinction now rather than a refusal.
	note := `This is not a reply — nothing here is sent to anybody. What you ask ` +
		`and what it does are kept on this conversation.`
	if canReply {
		note = `Not a reply — nothing typed here is sent. Use Reply above to answer ` +
			`` + html.EscapeString(replyWho) + ` yourself, or ask the agent to draft it.`
	}
	b.WriteString(`</div><p class="ib-ask-note">` + note + `</p>`)
	b.WriteString(`</form>`)
	return b.String()
}
