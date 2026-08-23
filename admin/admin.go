package admin

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/account"
	"mu/internal/app"
	"mu/internal/auth"
)

// Handler shows the admin page with user management
func Handler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	users := auth.AllAccounts()

	// Alphabetical. There is no ranking to express here, and a list that is
	// sorted is one nobody has to scan twice.
	content := `<div class="admin-links">
		<a href="/admin/alerts">Alerts</a>
		<a href="/admin/backup">Backup</a>
		<a href="/admin/config">Config</a>
		<a href="/admin/log">Logs` + alertBadge() + `</a>
		<a href="/admin/oauth">OAuth Clients</a>
		<a href="/admin/moderate">Moderation</a>
		<a href="/admin/server">Server</a>
		<a href="/admin/spam">Spam</a>
		<a href="/admin/traffic">Usage</a>
		<a href="/admin/users">Users <span class="count">` + fmt.Sprintf("%d", len(users)) + `</span></a>
	</div>`

	app.Respond(w, r, app.Response{Title: "Admin", Description: "Admin Dashboard", HTML: content})
}

// alertBadge puts the number of outstanding alerts beside the log, so that
// something serious is visible from the one page an admin actually opens.
//
// An alert that only exists in a log nobody has scrolled to is the same as no
// alert at all — which is what three of them were, written as the word CRITICAL
// at the front of an ordinary line.
func alertBadge() string {
	n := app.AlertCount()
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(` <span class="count row-danger">%d</span>`, n)
}

// balanceCell is what an account holds, on the row beside the button that adds
// to it.
//
// Granting credits without seeing the balance is working blind: the question is
// almost always "does this account have enough", and answering it meant leaving
// the page for the console. Zero is shown as a dash rather than 0 — a list where
// most rows read "0" is a column of noise around the few that matter.
func balanceCell(userID string) string {
	n := account.Balance(userID)
	if n == 0 {
		return `<span class="text-muted text-xs">—</span>`
	}
	return fmt.Sprintf(`<span class="text-sm">%d</span>`, n)
}

// UsersHandler shows and manages users with tabs: All, Banned, New.
func UsersHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}
	if r.Method == "POST" {
		r.ParseForm()
		action := r.FormValue("action")
		userID := r.FormValue("user_id")
		if userID == "" {
			app.BadRequest(w, r, "User ID required")
			return
		}
		switch action {
		case "toggle_admin":
			if u, err := auth.GetAccount(userID); err == nil {
				u.Admin = !u.Admin
				auth.UpdateAccount(u)
			}
		// Marking an account as a program rather than a person. It decides two
		// things: an agent account is recorded but not charged (the instance is
		// acting for itself, so there is nobody else to bill), and anything that
		// would write to a human — a password reset, a verification mail —
		// knows not to. Admin-only, which is what keeps "I am an agent" from
		// being a caller's own claim to be free.
		case "toggle_agent":
			if u, err := auth.GetAccount(userID); err == nil {
				u.Agent = !u.Agent
				auth.UpdateAccount(u)
			}
		case "delete":
			if userID != acc.ID {
				auth.DeleteAccount(userID)
			}
		case "ban":
			auth.BanAccount(userID)
		case "unban":
			auth.UnbanAccount(userID)
		case "approve":
			auth.ApproveAccount(userID)

		// Crediting an account by hand.
		//
		// It existed only as a console command — `credit <user> <amount>` at
		// /admin/console — which is to say it existed for somebody who already
		// knew it was there. An operator looking at a list of users, wanting to
		// comp one of them, found buttons for banning and approving and nothing
		// about money, and reasonably concluded the product could not do it.
		//
		// The same call the console makes, so there is one way credits are
		// granted and one operation in the ledger to look for afterwards.
		case "credit":
			var amount int
			fmt.Sscanf(strings.TrimSpace(r.FormValue("amount")), "%d", &amount)
			if amount > 0 {
				if u, err := auth.GetAccount(userID); err == nil {
					if err := account.AddCredits(u.ID, amount, "admin_grant", map[string]interface{}{
						"by": acc.ID,
					}); err != nil {
						app.Log("admin", "granting %d to %s: %v", amount, u.ID, err)
					} else {
						app.Log("admin", "%s granted %d credits to %s", acc.ID, amount, u.ID)
					}
				}
			}
		}
		tab := r.FormValue("tab")
		redir := "/admin/users"
		if tab != "" {
			redir += "?tab=" + tab
		}
		http.Redirect(w, r, redir, http.StatusSeeOther)
		return
	}
	users := auth.AllAccounts()
	sort.Slice(users, func(i, j int) bool { return users[i].Created.After(users[j].Created) })
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "all"
	}
	var sb strings.Builder
	// Invites are on this page rather than in the dashboard list, because the
	// question "who is here" and the question "who should be" are the same
	// errand, and an operator looking at a list of accounts is one click from
	// wanting to add one. It kept its own nav entry only because it had its own
	// handler.
	// No <h2>Users</h2>. The page shell already draws the title as an h1, and
	// this page said "Admin" up there and "Users" directly under it — the same
	// heading twice, one of them wrong. Every admin page did it.
	sb.WriteString(back() +
		`<p><a href="/admin/invite">Invites` + pendingInvites() + ` &rarr;</a></p>`)
	sb.WriteString(`<div class="app-filters">`)
	for _, t := range []struct{ id, label string }{{"all", "All"}, {"banned", "Banned"}, {"new", "New (24h)"}} {
		sb.WriteString(app.PillLink(t.label, "/admin/users?tab="+t.id, t.id == tab))
	}
	sb.WriteString(`</div>`)
	var filtered []*auth.Account
	for _, u := range users {
		switch tab {
		case "banned":
			if u.Banned {
				filtered = append(filtered, u)
			}
		case "new":
			if time.Since(u.Created) < 24*time.Hour {
				filtered = append(filtered, u)
			}
		default:
			filtered = append(filtered, u)
		}
	}
	sb.WriteString(fmt.Sprintf(`<p class="text-muted text-sm">%d users</p>`, len(filtered)))
	sb.WriteString(`<table class="admin-table"><thead><tr><th>Username</th><th>Name</th><th class="created-col">Created</th><th>Status</th><th class="center">Credits</th><th class="center">Actions</th></tr></thead><tbody>`)
	for _, u := range filtered {
		created := u.Created.Format("2006-01-02")
		var badges []string
		if u.Admin {
			badges = append(badges, `<span class="count-badge">admin</span>`)
		}
		if u.Agent {
			badges = append(badges, `<span class="count-badge quiet">agent</span>`)
		}
		if u.Banned {
			badges = append(badges, `<span class="count-badge">banned</span>`)
		}
		if u.EmailVerified {
			badges = append(badges, `<span class="count-badge good">verified</span>`)
		}
		if u.Approved {
			badges = append(badges, `<span class="count-badge info">approved</span>`)
		}
		statusHTML := strings.Join(badges, " ")
		if statusHTML == "" {
			statusHTML = `<span class="text-muted text-xs">—</span>`
		}
		var actions []string
		agentLabel, agentTitle := "Mark agent", "Mark as a program: recorded, not charged"
		if u.Agent {
			agentLabel, agentTitle = "Mark human", "Mark as a person: charged like any other account"
		}
		actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline"><input type="hidden" name="action" value="toggle_agent"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" title="%s" class="mini-btn">%s</button></form>`,
			u.ID, tab, agentTitle, agentLabel))
		if u.ID != acc.ID {
			if u.Banned {
				actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline"><input type="hidden" name="action" value="unban"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" class="mini-btn good">Unban</button></form>`, u.ID, tab))
			} else {
				actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline"><input type="hidden" name="action" value="ban"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" class="mini-btn danger" onclick="return confirm('Ban %s?')">Ban</button></form>`, u.ID, tab, u.ID))
			}
			actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline" onsubmit="return confirm('Delete %s?')"><input type="hidden" name="action" value="delete"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" class="btn-danger text-xs p-tight">Delete</button></form>`, u.ID, u.ID, tab))
		}
		// Credit, on the row, because that is where somebody wanting to comp an
		// account is looking. An amount box rather than fixed buttons: the
		// number is different every time — a refund is what a call cost, a comp
		// is what you feel like giving — and three preset buttons would be
		// wrong for most of them.
		actions = append(actions, fmt.Sprintf(
			`<form method="POST" class="d-inline inline-row">`+
				`<input type="hidden" name="action" value="credit">`+
				`<input type="hidden" name="user_id" value="%s">`+
				`<input type="hidden" name="tab" value="%s">`+
				`<input name="amount" inputmode="numeric" pattern="[0-9]*" placeholder="credits" `+
				`class="num-field">`+
				`<button type="submit" title="Top up this account" `+
				`class="mini-btn info">Credit</button></form>`,
			u.ID, tab))

		sb.WriteString(fmt.Sprintf(`<tr><td><strong><a href="/@%s">%s</a></strong></td><td>%s</td><td class="created-col">%s</td><td>%s</td><td class="center">%s</td><td class="center nowrap">%s</td></tr>`, u.ID, u.ID, u.Name, created, statusHTML, balanceCell(u.ID), strings.Join(actions, " ")))
	}
	sb.WriteString(`</tbody></table>`)
	app.Respond(w, r, app.Response{Title: "Users", Description: "Accounts on this instance", HTML: sb.String()})
}

// back is the way up, in the same words and the same place on every page.
//
// It was "← Admin" on four pages and "← Back to Admin" on five, at the top on
// some and the bottom on others, and on one page both. The top: a way out
// belongs where you can see it without reading to the end.
func back() string { return `<p><a href="/admin">← Admin</a></p>` }
