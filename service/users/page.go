package users

// The page: everybody here, and a way to say something to any of them.
//
// The complaint this answers, in the operator's words: a hundred and eighty
// people had signed up and "I can't even talk to them... it all feels very
// isolated. Where are the people". They were not hidden by a bug. There was no
// page. The only list of accounts on the instance was /admin/users, which needs
// admin rights, and a directory that only the operator can read is not a
// network — it is a customer table.
//
// Signed in, because a public list of everybody who signed up is a different
// object from a public profile. Each /@name page is already public and always
// has been; what changes here is that they can be enumerated, and enumeration
// is the part that makes a directory worth scraping. A reader who has an
// account is somebody the instance already knows.
//
// Every row is a way to start something. A name, whether they are here now,
// and the two things you can do about it — read their page, or write to them.
// A directory you can only look at is a phone book with no phone.

import (
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

// Handler serves /users.
func Handler(w http.ResponseWriter, r *http.Request) {
	_, me, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	list := List()
	if q != "" {
		list = Find(q)
	}

	var online, rest []User
	for _, u := range list {
		if u.Profile.Online {
			online = append(online, u)
			continue
		}
		rest = append(rest, u)
	}

	var b strings.Builder
	b.WriteString(`<p class="svc-lead">Everybody on this instance — the people and ` +
		`the agents.`)
	// The address only where there is one. Unconfigured this said
	// "name@this instance", which is not an address and reads as a bug in the
	// place a reader is most likely to copy something.
	if d := mailDomain(); d != "" {
		b.WriteString(` Write to any of them at <code>name@` + html.EscapeString(d) + `</code>.`)
	}
	b.WriteString(`</p>`)

	// Search, because a hundred and eighty rows is a list you scroll and a
	// thousand is one you cannot.
	b.WriteString(`<form method="GET" action="/users" class="users-find">` +
		`<input name="q" class="field" placeholder="Find somebody" value="` +
		html.EscapeString(q) + `">` +
		`<button class="btn" type="submit">Find</button>`)
	if q != "" {
		b.WriteString(` <a class="mini-btn" href="/users">Clear</a>`)
	}
	b.WriteString(`</form>`)

	if len(list) == 0 {
		b.WriteString(app.Note("Nobody here matches " + html.EscapeString(q) + "."))
		app.Respond(w, r, app.Response{Title: "Users", Description: "Who is on this instance", HTML: b.String()})
		return
	}

	if len(online) > 0 {
		b.WriteString(`<h3 class="lead-15">Here now</h3>`)
		b.WriteString(rows(online, me.ID))
	}
	if len(rest) > 0 {
		if len(online) > 0 {
			b.WriteString(`<h3 class="lead-15">Everybody else</h3>`)
		}
		b.WriteString(rows(rest, me.ID))
	}

	app.Respond(w, r, app.Response{Title: "Users", Description: "Who is on this instance", HTML: b.String()})
}

// rows is the list itself.
func rows(list []User, me string) string {
	var b strings.Builder
	b.WriteString(`<table class="admin-table"><tbody>`)
	for _, u := range list {
		name := html.EscapeString(u.Display())
		badge := ""
		if u.Account.Agent {
			badge = ` <span class="count-badge quiet">agent</span>`
		}
		if u.ID == me {
			badge += ` <span class="count-badge info">you</span>`
		}

		// No <strong>. Every link on the site is already bold — see the `a` rule in
		// mu.css — so wrapping one in strong is bold twice, and a page where every
		// name is heavier than every name elsewhere reads as a different typeface
		// rather than as emphasis. Reported that way.
		who := `<a href="` + html.EscapeString(u.Profile.Page) + `">` + name + `</a>` + badge
		if u.Display() != u.ID {
			who += ` <span class="text-muted text-xs">@` + html.EscapeString(u.ID) + `</span>`
		}

		// Writing to somebody is the point of knowing they exist. Not offered
		// for yourself — a row that invites you to email yourself reads as a
		// bug rather than a feature.
		act := ""
		if u.ID != me {
			act = `<a class="mini-btn" href="/inbox/new?to=` +
				html.EscapeString(u.ID) + `">Send message</a>`
		}

		b.WriteString(`<tr>` +
			`<td data-label="Name">` + who + `</td>` +
			`<td data-label="" class="actions-cell">` + act + `</td>` +
			`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

// mailDomain is what an address here ends in, for the line at the top.
//
// The setting, not service/mail.ConfiguredDomain — a service may not import
// another service, and TestServicesDoNotImportEachOther caught this the moment
// it was written. What the two share is the name of a setting, which is the
// smallest thing they could share and needs no home in internal/.
func mailDomain() string { return strings.TrimSpace(settings.Get("MAIL_DOMAIN")) }
