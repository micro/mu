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
		id := strings.TrimSpace(r.FormValue("client_id"))
		switch {
		case id == "":
		// Setting an address rather than removing, so a client whose id is
		// already pasted into somebody's config can be made to work instead of
		// having to come back under a new one.
		case r.FormValue("action") == "redirect":
			uri := strings.TrimSpace(r.FormValue("redirect_uri"))
			if err := auth.SetOAuthRedirects(id, []string{uri}); err != nil {
				app.BadRequest(w, r, err.Error())
				return
			}
			app.Log("admin", "oauth client %s now redirects to %s", id, uri)
		default:
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
		`can remove it. Removing one here stops it signing anybody in until it registers ` +
		`again, which it will do the next time it connects.</p>`)
	b.WriteString(`<p class="text-muted text-sm">A code is only ever sent to an address ` +
		`the client registered, matched exactly — the port aside, for loopback. The ` +
		`addresses each one gave are below, so a client that stops working after this ` +
		`can be told apart from one that never registered the address it is asking for.</p>`)

	suffix := "s"
	if len(clients) == 1 {
		suffix = ""
	}
	fmt.Fprintf(&b, `<p class="text-muted text-sm">%d client%s.</p>`, len(clients), suffix)
	b.WriteString(`<table class="admin-table"><thead><tr><th>Name</th><th>Client ID</th>` +
		`<th>Redirects to</th><th>Owner</th><th class="created-col">Created</th>` +
		`<th class="center"></th></tr></thead><tbody>`)

	for _, c := range clients {
		owner := `<span class="text-muted">self-registered</span>`
		if c.Account != "" {
			who := c.Account
			if a, err := auth.GetAccount(c.Account); err == nil {
				who = a.Name
			}
			owner = html.EscapeString(who)
		}
		// No address means the client cannot complete a sign-in at all, so the
		// row offers the one thing that fixes it. Every client the /token form
		// made before it asked for an address is in this state, and none of
		// them ever worked.
		where := `<form method="POST" action="/admin/oauth" style="margin:0;display:flex;gap:4px">` +
			`<input type="hidden" name="action" value="redirect">` +
			`<input type="hidden" name="client_id" value="` + html.EscapeString(c.ClientID) + `">` +
			`<input type="text" name="redirect_uri" placeholder="https://… or http://localhost:0/callback" ` +
			`style="font-size:11px;width:230px" value="` + html.EscapeString(firstURI(c.RedirectURIs)) + `">` +
			`<button type="submit" class="text-2xs">Set</button></form>`
		if len(c.RedirectURIs) == 0 {
			where = `<span class="text-muted text-2xs">none — cannot sign anybody in</span><br>` + where
		}
		fmt.Fprintf(&b, `<tr><td>%s</td><td><code class="text-2xs">%s</code></td>`+
			`<td>%s</td><td>%s</td><td class="created-col">%s</td><td class="center">`+
			`<form method="POST" action="/admin/oauth" class="d-inline" `+
			`onsubmit="return confirm('Remove this client?')">`+
			`<input type="hidden" name="client_id" value="%s">`+
			`<button type="submit" class="text-sm">Remove</button></form></td></tr>`,
			html.EscapeString(c.Name), html.EscapeString(c.ClientID), where, owner,
			c.CreatedAt.Format("2006-01-02"), html.EscapeString(c.ClientID))
	}
	if len(clients) == 0 {
		b.WriteString(`<tr><td colspan="6" class="center text-muted">Nothing registered.</td></tr>`)
	}
	b.WriteString(`</tbody></table>`)

	app.Respond(w, r, app.Response{Title: "Admin", Description: "OAuth Clients", HTML: b.String()})
}

// firstURI is what to show in the row's box: the address it has, or nothing.
func firstURI(uris []string) string {
	if len(uris) == 0 {
		return ""
	}
	return uris[0]
}
