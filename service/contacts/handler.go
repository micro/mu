package contacts

// The address book, for the person who owns it.
//
// Contacts was headless: a real service with real per-user data that only an
// agent could reach. That is the wrong shape for this product — two doors onto
// one set of services, and a service that appears in one of them is half built.
// It was also the thing you would most want to check by eye, since an agent
// mailing the wrong Sarah is a mistake you find out about afterwards.
//
// The page does exactly what the tools do, through the same functions, so the
// two cannot drift.

import (
	"fmt"
	"html"
	"net/http"
	neturl "net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
)

// Handler serves /contacts.
func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/contacts"), "/")
		handleAction(w, r, action)
		return
	}
	if app.WantsJSON(r) {
		handleJSON(w, r)
		return
	}
	listPage(w, r)
}

// handleJSON answers the page's own data as JSON, for anything reading it as an
// endpoint rather than a page.
func handleJSON(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		app.RespondJSON(w, map[string]any{"query": q, "contacts": Find(sess.Account, q)})
		return
	}
	app.RespondJSON(w, map[string]any{"contacts": List(sess.Account)})
}

// handleAction adds or removes a contact and returns to the page.
func handleAction(w http.ResponseWriter, r *http.Request, action string) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !auth.ValidCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}

	var actErr error
	switch {
	case action == "":
		_, actErr = Add(sess.Account,
			r.FormValue("name"), r.FormValue("email"),
			r.FormValue("phone"), r.FormValue("note"))
	case strings.HasSuffix(action, "/delete"):
		actErr = Remove(sess.Account, strings.TrimSuffix(action, "/delete"))
	default:
		app.NotFound(w, r, "Unknown action")
		return
	}

	// A mistake someone can fix — a missing name, a bad address — belongs on the
	// page they came from, not on an error screen.
	dest := "/contacts"
	if actErr != nil {
		dest += "?error=" + neturl.QueryEscape(actErr.Error())
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

func listPage(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	auth.SetCSRFCookie(w, r)
	csrf := auth.CSRFToken(r)
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	people := List(sess.Account)
	if query != "" {
		people = Find(sess.Account, query)
	}

	var b strings.Builder
	b.WriteString(`<div class="card">`)
	b.WriteString(`<p class="text-sm text-muted">Names your agent can turn into addresses. ` +
		`Ask it to mail someone and it looks them up here.</p>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		b.WriteString(`<p class="text-error">` + html.EscapeString(msg) + `</p>`)
	}

	fmt.Fprintf(&b, `<form method="GET" action="/contacts" class="contact-search">
  <input type="search" name="q" value="%s" placeholder="Search by name or address">
  <button type="submit">Search</button>
</form>`, html.EscapeString(query))

	// Add. Only the name is required — a contact with just a name is still
	// worth having, and the rest can be filled in later by saying so.
	fmt.Fprintf(&b, `<form method="POST" action="/contacts" class="contact-add">
  <input type="hidden" name="_csrf" value="%s">
  <input name="name" placeholder="Name" required>
  <input name="email" type="email" placeholder="Email">
  <input name="phone" placeholder="Phone">
  <input name="note" placeholder="Note">
  <button type="submit">Add</button>
</form>`, html.EscapeString(csrf))
	b.WriteString(`</div>`)

	if len(people) == 0 {
		b.WriteString(`<div class="card"><p class="text-sm text-muted">`)
		if query != "" {
			b.WriteString(`Nobody matches “` + html.EscapeString(query) + `”. <a href="/contacts">Show everyone</a>.`)
		} else {
			b.WriteString(`No contacts yet. Add someone above, or tell the agent — ` +
				`“remember Sarah's email is sarah@example.com” — and it will.`)
		}
		b.WriteString(`</p></div>`)
	} else {
		b.WriteString(`<div class="card"><table class="data-table contacts-table">`)
		b.WriteString(`<thead><tr><th>Name</th><th>Email</th><th>Phone</th><th>Note</th><th></th></tr></thead><tbody>`)
		for _, c := range people {
			// Classed cells, not positional ones: a phone renders this as a
			// block, the same way the files list does.
			fmt.Fprintf(&b, `<tr><td class="contact-name">%s</td>`, html.EscapeString(c.Name))
			mailLink := "—"
			if c.Email != "" {
				mailLink = `<a href="/mail?to=` + html.EscapeString(neturl.QueryEscape(c.Email)) + `">` +
					html.EscapeString(c.Email) + `</a>`
			}
			fmt.Fprintf(&b, `<td class="contact-meta">%s</td><td class="contact-meta">%s</td><td class="contact-meta">%s</td>`,
				mailLink, orDash(c.Phone), orDash(c.Note))

			fmt.Fprintf(&b, `<td class="contact-actions"><form method="POST" action="/contacts/%s/delete" onsubmit="return confirm('Remove %s?')">
  <input type="hidden" name="_csrf" value="%s">
  <button type="submit" class="link-button danger">Remove</button>
</form></td></tr>`,
				html.EscapeString(c.ID),
				html.EscapeString(strings.ReplaceAll(c.Name, "'", "\\'")),
				html.EscapeString(csrf))
		}
		b.WriteString(`</tbody></table></div>`)
	}

	b.WriteString(contactsPageCSS)
	w.Write([]byte(app.RenderHTMLForRequest("Contacts", "Your address book", b.String(), r)))
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return html.EscapeString(s)
}

// contactsPageCSS styles the page and, below 600px, unmakes the table — the
// same treatment the files list gets, for the same reason: five columns on a
// phone either scroll sideways or crush the name.
const contactsPageCSS = `<style>
.contact-search{display:flex;gap:8px;margin:10px 0}
.contact-search input{flex:1;min-width:0}
.contact-add{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px;margin:12px 0 4px}
.contact-add input{min-width:0}
.contacts-table{margin-bottom:0}
.contacts-table .contact-name{font-weight:var(--font-weight-medium)}
.contact-actions{white-space:nowrap}
.contact-actions form{display:inline}

@media only screen and (max-width:600px){
  .contact-add{grid-template-columns:1fr}
  .contact-add button{width:100%}
  .contacts-table,.contacts-table tbody,.contacts-table tr,.contacts-table td{display:block;width:auto}
  .contacts-table thead{display:none}
  .contacts-table tr{padding:12px 0;border-bottom:1px solid var(--divider)}
  .contacts-table tbody tr:last-child{border-bottom:none}
  .contacts-table td{padding:0;border:none;text-align:left}
  .contacts-table .contact-name{margin-bottom:2px}
  .contacts-table .contact-meta{display:inline;color:var(--text-muted);font-size:13px}
  .contacts-table .contact-meta + .contact-meta::before{content:" · "}
  .contacts-table td.contact-actions{margin-top:6px;text-align:left}
  .contacts-table .contact-actions .link-button{padding:6px 14px 6px 0;font-size:14px}
  .contacts-table tbody tr:nth-child(odd){background:none}
}
</style>`
