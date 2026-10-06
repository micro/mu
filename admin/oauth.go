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
	"time"

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
	connections := auth.OAuthConnections()

	var b strings.Builder
	b.WriteString(`<p class="text-muted">Applications registered to sign in to this instance.</p><details class="disclosure"><summary>About registrations and redirects</summary><p>MCP clients can register anonymously. Clients created on an account’s token page retain that owner. Removing a client stops new sign-ins until it registers again.</p><p>Redirect addresses must match the registered address exactly, apart from the port for loopback addresses.</p></details>`)

	suffix := "s"
	if len(clients) == 1 {
		suffix = ""
	}
	fmt.Fprintf(&b, `<p class="text-muted text-sm">%d client%s.</p>`, len(clients), suffix)
	b.WriteString(`<div class="table-scroll" tabindex="0" role="region" aria-label="OAuth clients"><table class="data-table table-records"><thead><tr><th>Client</th><th>Registration</th><th>Redirects to</th><th>Account connections</th><th>Actions</th></tr></thead><tbody>`)

	for _, c := range clients {
		owner := `<span class="text-muted">Anonymous registration</span>`
		if c.Account != "" {
			owner = html.EscapeString(c.Account)
		}
		var connected strings.Builder
		for _, connection := range connections[c.ClientID] {
			state := "Active"
			if !connection.ExpiresAt.IsZero() && connection.ExpiresAt.Before(time.Now()) {
				state = "Expired"
			}
			lastUsed := "Never used"
			if !connection.LastUsed.IsZero() {
				lastUsed = "Last used " + connection.LastUsed.UTC().Format("2 Jan 15:04 UTC")
			}
			if connection.Legacy {
				state += "; inferred from legacy token name"
			}
			fmt.Fprintf(&connected, `<div class="collection-item"><strong>%s</strong><p class="text-sm text-muted">%s · Issued %s · %s</p></div>`, html.EscapeString(connection.Account), state, connection.Created.UTC().Format("2 Jan 15:04 UTC"), lastUsed)
		}
		if connected.Len() == 0 {
			connected.WriteString(`<span class="text-muted">No retained tokens</span>`)
		}
		// No address means the client cannot complete a sign-in at all, so the
		// row offers the one thing that fixes it. Every client the /token form
		// made before it asked for an address is in this state, and none of
		// them ever worked.
		where := `<form method="POST" action="/admin/oauth" class="form-inline">` +
			`<input type="hidden" name="action" value="redirect">` +
			`<input type="hidden" name="client_id" value="` + html.EscapeString(c.ClientID) + `">` +
			`<input type="text" name="redirect_uri" placeholder="https://… or http://localhost:0/callback" ` +
			`aria-label="Redirect URI" value="` + html.EscapeString(firstURI(c.RedirectURIs)) + `">` +
			`<button type="submit" >Set</button></form>`
		if len(c.RedirectURIs) == 0 {
			where = `<span class="text-muted text-sm">none — cannot sign anybody in</span>` + where
		}
		fmt.Fprintf(&b, `<tr><td data-label="Client"><strong>%s</strong><div class="text-sm text-muted"><code>%s</code></div></td>`+
			`<td data-label="Registration">%s<div class="text-sm text-muted">%s</div></td><td data-label="Redirects to">%s</td><td data-label="Account connections">%s</td><td data-label="Actions">`+
			`<form method="POST" action="/admin/oauth" class="form-action" `+
			`onsubmit="return confirm('Remove this client?')">`+
			`<input type="hidden" name="client_id" value="%s">`+
			`<button type="submit" class="btn-danger">Remove</button></form></td></tr>`,
			html.EscapeString(c.Name), html.EscapeString(c.ClientID), owner,
			c.CreatedAt.Format("2006-01-02"), where, connected.String(), html.EscapeString(c.ClientID))
	}
	if len(clients) == 0 {
		b.WriteString(`<tr><td colspan="5" class="center text-muted">Nothing registered.</td></tr>`)
	}
	b.WriteString(`</tbody></table></div>`)

	app.Respond(w, r, app.Response{Title: "OAuth Clients", Description: "Applications that may sign somebody in here", HTML: b.String()})
}

// firstURI is what to show in the row's box: the address it has, or nothing.
func firstURI(uris []string) string {
	if len(uris) == 0 {
		return ""
	}
	return uris[0]
}
