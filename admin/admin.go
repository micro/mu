package admin

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/mail"
)

// AdminHandler shows the admin page with user management
func AdminHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	users := auth.GetAllAccounts()

	content := `<div class="admin-links">
		<a href="/admin/usage">API Usage</a>
		<a href="/admin/api">API Log</a>
		<a href="/admin/blocklist">Blocklist</a>
		<a href="/admin/console">Console</a>
		<a href="/admin/env">Environment</a>
		<a href="/admin/email">Mail Log</a>
		<a href="/admin/moderate">Moderation</a>
		<a href="/admin/server">Server</a>
		<a href="/admin/spam">Spam Filter</a>
		<a href="/admin/log">System Log</a>
		<a href="/admin/users">Users <span class="count">` + fmt.Sprintf("%d", len(users)) + `</span></a>
	</div>`

	html := app.RenderHTMLForRequest("Admin", "Admin Dashboard", content, r)
	w.Write([]byte(html))
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
		case "delete":
			if userID != acc.ID { auth.DeleteAccount(userID) }
		case "ban":
			auth.BanAccount(userID)
		case "unban":
			auth.UnbanAccount(userID)
		case "approve":
			auth.ApproveAccount(userID)
		}
		tab := r.FormValue("tab")
		redir := "/admin/users"
		if tab != "" { redir += "?tab=" + tab }
		http.Redirect(w, r, redir, http.StatusSeeOther)
		return
	}
	users := auth.GetAllAccounts()
	sort.Slice(users, func(i, j int) bool { return users[i].Created.After(users[j].Created) })
	tab := r.URL.Query().Get("tab")
	if tab == "" { tab = "all" }
	var sb strings.Builder
	sb.WriteString(`<p><a href="/admin">← Admin</a></p><h2>Users</h2>`)
	sb.WriteString(`<div style="display:flex;gap:6px;margin-bottom:16px;flex-wrap:wrap">`)
	for _, t := range []struct{ id, label string }{{"all", "All"}, {"banned", "Banned"}, {"new", "New (24h)"}} {
		style := "padding:4px 14px;border-radius:14px;font-size:13px;text-decoration:none;color:#555"
		if t.id == tab { style = "padding:4px 14px;border-radius:14px;font-size:13px;text-decoration:none;background:#000;color:#fff" }
		sb.WriteString(fmt.Sprintf(`<a href="/admin/users?tab=%s" style="%s">%s</a>`, t.id, style, t.label))
	}
	sb.WriteString(`</div>`)
	var filtered []*auth.Account
	for _, u := range users {
		switch tab {
		case "banned":
			if u.Banned { filtered = append(filtered, u) }
		case "new":
			if time.Since(u.Created) < 24*time.Hour { filtered = append(filtered, u) }
		default:
			filtered = append(filtered, u)
		}
	}
	sb.WriteString(fmt.Sprintf(`<p class="text-muted text-sm">%d users</p>`, len(filtered)))
	sb.WriteString(`<table class="admin-table"><thead><tr><th>Username</th><th>Name</th><th class="created-col">Created</th><th>Status</th><th class="center">Actions</th></tr></thead><tbody>`)
	for _, u := range filtered {
		created := u.Created.Format("2006-01-02")
		var badges []string
		if u.Admin { badges = append(badges, `<span style="background:#000;color:#fff;padding:1px 6px;border-radius:8px;font-size:11px">admin</span>`) }
		if u.Banned { badges = append(badges, `<span style="background:#c00;color:#fff;padding:1px 6px;border-radius:8px;font-size:11px">banned</span>`) }
		if u.EmailVerified { badges = append(badges, `<span style="background:#22c55e;color:#fff;padding:1px 6px;border-radius:8px;font-size:11px">verified</span>`) }
		if u.Approved { badges = append(badges, `<span style="background:#06b;color:#fff;padding:1px 6px;border-radius:8px;font-size:11px">approved</span>`) }
		statusHTML := strings.Join(badges, " ")
		if statusHTML == "" { statusHTML = `<span class="text-muted" style="font-size:12px">—</span>` }
		var actions []string
		if u.ID != acc.ID {
			if u.Banned {
				actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline"><input type="hidden" name="action" value="unban"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" style="font-size:12px;padding:2px 8px;border-radius:4px;border:1px solid #22c55e;background:#fff;color:#22c55e;cursor:pointer">Unban</button></form>`, u.ID, tab))
			} else {
				actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline"><input type="hidden" name="action" value="ban"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" style="font-size:12px;padding:2px 8px;border-radius:4px;border:1px solid #c00;background:#fff;color:#c00;cursor:pointer" onclick="return confirm('Ban %s?')">Ban</button></form>`, u.ID, tab, u.ID))
			}
			actions = append(actions, fmt.Sprintf(`<form method="POST" class="d-inline" onsubmit="return confirm('Delete %s?')"><input type="hidden" name="action" value="delete"><input type="hidden" name="user_id" value="%s"><input type="hidden" name="tab" value="%s"><button type="submit" class="btn-danger" style="font-size:12px;padding:2px 8px">Delete</button></form>`, u.ID, u.ID, tab))
		}
		sb.WriteString(fmt.Sprintf(`<tr><td><strong><a href="/@%s">%s</a></strong></td><td>%s</td><td class="created-col">%s</td><td>%s</td><td class="center" style="white-space:nowrap">%s</td></tr>`, u.ID, u.ID, u.Name, created, statusHTML, strings.Join(actions, " ")))
	}
	sb.WriteString(`</tbody></table>`)
	html := app.RenderHTMLForRequest("Admin", "Users", sb.String(), r)
	w.Write([]byte(html))
}

// BlocklistHandler shows and manages the mail blocklist
func BlocklistHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// Handle POST requests for blocklist actions
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		action := r.FormValue("action")

		switch action {
		case "block_email":
			email := r.FormValue("email")
			if email == "" {
				http.Error(w, "Email required", http.StatusBadRequest)
				return
			}
			// Import mail package to access BlockEmail
			if err := blockEmail(email); err != nil {
				http.Error(w, "Failed to block email: "+err.Error(), http.StatusBadRequest)
				return
			}

		case "block_ip":
			ip := r.FormValue("ip")
			if ip == "" {
				http.Error(w, "IP required", http.StatusBadRequest)
				return
			}
			if err := blockIP(ip); err != nil {
				http.Error(w, "Failed to block IP: "+err.Error(), http.StatusBadRequest)
				return
			}

		case "unblock_email":
			email := r.FormValue("email")
			if err := unblockEmail(email); err != nil {
				http.Error(w, "Failed to unblock email: "+err.Error(), http.StatusBadRequest)
				return
			}

		case "unblock_ip":
			ip := r.FormValue("ip")
			if err := unblockIP(ip); err != nil {
				http.Error(w, "Failed to unblock IP: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		http.Redirect(w, r, "/admin/blocklist", http.StatusSeeOther)
		return
	}

	// GET request - show blocklist
	bl := getBlocklist()

	content := `<h2>Mail Blocklist</h2>

	<div class="blocklist-section">
		<h3>Blocked Emails (` + fmt.Sprintf("%d", len(bl.Emails)) + `)</h3>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_email">
				<input type="text" name="email" placeholder="email@example.com or *@domain.com" required>
				<button type="submit">Block Email</button>
			</form>
			<p class="text-sm text-muted mt-1">Use *@domain.com to block entire domain</p>
		</div>`

	if len(bl.Emails) > 0 {
		content += `<table class="blacklist-table">
			<thead>
				<tr>
					<th>Email</th>
					<th class="text-center" style="width: 100px;">Action</th>
				</tr>
			</thead>
			<tbody>`

		for _, email := range bl.Emails {
			content += `
				<tr>
					<td><code>` + email + `</code></td>
					<td class="text-center">
						<form method="POST" class="d-inline">
							<input type="hidden" name="action" value="unblock_email">
							<input type="hidden" name="email" value="` + email + `">
							<button type="submit" class="btn-success">Unblock</button>
						</form>
					</td>
				</tr>`
		}

		content += `</tbody></table>`
	} else {
		content += `<p>No blocked emails</p>`
	}

	content += `</div>

	<div class="blocklist-section">
		<h3>Blocked IPs (` + fmt.Sprintf("%d", len(bl.IPs)) + `)</h3>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_ip">
				<input type="text" name="ip" placeholder="192.168.1.1" required>
				<button type="submit">Block IP</button>
			</form>
		</div>`

	if len(bl.IPs) > 0 {
		content += `<table class="blacklist-table">
			<thead>
				<tr>
					<th>IP Address</th>
					<th class="text-center" style="width: 100px;">Action</th>
				</tr>
			</thead>
			<tbody>`

		for _, ip := range bl.IPs {
			content += `
				<tr>
					<td><code>` + ip + `</code></td>
					<td class="text-center">
						<form method="POST" class="d-inline">
							<input type="hidden" name="action" value="unblock_ip">
							<input type="hidden" name="ip" value="` + ip + `">
							<button type="submit" class="btn-success">Unblock</button>
						</form>
					</td>
				</tr>`
		}

		content += `</tbody></table>`
	} else {
		content += `<p>No blocked IPs</p>`
	}

	content += `</div>
	<div class="mt-6">
		<p><a href="/admin">← Back to Admin</a></p>
	</div>`

	html := app.RenderHTMLForRequest("Admin", "Mail Blocklist", content, r)
	w.Write([]byte(html))
}

// SpamFilterHandler shows and manages the spam filter settings
func SpamFilterHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		action := r.FormValue("action")
		value := r.FormValue("value")

		switch action {
		case "toggle":
			sf := mail.GetSpamFilter()
			mail.SetSpamFilterEnabled(!sf.Enabled) //nolint:errcheck
		case "set_threshold":
			t := 5
			fmt.Sscanf(value, "%d", &t)
			mail.SetSpamThreshold(t) //nolint:errcheck
		case "toggle_reject":
			sf := mail.GetSpamFilter()
			mail.SetRejectSpam(!sf.RejectSpam) //nolint:errcheck
		case "toggle_autoblock":
			sf := mail.GetSpamFilter()
			mail.SetAutoBlockDomains(!sf.AutoBlockDomains) //nolint:errcheck
		case "add_tld":
			if value != "" {
				mail.AddBlockedTLD(value) //nolint:errcheck
			}
		case "remove_tld":
			if value != "" {
				mail.RemoveBlockedTLD(value) //nolint:errcheck
			}
		case "add_keyword":
			if value != "" {
				mail.AddBlockedKeyword(value) //nolint:errcheck
			}
		case "remove_keyword":
			if value != "" {
				mail.RemoveBlockedKeyword(value) //nolint:errcheck
			}
		case "add_allowed":
			if value != "" {
				mail.AddAllowedSender(value) //nolint:errcheck
			}
		case "remove_allowed":
			if value != "" {
				mail.RemoveAllowedSender(value) //nolint:errcheck
			}
		}

		http.Redirect(w, r, "/admin/spam", http.StatusSeeOther)
		return
	}

	sf := mail.GetSpamFilter()

	enabledStatus := "Disabled"
	enabledBtn := "Enable"
	if sf.Enabled {
		enabledStatus = "Enabled"
		enabledBtn = "Disable"
	}

	rejectStatus := "Drop silently"
	rejectBtn := "Switch to reject"
	if sf.RejectSpam {
		rejectStatus = "Save to filtered folder"
		rejectBtn = "Switch to silent drop"
	}

	autoBlockStatus := "Off"
	autoBlockBtn := "Enable"
	if sf.AutoBlockDomains {
		autoBlockStatus = "On"
		autoBlockBtn = "Disable"
	}

	content := fmt.Sprintf(`<h2>Spam Filter</h2>

	<div class="spam-settings">
		<h3>Settings</h3>
		<table class="blacklist-table">
			<tr>
				<td><strong>Filter Status</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Spam Handling</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_reject">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Auto-block spam domains</strong></td>
				<td>%s</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_autoblock">
						<button type="submit">%s</button>
					</form>
				</td>
			</tr>
			<tr>
				<td><strong>Score Threshold</strong></td>
				<td>%d</td>
				<td>
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="set_threshold">
						<input type="number" name="value" value="%d" min="1" max="100" style="width:60px">
						<button type="submit">Set</button>
					</form>
				</td>
			</tr>
		</table>
	</div>`, enabledStatus, enabledBtn, rejectStatus, rejectBtn,
		autoBlockStatus, autoBlockBtn, sf.Threshold, sf.Threshold)

	// Blocked TLDs
	content += `<div class="spam-section mt-4">
		<h3>Blocked TLDs (` + fmt.Sprintf("%d", len(sf.BlockedTLDs)) + `)</h3>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_tld">
			<input type="text" name="value" placeholder=".vn, .xyz, .top" required>
			<button type="submit">Block TLD</button>
		</form>`

	if len(sf.BlockedTLDs) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, tld := range sf.BlockedTLDs {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_tld">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, tld, tld)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	// Blocked keywords
	content += `<div class="spam-section mt-4">
		<h3>Blocked Keywords (` + fmt.Sprintf("%d", len(sf.BlockedKeywords)) + `)</h3>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_keyword">
			<input type="text" name="value" placeholder="keyword or phrase" required>
			<button type="submit">Block Keyword</button>
		</form>`

	if len(sf.BlockedKeywords) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, kw := range sf.BlockedKeywords {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_keyword">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, kw, kw)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	// Allowed senders
	content += `<div class="spam-section mt-4">
		<h3>Allowed Senders (` + fmt.Sprintf("%d", len(sf.AllowedSenders)) + `)</h3>
		<p class="text-sm text-muted">These senders bypass spam checks. Use @domain.com for entire domains.</p>
		<form method="POST" class="block-form">
			<input type="hidden" name="action" value="add_allowed">
			<input type="text" name="value" placeholder="user@example.com or @domain.com" required>
			<button type="submit">Allow Sender</button>
		</form>`

	if len(sf.AllowedSenders) > 0 {
		content += `<table class="blacklist-table"><tbody>`
		for _, s := range sf.AllowedSenders {
			content += fmt.Sprintf(`<tr><td><code>%s</code></td><td class="text-center">
				<form method="POST" class="d-inline">
					<input type="hidden" name="action" value="remove_allowed">
					<input type="hidden" name="value" value="%s">
					<button type="submit" class="btn-success">Remove</button>
				</form></td></tr>`, s, s)
		}
		content += `</tbody></table>`
	}
	content += `</div>`

	content += `<div class="mt-6">
		<p><a href="/admin">← Back to Admin</a></p>
	</div>`

	htmlPage := app.RenderHTMLForRequest("Admin", "Spam Filter", content, r)
	w.Write([]byte(htmlPage))
}

// Helper functions to access mail package functions
func blockEmail(email string) error {
	return mail.BlockEmail(email)
}

func blockIP(ip string) error {
	return mail.BlockIP(ip)
}

func unblockEmail(email string) error {
	return mail.UnblockEmail(email)
}

func unblockIP(ip string) error {
	return mail.UnblockIP(ip)
}

func getBlocklist() *mail.Blocklist {
	return mail.GetBlocklist()
}
