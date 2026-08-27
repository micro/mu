package inbox

// What is waiting to be let in.
//
// A held conversation is one that arrived from somebody this account has never
// heard of. It is in the record and it is not in the list — that is the whole
// point of the state — so without somewhere to show it, holding a message and
// dropping it would look identical from here.
//
// Above the mailbox rather than below it, and only when there is something in
// it. It is the one part of this page that is asking a question.

import (
	"html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/sms"
)

// heldShown is how many waiting conversations are drawn. More than this and it
// is not a question any more, it is a mailbox of its own — and that is the
// point at which the number matters more than the messages.
const heldShown = 10

// waiting renders the held arrivals, or nothing.
func waiting(r *http.Request, accountID string) string {
	held := thread.HeldFor(accountID, heldShown+1)
	if len(held) == 0 {
		return ""
	}
	more := 0
	if len(held) > heldShown {
		more, held = len(held)-heldShown, held[:heldShown]
	}

	var b strings.Builder
	b.WriteString(`<div class="ib-held"><div class="ib-held-head">` +
		`<strong>Waiting to be let in</strong>` +
		`<span class="ib-held-note">From people nobody here has written to. ` +
		`Nothing has acted on these.</span></div>`)

	for _, t := range held {
		b.WriteString(heldRow(r, accountID, t))
	}
	if more > 0 {
		b.WriteString(`<p class="ib-held-note">And ` + itoa(more) + ` more.</p>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// heldRow is one waiting conversation: who, what they said, and the two
// answers.
//
// The message in full rather than a preview. A held conversation is one line
// long — it is the opening message from a stranger — and the decision being
// asked for is about that line, so making somebody open it to read it and come
// back is asking them to do the work twice.
func heldRow(r *http.Request, accountID string, t thread.Thread) string {
	who := strings.TrimSpace(t.Key)
	if who == "" {
		who = "Somebody"
	}
	said := ""
	if msgs := thread.Messages(accountID, t.ID, 1); len(msgs) > 0 {
		said = trimTo(strings.TrimSpace(msgs[0].Text), 300)
	}

	return `<div class="ib-held-row">` +
		`<div class="ib-held-who">` +
		`<a href="/inbox?id=` + url.QueryEscape(t.ID) + `">` + html.EscapeString(who) + `</a>` +
		app.Pill(app.ClientName(t.Client)) +
		`<span class="ib-held-when">` + html.EscapeString(app.TimeAgo(t.Updated)) + `</span></div>` +
		`<div class="ib-held-said">` + html.EscapeString(said) + `</div>` +
		heldActions(r, t) +
		`</div>`
}

// heldActions is let in, or block.
//
// Two, not three. "Leave it" is the state it is already in and needs no button;
// offering one would suggest that doing nothing is a decision somebody has to
// make, and the reason this list is short is that most of it can be ignored.
func heldActions(r *http.Request, t thread.Thread) string {
	csrf := `<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `">`

	// Block is only offered where blocking means something. It stops this
	// instance texting the number ever again, which is service/sms's own STOP
	// list — there is no equivalent for a channel that has not grown one, and a
	// button that quietly does nothing is worse than no button.
	block := ""
	if t.Client == thread.SMSClient && strings.TrimSpace(t.Key) != "" {
		block = `<form method="post" action="/inbox/held" class="d-inline">` + csrf +
			`<input type="hidden" name="do" value="block">` +
			`<button class="pill pill-danger" type="submit">Block ` +
			html.EscapeString(trimTo(t.Key, 20)) + `</button></form>`
	}

	return `<div class="ib-held-acts">` +
		`<form method="post" action="/inbox/held" class="d-inline">` + csrf +
		`<input type="hidden" name="do" value="let">` +
		`<button class="pill" type="submit">Let in</button></form>` +
		block + `</div>`
}

// HeldHandler serves POST /inbox/held: let one in, or block the sender.
func HeldHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
		return
	}
	// Scoped to the reader: thread.Get returns nothing for somebody else's id,
	// so neither branch below can act on a conversation that is not theirs.
	t := thread.Get(acc.ID, id)
	if t == nil {
		app.NotFound(w, r, "no conversation here with that id")
		return
	}

	switch r.FormValue("do") {
	case "block":
		// The number stops being able to reach this instance at all, which is
		// the same list STOP writes to — one way to be left alone rather than
		// two that disagree. The conversation stays held rather than being
		// deleted: what somebody said is evidence, and throwing it away is a
		// separate decision with its own button.
		if t.Client == thread.SMSClient {
			if key := strings.TrimSpace(t.Key); key != "" {
				// OptOut is the same list STOP writes to, which is the point:
				// one way to be left alone rather than two that disagree.
				sms.OptOut(key)
			}
		}
	case "let":
		thread.Release(acc.ID, id)
	}
	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}
