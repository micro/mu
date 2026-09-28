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
// Mail and the live chat share one history; original mail remains service-owned.
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
	target, err := consolidateCheckin(acc.ID, source)
	if err != nil || target == nil {
		app.NotFound(w, r, "Checkin not found")
		return
	}
	if err := thread.Flush(); err != nil {
		app.BadRequest(w, r, "Could not save checkin; please try again")
		return
	}
	markCheckinRead(acc.ID, target.ID)
	http.Redirect(w, r, Path(acc.ID, target.Agent)+"?session="+url.QueryEscape(target.ID), http.StatusSeeOther)
}

// consolidateCheckin retains mail identities and both sides of legacy split
// conversations. Old mail URLs and transport keys resolve to the live chat.
func consolidateCheckin(owner string, source *thread.Thread) (*thread.Thread, error) {
	if source.Client == thread.WebClient && strings.HasPrefix(source.Key, "checkin:") {
		old := thread.Get(owner, strings.TrimPrefix(source.Key, "checkin:"))
		if old == nil || old.ID == source.ID {
			return source, nil
		}
		source = old
	}
	messages := thread.Messages(owner, source.ID, 0)
	isCheckin := false
	copies := map[string]bool{}
	for _, m := range messages {
		if strings.Contains(m.To, "+checkin@") {
			isCheckin = true
		}
		copies["checkin-import:"+m.ID] = true
	}
	if !isCheckin {
		return nil, nil
	}
	target := thread.Open(owner, thread.WebClient, "checkin:"+source.ID)
	thread.Name(owner, target.ID, source.Subject)
	thread.SetAgent(owner, target.ID, source.Agent)
	err := thread.Merge(owner, source.ID, target.ID, copies, func(m *thread.Message) {
		if m.From == "agent@"+mail.ConfiguredDomain() {
			m.Role = thread.RoleAgent
		}
		m.Text = app.NormalizeAnswerMarkdown(html.UnescapeString(m.Text))
	})
	if err != nil {
		return nil, err
	}
	return target, nil
}

func markCheckinRead(owner, id string) {
	thread.MarkSeen(owner, id)
	for _, m := range thread.Messages(owner, id, 0) {
		if native := mail.FindMessageByMessageID(m.Ref); native != nil && native.ToID == owner && !native.Read {
			if err := mail.MarkAsRead(native.ID, owner); err != nil {
				app.Log("agent", "mark checkin read: %v", err)
			}
		}
	}
}

func consolidateCheckins() {
	for _, acc := range auth.AllAccounts() {
		for _, t := range thread.List(acc.ID, 0) {
			if t.Client != thread.WebClient || !strings.HasPrefix(t.Key, "checkin:") {
				continue
			}
			if _, err := consolidateCheckin(acc.ID, &t); err != nil {
				app.Log("agent", "consolidate checkin: %v", err)
			}
		}
	}
	if err := thread.Flush(); err != nil {
		app.Log("agent", "save consolidated checkins: %v", err)
	}
}
