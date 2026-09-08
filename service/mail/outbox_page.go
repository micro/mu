package mail

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/userdb"
)

func outboxPage(w http.ResponseWriter, r *http.Request, owner string) {
	if r.Method == http.MethodPost {
		if !auth.StrictCSRF(r) {
			app.Forbidden(w, r, "Invalid CSRF token")
			return
		}
		if err := retryQueued(owner, r.FormValue("id")); err != nil {
			app.Respond(w, r, app.Response{Title: "Outbox", HTML: `<p>` + html.EscapeString(err.Error()) + `</p>`})
			return
		}
		http.Redirect(w, r, "/mail?view=outbox", http.StatusSeeOther)
		return
	}
	auth.SetCSRFCookie(w, r)
	rows, err := userdb.List("mail", owner, outboxCollection, "mine", nil, "", "", userdb.MaxListLimit)
	if err != nil {
		app.Respond(w, r, app.Response{Title: "Outbox", HTML: `<p>Could not load outgoing mail.</p>`})
		return
	}
	var b strings.Builder
	b.WriteString(`<div class="page-stack"><p>Outgoing mail retries automatically. Successful deliveries leave this list. A retry keeps the original message and skips recipients who already received it.</p>`)
	if len(rows) == 0 {
		b.WriteString(`<p>No pending deliveries.</p>`)
	}
	for _, rec := range rows {
		m, err := readQueued(&rec)
		if err != nil {
			b.WriteString(`<div class="card">Could not read a saved delivery.</div>`)
			continue
		}
		state := "Queued"
		if m.Attempts > 0 {
			state = "Retrying"
		}
		pending := rec.Data["pending"] == true
		if !pending {
			state = "Needs attention"
		}
		fmt.Fprintf(&b, `<div class="card page-stack"><strong>%s</strong><div>%s</div><div>%s · %d of %d recipients accepted</div>`,
			html.EscapeString(m.Subject), html.EscapeString(strings.Join(m.Recipients, ", ")), state, len(m.Delivered), len(m.Recipients))
		if m.LastError != "" {
			b.WriteString(`<p>` + html.EscapeString(m.LastError) + `</p>`)
		}
		if !pending {
			fmt.Fprintf(&b, `<form method="post" action="/mail?view=outbox" class="form-actions"><input type="hidden" name="_csrf" value="%s"><input type="hidden" name="id" value="%s"><button type="submit">Retry</button></form>`, html.EscapeString(auth.CSRFToken(r)), html.EscapeString(rec.ID))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	page := app.Page(app.PageOpts{Filters: `<div class="mail-tabs"><a href="/mail" class="mail-tab">Inbox</a><a href="/mail?view=sent" class="mail-tab">Sent</a><a href="/mail?view=outbox" class="mail-tab active">Outbox</a></div>`, Content: b.String()})
	app.Respond(w, r, app.Response{Title: "Outbox", HTML: page})
}

func retryQueued(owner, id string) error {
	outboxRunMu.Lock()
	defer outboxRunMu.Unlock()
	rec, err := userdb.Get("mail", owner, outboxCollection, id)
	if err != nil {
		return fmt.Errorf("no outgoing message here with that id")
	}
	if rec.Data["pending"] == true {
		return fmt.Errorf("this message is already queued")
	}
	m, err := readQueued(rec)
	if err != nil {
		return err
	}
	m.Created, m.Attempts, m.LastError = time.Now().UTC(), 0, ""
	if err := saveQueued(owner, id, m, true, time.Now()); err != nil {
		return err
	}
	select {
	case outboxWake <- struct{}{}:
	default:
	}
	return nil
}
