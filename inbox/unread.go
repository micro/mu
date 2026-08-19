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

// deleteButton removes a conversation and everything said on it.
//
// A mailbox you cannot delete from is a list that only grows. The record is
// somebody's own memory and nothing expires it, which is right — but that is a
// durability guarantee, not a reason to withhold the button.
//
// A confirm, because it is not undoable and the thing it takes is a
// conversation rather than a row.
func deleteButton(r *http.Request, id string) string {
	return `<form method="post" action="/inbox/delete" ` +
		`onsubmit="return confirm('Delete this conversation? What was said in it is gone.')">` +
		`<input type="hidden" name="id" value="` + html.EscapeString(id) + `">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<button class="pill pill-danger" type="submit">Delete</button></form>`
}

// DeleteHandler serves POST /inbox/delete.
func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodPost {
		Handler(w, r)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "that request did not carry a valid token")
		return
	}
	if id := strings.TrimSpace(r.FormValue("id")); id != "" {
		// Scoped to the reader by thread.Delete, so somebody else's id removes
		// nothing rather than removing theirs.
		thread.Delete(acc.ID, id)
	}
	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}

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
	return `<form method="post" action="/inbox/unread">` +
		`<input type="hidden" name="id" value="` + html.EscapeString(id) + `">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<button class="pill" type="submit">Mark unread</button></form>`
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
