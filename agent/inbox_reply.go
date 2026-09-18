package agent

import (
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// inboxReply is the form fallback for the same conversation used by /agent.
// JavaScript uses the normal streaming endpoint; neither path assigns a task.
func inboxReply(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Invalid message")
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid form token")
		return
	}
	if !acc.Admin && !acc.Approved && !acc.EmailVerified {
		app.Forbidden(w, r, "Verify your email or ask the operator to approve your account")
		return
	}
	t := thread.Get(acc.ID, r.PostFormValue("id"))
	if t == nil || t.Client != thread.WebClient {
		app.NotFound(w, r, "Conversation not found")
		return
	}
	text := strings.TrimSpace(r.PostFormValue("ask"))
	if text == "" || len([]rune(text)) > 8000 {
		app.BadRequest(w, r, "Write a message of up to 8,000 characters")
		return
	}
	_, err = Ask(AskRequest{Account: acc.ID, Client: thread.WebClient, On: t.ID, Agent: t.Agent, Text: text})
	if err != nil {
		app.Log("agent", "inbox reply failed: %v", err)
		app.Respond(w, r, app.Response{Title: "Reply interrupted", HTML: app.Problem("The reply could not be completed. Check the conversation before sending again.") + `<a href="/inbox?id=` + url.QueryEscape(t.ID) + `">Back to conversation</a>`})
		return
	}
	http.Redirect(w, r, "/inbox?id="+url.QueryEscape(t.ID), http.StatusSeeOther)
}
