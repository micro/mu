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
	"mu/internal/origin"
	"mu/internal/sshaccess"
)

func handleTokenPage(w http.ResponseWriter, r *http.Request, accountID, sessionID string) {
	var b strings.Builder
	b.WriteString(Navigation("/account/tokens") + `<div class="account-access">`)
	b.WriteString(`<p>Tokens let apps and programs access your Micro account without using your password. Create a separate token for each app. <a href="/account/clients">Client setup</a>.</p>`)
	b.WriteString(tokenList(r, accountID, false))

	b.WriteString(apiTokenForm(r, accountID))
	b.WriteString(`<h2 id="oauth">OAuth clients</h2><p>Register an app you are building so people can sign in and grant it access to Micro. For your own scripts, use an API token above.</p><p>Use the authorization-code flow with PKCE (S256): send users to <code>/oauth/authorize</code>, then exchange the returned code at <code>/oauth/token</code>. Register your app’s exact callback URL below. Configuration is available at <code>/.well-known/oauth-authorization-server</code>.</p>` + tokenList(r, accountID, true) + oauthClientForm(r))
	b.WriteString(sshaccess.Card(r, accountID, "/account/tokens", "SSH keys", "Use a public SSH key to access Micro’s shell and files from a terminal or SFTP client. Add the contents of your .pub file; keep the private key on your device.", "ssh") + "</div>")
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

// ClientsHandler explains how other apps access this account. Credentials remain in Tokens.
func ClientsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || app.WantsJSON(r) || r.URL.RawQuery != "" {
		TokenHandler(w, r)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	base := html.EscapeString(strings.TrimRight(origin.URL(r), "/"))
	body := Navigation("/account/clients") + `<p>Use your Micro account from a mail app, chat app or your own code. Tokens act as passwords for these clients.</p>
 <h2>Mail</h2><p>Read and send Micro mail in your own mail app. This opens your Micro mailbox; it does not import another email account.</p><p><a href="/account/tokens?add=mail#create-token-form">Create mail token</a> and use it as the app’s password.</p>` + inbox.ClientSettings(acc.ID, "mail") +
		`<h2>Chat (XMPP)</h2><p>Send and receive messages using an XMPP chat app.</p><p><a href="/account/tokens?add=xmpp#create-token-form">Create chat token</a> and use it as the app’s password.</p>` + inbox.ClientSettings(acc.ID, "chat") +
		`<h2>Assistant API</h2><p>Call your assistant from a script using your account’s allowance and balance. <a href="/account/tokens?add=api#create-token-form">Create an API token</a> with Agents and Allow actions selected. Set it as MICRO_TOKEN, then run:</p><pre>curl '` + base + `/agent' \
  -H "Authorization: Bearer $MICRO_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Help me plan my week"}'</pre><p>The reply is in <code>text</code>. Include the returned <code>thread</code> as <code>thread</code> in your next request to continue. Keep tokens out of browser code and source control.</p>
 <h2>Service tools (MCP)</h2><p>Connect your own agent to Micro’s services using <code>` + base + `/mcp</code> and a <a href="/account/tokens?access=services#create-token-form">Services token</a>. This gives your client tools; it does not talk to your Micro assistant. <a href="/tools">Tool reference</a>.</p>
 <h2>OAuth and SSH</h2><p>For apps you build, register an OAuth client. For terminal and file access, add an SSH public key. Setup and credentials are on <a href="/account/tokens#oauth">Tokens</a>.</p>`
	app.Respond(w, r, app.Response{Title: "Clients", HTML: body})
}
