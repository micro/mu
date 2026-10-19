package mail

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

const junkURL = "/inbox?view=junk"
const senderSettingsURL = "/inbox?view=mail-settings"

func mailPreferencesSession(w http.ResponseWriter, r *http.Request) (*auth.Account, bool) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return nil, false
	}
	w.Header().Set("Cache-Control", "private, no-store")
	auth.SetCSRFCookie(w, r)
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		app.MethodNotAllowed(w, r)
		return nil, false
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil || !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid form")
			return nil, false
		}
		if err := auth.CheckPostRate(acc.ID); err != nil {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return nil, false
		}
	}
	return acc, true
}

func mailPreferencesNav(active string) string {
	return app.ViewNavigation("Inbox views", active, []app.ViewLink{
		{Key: "all", Label: "All", URL: "/inbox"}, {Key: "unread", Label: "Unread", URL: "/inbox?filter=unread"},
		{Key: "saved", Label: "Saved", URL: "/inbox?filter=saved"}, {Key: "sent", Label: "Sent", URL: "/inbox?filter=sent"},
		{Key: "junk", Label: "Junk", URL: junkURL}, {Key: "settings", Label: "Settings", URL: senderSettingsURL},
	}, false)
}

// SenderSettingsHandler manages delivery preferences for the signed-in mailbox.
func SenderSettingsHandler(w http.ResponseWriter, r *http.Request) {
	acc, ok := mailPreferencesSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		action := r.PostFormValue("action")
		if action != "allow" && action != "remove" {
			app.BadRequest(w, r, "Unknown action")
			return
		}
		if err := changeAllowedSender(acc.ID, r.PostFormValue("sender"), action == "remove"); err != nil {
			app.BadRequest(w, r, err.Error())
			return
		}
		http.Redirect(w, r, senderSettingsURL, http.StatusSeeOther)
		return
	}
	rules, err := allowedSenders(acc.ID)
	if err != nil {
		app.Error(w, r, 503, err.Error())
		return
	}
	csrf := app.CSRFField(auth.CSRFToken(r))
	var b strings.Builder
	b.WriteString(mailPreferencesNav("settings") + `<h2>Allowed senders</h2><p>Allow an email address or exact domain for your mailbox. Authenticated mail from these senders goes to Inbox. This does not give them access to your assistant.</p>`)
	b.WriteString(`<form method="POST" action="` + senderSettingsURL + `" class="form record-editor">` + csrf + `<input type="hidden" name="action" value="allow"><label for="allowed-sender">Email address or domain</label><input type="text" id="allowed-sender" name="sender" autocapitalize="none" spellcheck="false" placeholder="person@example.com or peerlist.io" maxlength="254" required><div class="form-actions"><button type="submit">Allow sender</button></div></form>`)
	if len(rules) == 0 {
		b.WriteString(`<p class="text-muted">No personal allowed senders yet.</p>`)
	}
	b.WriteString(`<div class="collection-list">`)
	for _, rule := range rules {
		display := strings.TrimPrefix(rule, "@")
		fmt.Fprintf(&b, `<div class="collection-item"><span>%s</span><form method="POST" action="%s" class="form-action">%s<input type="hidden" name="action" value="remove"><input type="hidden" name="sender" value="%s"><button class="btn-secondary" type="submit">Remove</button></form></div>`, html.EscapeString(display), senderSettingsURL, csrf, html.EscapeString(rule))
	}
	b.WriteString(`</div><p><a href="/inbox/imap">Mail client settings</a></p>`)
	app.Respond(w, r, app.Response{Title: "Inbox settings", HTML: b.String()})
}

func junkMessage(owner, id string) *Message {
	mutex.RLock()
	defer mutex.RUnlock()
	m := byID[id]
	if m == nil || m.ToID != owner || !m.Spam {
		return nil
	}
	cp := *m
	cp.SpamReasons = append([]string(nil), m.SpamReasons...)
	return &cp
}

func junkActions(r *http.Request, m *Message) string {
	var b strings.Builder
	b.WriteString(`<form method="POST" action="` + junkURL + `" class="form-actions">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="msg_id" value="` + html.EscapeString(m.ID) + `">`)
	for _, a := range []struct{ value, label string }{{"not_spam", "Move to inbox"}, {"allow_sender", "Always allow sender"}, {"allow_domain", "Always allow domain"}, {"delete_spam", "Delete"}} {
		b.WriteString(`<button type="submit" name="action" value="` + a.value + `" class="btn-secondary">` + a.label + `</button>`)
	}
	b.WriteString(`</form>`)
	return b.String()
}

// JunkHandler reuses the stored spam flag and the IMAP Junk folder. Releasing
// mail updates Inbox but never publishes the event that starts an agent.
func JunkHandler(w http.ResponseWriter, r *http.Request) {
	acc, ok := mailPreferencesSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPost {
		m := junkMessage(acc.ID, r.PostFormValue("msg_id"))
		if m == nil {
			app.NotFound(w, r, "Message not found")
			return
		}
		var err error
		switch r.PostFormValue("action") {
		case "allow_sender", "allow_domain":
			rule := m.FromID
			if r.PostFormValue("action") == "allow_domain" {
				_, domain, found := strings.Cut(rule, "@")
				if !found {
					app.BadRequest(w, r, "Sender has no domain")
					return
				}
				rule = "@" + domain
			}
			err = changeAllowedSender(acc.ID, rule, false)
			if err == nil {
				err = NotSpamMessage(acc.ID, m.ID)
			}
		case "not_spam":
			err = NotSpamMessage(acc.ID, m.ID)
		case "delete_spam":
			err = DeleteSpamMessage(acc.ID, m.ID)
		default:
			app.BadRequest(w, r, "Unknown action")
			return
		}
		if err != nil {
			app.Error(w, r, 503, "Could not update this message: "+err.Error())
			return
		}
		http.Redirect(w, r, junkURL, http.StatusSeeOther)
		return
	}
	var b strings.Builder
	b.WriteString(mailPreferencesNav("junk"))
	if id := r.URL.Query().Get("id"); id != "" {
		m := junkMessage(acc.ID, id)
		if m == nil {
			app.NotFound(w, r, "Message not found")
			return
		}
		b.WriteString(`<p><a href="` + junkURL + `">Back to Junk</a></p><p class="text-muted">` + html.EscapeString(m.FromID) + `</p><p class="text-sm text-muted">` + html.EscapeString(strings.Join(m.SpamReasons, "; ")) + `</p>`)
		b.WriteString(`<div class="thread-message-body">` + Rendered(m) + `</div>` + junkActions(r, m))
		app.Respond(w, r, app.Response{Title: decodeMIMEHeader(m.Subject), HTML: b.String()})
		return
	}
	mutex.RLock()
	var items []*Message
	for _, m := range messages {
		if m.ToID == acc.ID && m.Spam {
			cp := *m
			items = append(items, &cp)
		}
	}
	mutex.RUnlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if len(items) == 0 {
		b.WriteString(`<p class="text-muted">No junk mail.</p>`)
	}
	pager := app.Paginate(r, len(items), 25)
	b.WriteString(`<div class="collection-list">`)
	for _, m := range items[pager.From:pager.To] {
		subject := decodeMIMEHeader(m.Subject)
		if subject == "" {
			subject = "(no subject)"
		}
		fmt.Fprintf(&b, `<div class="record-card"><a href="%s&id=%s"><strong>%s</strong></a><div class="metadata-row"><span>%s</span><span>%s</span></div><p class="text-sm text-muted">%s</p>%s</div>`, junkURL, url.QueryEscape(m.ID), html.EscapeString(subject), html.EscapeString(m.FromID), app.TimeAgo(m.CreatedAt), html.EscapeString(strings.Join(m.SpamReasons, "; ")), junkActions(r, m))
	}
	b.WriteString(`</div>` + pager.Nav(junkURL))
	app.Respond(w, r, app.Response{Title: "Junk", Description: "Review unfamiliar or filtered mail. Messages here do not trigger your assistant.", HTML: b.String()})
}
