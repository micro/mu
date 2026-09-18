package account

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/sshaccess"
)

func handleTokenPage(w http.ResponseWriter, r *http.Request, accountID, sessionID string) {
	var b strings.Builder
	b.WriteString(Navigation("/account/tokens") + `<div class="account-access">`)
	b.WriteString(tokenList(r, accountID, false))

	b.WriteString(apiTokenForm(r, accountID))
	b.WriteString(`<h2>OAuth clients</h2>` + tokenList(r, accountID, true) + oauthClientForm(r))
	b.WriteString(sshaccess.Card(r, accountID, "/account/tokens", "SSH keys", "", "ssh") + "</div>")
	app.Respond(w, r, app.Response{Title: "Tokens", Description: "Account tokens", HTML: b.String()})

}

func tokenList(r *http.Request, accountID string, oauth bool) string {
	type row struct {
		id, name, kind, used, detail, action string
		created                              time.Time
	}
	var rows []row
	csrf := app.CSRFField(auth.CSRFToken(r))
	for _, t := range auth.ListTokens(accountID) {
		if oauth {
			continue
		}
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
		action := `<form method="POST" action="/account/tokens?id=` + html.EscapeString(t.ID) + `" class="form-action" data-confirm="` + html.EscapeString("Delete token \""+t.Name+"\"? Apps using it will lose access.") + `">` + csrf + `<input type="hidden" name="_method" value="DELETE"><button type="submit">Delete</button></form>`
		rows = append(rows, row{t.ID, t.Name, kind, used, detail, action, t.Created})
	}
	for _, c := range auth.OAuthClientsFor(accountID) {
		if !oauth {
			continue
		}
		detail := `<details><summary>Details</summary><p>Client ID: <code>` + html.EscapeString(c.ClientID) + `</code></p>`
		for _, u := range c.RedirectURIs {
			detail += `<p>Callback URL: <code>` + html.EscapeString(u) + `</code></p>`
		}
		detail += `</details>`
		action := `<form method="POST" action="/account/tokens?delete_client=` + html.EscapeString(c.ClientID) + `" class="form-action" data-confirm="` + html.EscapeString("Delete OAuth client \""+c.Name+"\"? Apps using it will lose access.") + `">` + csrf + `<input type="hidden" name="_method" value="DELETE"><button type="submit">Delete</button></form>`
		rows = append(rows, row{c.ClientID, c.Name, "OAuth", "Not tracked", detail, action, c.CreatedAt})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].created.Equal(rows[j].created) {
			return rows[i].id < rows[j].id
		}
		return rows[i].created.After(rows[j].created)
	})
	if len(rows) == 0 {
		if oauth {
			return `<p>No OAuth clients.</p>`
		}
		return `<p>No tokens.</p>`
	}
	var b strings.Builder
	b.WriteString(`<table class="data-table stacked clients-table"><thead><tr><th>Name</th><th>Type</th><th>Last used</th><th></th><th></th></tr></thead><tbody>`)
	for _, x := range rows {
		fmt.Fprintf(&b, `<tr><td data-label="Name">%s</td><td data-label="Type">%s</td><td data-label="Last used">%s</td><td>%s</td><td>%s</td></tr>`, html.EscapeString(x.name), x.kind, html.EscapeString(x.used), x.detail, x.action)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}
