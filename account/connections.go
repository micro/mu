package account

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
)

// Navigation keeps account destinations the same on settings and credential pages.
func Navigation(active string) string {
	var b strings.Builder
	b.WriteString(`<nav class="view-switch" aria-label="Account settings">`)
	for _, tab := range []struct{ path, label string }{{"/account", "Settings"}, {"/account/billing", "Billing"}} {
		current := ""
		if active == tab.path {
			current = ` aria-current="page"`
		}
		b.WriteString(`<a href="` + tab.path + `"` + current + `>` + tab.label + `</a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

func appPassword(t *auth.Token) bool {
	protocol := false
	for _, p := range t.Permissions {
		switch p {
		case "read", "write":
		case "protocol:mail", "protocol:chat":
			protocol = true
		default:
			return false
		}
	}
	return protocol
}

// AppPasswordHandler deliberately cannot create developer credentials.
func AppPasswordHandler(w http.ResponseWriter, r *http.Request) {
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if sess.Type != "account" || r.Method != http.MethodPost || !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Reopen App passwords and try again.")
		return
	}
	if err = r.ParseForm(); err != nil {
		app.BadRequest(w, r, "Invalid form")
		return
	}
	if id := r.FormValue("disconnect"); id != "" {
		token, err := auth.TokenByID(id)
		if err != nil || token.Account != acc.ID || !appPassword(token) {
			app.Forbidden(w, r, "Connection not found.")
			return
		}
		if err = auth.DeleteToken(id, acc.ID); err != nil {
			app.ServerError(w, r, "Could not disconnect this app.")
			return
		}
		http.Redirect(w, r, "/account/connections", http.StatusSeeOther)
		return
	}
	if err = auth.CheckCredentialAccess(acc.ID); err != nil {
		app.Forbidden(w, r, err.Error())
		return
	}
	if err = auth.CheckPostRate(acc.ID); err != nil {
		app.TooManyRequests(w, r, err.Error())
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	kind := r.FormValue("client")
	if name == "" || len(name) > 80 || (kind != "mail" && kind != "chat") {
		app.BadRequest(w, r, "Enter an app name and choose Mail or XMPP.")
		return
	}
	_, raw, err := auth.CreateToken(acc.ID, name, []string{"read", "write", "protocol:" + kind}, time.Time{})
	if err != nil {
		app.ServerError(w, r, "Could not create the app password: "+err.Error())
		return
	}
	body := Navigation("/account") + `<div class="page-col"><h2>Connect ` + html.EscapeString(name) + `</h2><p>Copy this app password into your app. It is shown only once and works until you disconnect it.</p><label class="field-label">App password<input id="app-password" readonly autocomplete="off" value="` + html.EscapeString(raw) + `"></label><div class="form-actions"><button type="button" data-copy-password>Copy password</button><a href="/account/connections">Done</a></div><p data-copy-status role="status"></p>` + inbox.ClientSettings(acc.ID) + `</div>`
	app.Respond(w, r, app.Response{Title: "App password", HTML: body})
}

func connectionApps(r *http.Request, accountID string) string {
	csrf := html.EscapeString(auth.CSRFToken(r))
	var b strings.Builder
	b.WriteString(`<section class="section-stack"><h2>App passwords</h2><p>Read and send your Micro mail in a mail app, or chat through an XMPP app. Create a password for each app, then enter it with the server settings below. Revoking it only disconnects that app.</p><form method="POST" action="/account/app-password" class="form"><input type="hidden" name="_csrf" value="` + csrf + `"><label class="field-label">App name<input name="name" maxlength="80" placeholder="e.g. Mail on my phone" required></label><div class="form-actions"><button name="client" value="mail">Connect a mail app</button><button name="client" value="chat">Connect an XMPP app</button></div></form><div class="collection-list">`)
	count := 0
	for _, t := range auth.ListTokens(accountID) {
		if !appPassword(t) {
			continue
		}
		count++
		label := "Mail"
		if t.HasPermission("protocol:chat") {
			label = "XMPP"
			if t.HasPermission("protocol:mail") {
				label = "Mail and XMPP"
			}
		}
		last := "Not used yet"
		if !t.LastUsed.IsZero() {
			last = "Last used " + t.LastUsed.Format("2 Jan 2006")
		}
		expiry := ""
		if !t.ExpiresAt.IsZero() {
			expiry = " · expires " + t.ExpiresAt.Format("2 Jan 2006")
		}
		fmt.Fprintf(&b, `<div class="record-card"><strong>%s</strong><p class="metadata-row">%s · %s%s</p><form method="POST" action="/account/app-password" class="form-actions"><input type="hidden" name="_csrf" value="%s"><button name="disconnect" value="%s">Disconnect</button></form></div>`, html.EscapeString(t.Name), label, last, expiry, csrf, html.EscapeString(t.ID))
	}
	if count == 0 {
		b.WriteString(`<p class="text-muted">No apps connected yet.</p>`)
	}
	b.WriteString(`</div><details class="disclosure"><summary>Server settings</summary>` + inbox.ClientSettings(accountID) + `</details></section>`)
	return b.String()
}
