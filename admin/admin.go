package admin

import (
	"fmt"
	"net/http"
	"sort"

	"mu/app"
	"mu/auth"
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

	content := `<h2>Admin Dashboard</h2>
	<div class="admin-links">
		<a href="/admin/users">Users <span class="count">` + fmt.Sprintf("%d", len(users)) + `</span></a>
		<a href="/admin/moderate">Moderation Queue</a>
		<a href="/admin/blocklist">Mail Blocklist</a>
		<a href="/admin/email">Email Log</a>
	</div>`

	html := app.RenderHTMLForRequest("Admin", "Admin Dashboard", content, r)
	w.Write([]byte(html))
}

// UsersHandler shows and manages users
func UsersHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	_, acc, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// Handle POST requests for user management actions
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		action := r.FormValue("action")
		userID := r.FormValue("user_id")

		if userID == "" {
			app.BadRequest(w, r, "User ID required")
			return
		}

		targetUser, err := auth.GetAccount(userID)
		if err != nil {
			app.NotFound(w, r, "User not found")
			return
		}

		switch action {
		case "toggle_admin":
			targetUser.Admin = !targetUser.Admin
			auth.UpdateAccount(targetUser)
		case "delete":
			if err := auth.DeleteAccount(userID); err != nil {
				http.Error(w, "Failed to delete user", http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	// GET request - show user list
	users := auth.GetAllAccounts()

	// Sort users by created date (newest first)
	sort.Slice(users, func(i, j int) bool {
		return users[i].Created.After(users[j].Created)
	})

	content := `<p><a href="/admin">← Admin</a></p>
	<h2>Users</h2>
	<p>Total: ` + fmt.Sprintf("%d", len(users)) + `</p>
	<table class="admin-table">
		<thead>
			<tr>
				<th>Username</th>
				<th>Name</th>
				<th class="created-col">Created</th>
				<th class="center">Admin</th>
				<th class="center">Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, user := range users {
		createdStr := user.Created.Format("2006-01-02")

		// Don't allow deleting yourself
		deleteButton := ""
		if user.ID != acc.ID {
			deleteButton = `<form method="POST" class="d-inline" onsubmit="return confirm('Delete user ` + user.ID + `?');">
				<input type="hidden" name="action" value="delete">
				<input type="hidden" name="user_id" value="` + user.ID + `">
				<button type="submit" class="btn-danger">Delete</button>
			</form>`
		}

		content += `
			<tr>
				<td><strong><a href="/@` + user.ID + `">` + user.ID + `</a></strong></td>
				<td>` + user.Name + `</td>
				<td class="created-col">` + createdStr + `</td>
				<td class="center">
					<form method="POST" class="d-inline">
						<input type="hidden" name="action" value="toggle_admin">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<input type="checkbox" ` + func() string {
			if user.Admin {
				return "checked"
			}
			return ""
		}() + ` onchange="this.form.submit()" class="cursor-pointer" style="width: 18px; height: 18px;">
					</form>
				</td>
				<td class="center">
					` + deleteButton + `
				</td>
			</tr>`
	}

	content += `
		</tbody>
	</table>`

	html := app.RenderHTMLForRequest("Admin", "Users", content, r)
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

	<div class="blacklist-section">
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
