package account

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/inbox"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/sshaccess"
)

func handleTokenPage(w http.ResponseWriter, r *http.Request, accountID, sessionID string) {
	var b strings.Builder
	b.WriteString(Navigation("/account/clients") + `<div class="account-access">`)
	b.WriteString(clientList(r, accountID))
	kind := r.URL.Query().Get("add")
	if kind == "" && r.URL.Query().Get("access") != "" {
		kind = "api"
	}
	open := ""
	if kind != "" {
		open = " open"
	}
	b.WriteString(`<details id="add-client" class="disclosure"` + open + `><summary class="btn">Add client</summary><div class="page-col">`)
	switch kind {
	case "mail", "xmpp":
		protocol, label := "mail", "Mail"
		if kind == "xmpp" {
			protocol, label = "chat", "XMPP"
		}
		b.WriteString(`<h2>` + label + ` app</h2><p>Create an app password, then enter it in your app with the settings below.</p><form method="POST" action="/account/app-password" class="form">` + app.CSRFField(auth.CSRFToken(r)) + `<input type="hidden" name="client" value="` + protocol + `"><label class="field-label">App name<input name="name" maxlength="80" required placeholder="e.g. My phone"></label><button type="submit">Create app password</button></form>` + inbox.ClientSettings(accountID, protocol))
	case "api":
		b.WriteString(apiTokenForm(r))
	case "oauth":
		b.WriteString(oauthClientForm(r))
	default:
		b.WriteString(`<div class="form-actions"><a class="btn" href="/account/clients?add=mail#add-client">Mail</a><a class="btn" href="/account/clients?add=xmpp#add-client">XMPP</a><a class="btn" href="/account/clients?add=api#add-client">API</a><a class="btn" href="/account/clients?add=oauth#add-client">OAuth</a></div>`)
	}
	if kind == "mail" || kind == "xmpp" || kind == "api" || kind == "oauth" {
		b.WriteString(`<p><a href="/account/clients?add=choose#add-client">Change type</a></p>`)
	}
	b.WriteString(`</div></details><p><a href="/developers">Developers</a></p><details class="disclosure"><summary>SSH keys</summary>` + sshaccess.Card(r, accountID, "/account/clients", "SSH and SFTP", "Connect with your public key.", "ssh") + `</details></div>`)
	app.Respond(w, r, app.Response{Title: "Clients", Description: "Connected clients", HTML: b.String()})
}

func clientList(r *http.Request, accountID string) string {
	type row struct {
		id, name, kind, used, detail, action string
		created                              time.Time
	}
	var rows []row
	csrf := app.CSRFField(auth.CSRFToken(r))
	for _, t := range auth.ListTokens(accountID) {
		kind := "API"
		if appPassword(t) {
			kind = "Mail"
			if t.HasPermission("protocol:chat") {
				kind = "XMPP"
				if t.HasPermission("protocol:mail") {
					kind = "Mail / XMPP"
				}
			}
		}
		used := "Never"
		if !t.LastUsed.IsZero() {
			used = app.TimeAgo(t.LastUsed)
		}
		expiry := "Never"
		if !t.ExpiresAt.IsZero() {
			expiry = t.ExpiresAt.Format("2 Jan 2006")
		}
		detail := `<details><summary>Details</summary><p>` + tokenScope(t) + `</p><p>Created ` + t.Created.Format("2 Jan 2006") + `</p><p>Expires: ` + expiry + `</p></details>`
		action := `<form method="POST" action="/account/clients?id=` + html.EscapeString(t.ID) + `" class="form-action">` + csrf + `<input type="hidden" name="_method" value="DELETE"><button type="submit">Revoke</button></form>`
		rows = append(rows, row{t.ID, t.Name, kind, used, detail, action, t.Created})
	}
	for _, c := range auth.OAuthClientsFor(accountID) {
		detail := `<details><summary>Details</summary><p>Client ID: <code>` + html.EscapeString(c.ClientID) + `</code></p>`
		for _, u := range c.RedirectURIs {
			detail += `<p>Callback URL: <code>` + html.EscapeString(u) + `</code></p>`
		}
		detail += `</details>`
		action := `<form method="POST" action="/account/clients?delete_client=` + html.EscapeString(c.ClientID) + `" class="form-action">` + csrf + `<input type="hidden" name="_method" value="DELETE"><button type="submit">Delete</button></form>`
		rows = append(rows, row{c.ClientID, c.Name, "OAuth", "Not tracked", detail, action, c.CreatedAt})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].created.Equal(rows[j].created) {
			return rows[i].id < rows[j].id
		}
		return rows[i].created.After(rows[j].created)
	})
	if len(rows) == 0 {
		return `<p>No clients connected.</p>`
	}
	var b strings.Builder
	b.WriteString(`<table class="data-table stacked clients-table"><thead><tr><th>Name</th><th>Type</th><th>Last used</th><th></th><th></th></tr></thead><tbody>`)
	for _, x := range rows {
		fmt.Fprintf(&b, `<tr><td data-label="Name">%s</td><td data-label="Type">%s</td><td data-label="Last used">%s</td><td>%s</td><td>%s</td></tr>`, html.EscapeString(x.name), x.kind, html.EscapeString(x.used), x.detail, x.action)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}
