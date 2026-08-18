package admin

// Every OAuth client on the instance, and whose it is.
//
// /oauth/register is public and anonymous — that is what dynamic client
// registration is — so anything that speaks MCP can register itself by
// connecting, and the registry grows for the life of the instance with records
// belonging to nobody. There was no way to see them except on /token, which
// showed all of them to every signed-in person as though they were theirs.
//
// They are off that page now, which leaves them reachable from here and nowhere
// else. An operator is the only person for whom "every client on the instance"
// is a sensible thing to be looking at.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// OAuthHandler serves /admin/oauth.
func OAuthHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	if r.Method == "POST" {
		r.ParseForm() //nolint:errcheck
		if id := strings.TrimSpace(r.FormValue("client_id")); id != "" {
			auth.ForceDeleteOAuthClient(id)
			app.Log("admin", "removed oauth client %s", id)
		}
		http.Redirect(w, r, "/admin/oauth", http.StatusSeeOther)
		return
	}

	clients := auth.AllOAuthClients()

	var b strings.Builder
	b.WriteString(`<p><a href="/admin">← Admin</a></p><h2>OAuth Clients</h2>`)
	b.WriteString(`<p class="text-muted text-sm">Anything that speaks MCP can register ` +
		`itself at <code>/oauth/register</code> without signing in — that is what the ` +
		`protocol asks for — so most of these belong to nobody. A client registered from ` +
		`somebody's <a href="/token">token page</a> carries their account, and only they ` +
		`can remove it. Removing one here stops that client from completing a sign-in; ` +
		`it will register again next time it connects.</p>`)

	suffix := "s"
	if len(clients) == 1 {
		suffix = ""
	}
	fmt.Fprintf(&b, `<p class="text-muted text-sm">%d client%s.</p>`, len(clients), suffix)
	b.WriteString(`<table class="admin-table"><thead><tr><th>Name</th><th>Client ID</th>` +
		`<th>Owner</th><th class="created-col">Created</th><th class="center"></th></tr></thead><tbody>`)

	for _, c := range clients {
		owner := `<span class="text-muted">self-registered</span>`
		if c.Account != "" {
			who := c.Account
			if a, err := auth.GetAccount(c.Account); err == nil {
				who = a.Name
			}
			owner = html.EscapeString(who)
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td><code style="font-size:11px">%s</code></td>`+
			`<td>%s</td><td class="created-col">%s</td><td class="center">`+
			`<form method="POST" action="/admin/oauth" style="display:inline" `+
			`onsubmit="return confirm('Remove this client?')">`+
			`<input type="hidden" name="client_id" value="%s">`+
			`<button type="submit" style="font-size:13px">Remove</button></form></td></tr>`,
			html.EscapeString(c.Name), html.EscapeString(c.ClientID), owner,
			c.CreatedAt.Format("2006-01-02"), html.EscapeString(c.ClientID))
	}
	if len(clients) == 0 {
		b.WriteString(`<tr><td colspan="5" class="center text-muted">Nothing registered.</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)

	w.Write([]byte(app.RenderHTMLForRequest("Admin", "OAuth Clients", b.String(), r))) //nolint:errcheck
}
