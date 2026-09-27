package agent

import (
	"html"
	"mu/service/mail"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// CheckinHandler continues an owned scheduled check-in in the live web agent.
// The original mail stays available to email clients; imports are idempotent.
func CheckinHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if r.Method != http.MethodGet {
		app.MethodNotAllowed(w, r)
		return
	}
	source := thread.Get(acc.ID, r.URL.Query().Get("id"))
	if source == nil {
		app.NotFound(w, r, "Check-in not found")
		return
	}
	if source.Client == thread.WebClient && strings.HasPrefix(source.Key, "checkin:") {
		http.Redirect(w, r, Path(acc.ID, source.Agent)+"?session="+url.QueryEscape(source.ID), http.StatusSeeOther)
		return
	}
	messages := thread.Messages(acc.ID, source.ID, 100)
	isCheckin := false
	for _, m := range messages {
		if strings.Contains(m.To, "+checkin@") {
			isCheckin = true
			break
		}
	}
	if !isCheckin {
		app.NotFound(w, r, "Check-in not found")
		return
	}
	target := thread.Open(acc.ID, thread.WebClient, "checkin:"+source.ID)
	thread.Name(acc.ID, target.ID, "Daily check-in")
	thread.SetAgent(acc.ID, target.ID, source.Agent)
	for _, m := range messages {
		role := m.Role
		if m.From == "agent@"+mail.ConfiguredDomain() {
			role = thread.RoleAgent
		}
		thread.Add(thread.Message{Account: acc.ID, Thread: target.ID, Role: role, Text: app.NormalizeAnswerMarkdown(html.UnescapeString(m.Text)), At: m.At, Ref: "checkin-import:" + m.ID})
	}
	http.Redirect(w, r, Path(acc.ID, source.Agent)+"?session="+url.QueryEscape(target.ID), http.StatusSeeOther)
}
