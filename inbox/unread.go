package inbox

// Marking a conversation unread again.
//
// The half a mailbox is unusable without. Opening something is reading it,
// which is right and is also how a thing you meant to come back to disappears
// into the pile — so there has to be a way to put it back. Every mail client
// has this and everybody uses it.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// unreadButton is the control on an open conversation.
//
// Shown only on one that was unread when you opened it, because that is when
// somebody wants it: marking unread something you had already dealt with is a
// thing nobody does, and a control offered always is one more piece of
// furniture on every page.
func unreadButton(r *http.Request, id string, wasUnread bool) string {
	if !wasUnread {
		return ""
	}
	return `<form class="ib-markform" method="post" action="/inbox/unread" style="display:inline">` +
		`<input type="hidden" name="id" value="` + html.EscapeString(id) + `">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<button class="ib-mark" type="submit">Mark unread</button></form>`
}

// UnreadHandler serves POST /inbox/unread.
//
// A path of its own rather than another branch on /inbox, because /inbox's own
// POST is the instruction box and the two are not variations of one another —
// one runs the agent and costs credits, this one sets a timestamp.
func UnreadHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	// A GET here is a mailbox called "unread" — unlikely, and free to allow:
	// box names come from agent names, and reserving a word would be a rule
	// somebody has to know. The exact route wins over /inbox/ for the POST,
	// which is all this needs.
	if r.Method != http.MethodPost {
		Handler(w, r)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id != "" {
		thread.MarkUnread(acc.ID, id)
	}
	// Back to the list rather than to the conversation: you have just said you
	// have not dealt with this, and returning you to it would mark it read
	// again on the next render.
	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}
