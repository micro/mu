package account

import (
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
	if active == "/account" {
		return ""
	}
	return `<p><a href="/account">Back to Account</a></p>`
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
		http.Redirect(w, r, "/account/clients", http.StatusSeeOther)
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
	body := Navigation("/account/clients") + `<div class="page-col"><h2>Connect ` + html.EscapeString(name) + `</h2><p>Copy this app password into your app. It is shown only once and works until you disconnect it.</p><label class="field-label">App password<input id="app-password" readonly autocomplete="off" value="` + html.EscapeString(raw) + `"></label><div class="form-actions"><button type="button" data-copy-password>Copy password</button><a href="/account/clients">Done</a></div><p data-copy-status role="status"></p>` + inbox.ClientSettings(acc.ID, kind) + `</div>`
	app.Respond(w, r, app.Response{Title: "App password", HTML: body})
}
