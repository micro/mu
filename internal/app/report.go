package app

// Telling an operator that something here should not be.
//
// The one control a reader keeps over somebody else's item. It used to live in
// a handler with six others — save, unsave, hide, unhide, block, unblock — and
// those were the controls of a feed: somewhere strangers' content is put in
// front of you and you need a way to push it back. Mu has no feed, so they went.
//
// This is a different thing and it survives them. It is not about what the
// reporter sees; it is a message to whoever runs the instance, and it stays
// worth having while there is a blog and public threads on it.
//
// At /report rather than under /user, because the page that word named is gone
// and a URL that says "user" leading to a moderation queue would be the next
// reader's puzzle.

import (
	"net/http"
	"strings"

	"mu/internal/auth"
	"mu/internal/flag"
)

// ReportHandler serves /report.
func ReportHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "sign in to report something")
		return
	}
	q := r.URL.Query()
	kind := strings.TrimSpace(q.Get("type"))
	id := strings.TrimSpace(q.Get("id"))
	if kind == "" || id == "" {
		RespondError(w, http.StatusBadRequest, "missing type or id")
		return
	}
	if _, _, err := flag.Add(kind, id, sess.Account); err != nil {
		RespondError(w, http.StatusInternalServerError, "could not report that")
		return
	}
	RespondJSON(w, map[string]any{"status": "ok"})
}
