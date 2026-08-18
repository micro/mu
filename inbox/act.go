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

// askBox is the control itself: somewhere to type, and four things people
// actually ask for.
//
// The suggestions are there because an empty box on a page nobody has seen
// before is a question with no hint of what the answers look like — and the
// gap between "there is an agent here" and "I can tell it to put this in my
// calendar" is exactly one example.
func askBox(r *http.Request, threadID string) string {
	if Act == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<form class="ib-ask" method="post" action="/inbox">`)
	b.WriteString(`<input type="hidden" name="id" value="` + html.EscapeString(threadID) + `">`)
	b.WriteString(`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">`)
	if problem := strings.TrimSpace(r.URL.Query().Get("problem")); problem != "" {
		b.WriteString(`<p class="ib-ask-problem">` + html.EscapeString(problem) + `</p>`)
	}
	b.WriteString(`<textarea name="ask" rows="2" maxlength="` + strconv.Itoa(askLimit) + `" ` +
		`placeholder="Tell the agent what to do about this"></textarea>`)
	b.WriteString(`<div class="ib-ask-row"><button type="submit">Ask</button>`)
	for _, s := range []string{
		"Add this to my calendar",
		"Summarise this",
		"Remind me about this",
		"What should I do about this?",
	} {
		// Fills the box rather than submitting, so the suggestion is a starting
		// point somebody can change — which is what makes it a suggestion.
		b.WriteString(`<button type="button" class="ib-chip" onclick="this.form.ask.value='` +
			html.EscapeString(s) + `';this.form.ask.focus()">` + html.EscapeString(s) + `</button>`)
	}
	b.WriteString(`</div><p class="ib-ask-note">This is not a reply — nothing here is sent to ` +
		`anybody. What you ask and what it does are kept on this conversation.</p>`)
	b.WriteString(`</form>`)
	return b.String()
}

const askCSS = `<style>
.ib-ask{margin-top:22px;padding-top:16px;border-top:1px solid #eee}
.ib-ask textarea{width:100%;box-sizing:border-box;font:inherit;font-size:14px;padding:9px 11px;border:1px solid #e2e2e2;border-radius:8px;resize:vertical}
.ib-ask textarea:focus{outline:none;border-color:#bbb}
.ib-ask-row{display:flex;flex-wrap:wrap;gap:6px;margin-top:8px;align-items:center}
.ib-ask button[type=submit]{font:inherit;font-size:13px;padding:6px 16px;border:1px solid #111;background:#111;color:#fff;border-radius:999px;cursor:pointer}
.ib-chip{font:inherit;font-size:12px;padding:4px 11px;border:1px solid #eee;background:none;color:#666;border-radius:999px;cursor:pointer}
.ib-chip:hover{border-color:#ddd;color:#111}
.ib-ask-note{font-size:12px;color:#aaa;margin:9px 0 0}
.ib-ask-problem{font-size:13px;color:#b00;margin:0 0 9px}
@media (max-width:640px){
  .ib-chip{font-size:11px}
}
</style>`
