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
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil || !acc.Admin {
		http.Error(w, "Forbidden - Admin access required", http.StatusForbidden)
		return
	}

	// Handle POST requests for user management actions
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		action := r.FormValue("action")
		userID := r.FormValue("user_id")

		if userID == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		targetUser, err := auth.GetAccount(userID)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		switch action {
		case "toggle_admin":
			targetUser.Admin = !targetUser.Admin
			auth.UpdateAccount(targetUser)
		case "toggle_member":
			targetUser.Member = !targetUser.Member
			auth.UpdateAccount(targetUser)
		case "delete":
			if err := auth.DeleteAccount(userID); err != nil {
				http.Error(w, "Failed to delete user", http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	// GET request - show user list
	users := auth.GetAllAccounts()

	// Sort users by created date (newest first)
	sort.Slice(users, func(i, j int) bool {
		return users[i].Created.After(users[j].Created)
	})

	content := `<h2>User Management</h2>
	<p>Total Users: ` + fmt.Sprintf("%d", len(users)) + `</p>
	<style>
		.admin-table { width: 100%; border-collapse: collapse; }
		.admin-table th { text-align: left; padding: 10px; border-bottom: 2px solid #ddd; }
		.admin-table td { padding: 10px; border-bottom: 1px solid #eee; }
		.admin-table .center { text-align: center; }
		@media only screen and (max-width: 600px) {
			.admin-table { font-size: 0.85em; }
			.admin-table th, .admin-table td { padding: 5px 3px; }
			.admin-table .created-col { display: none; }
			.admin-table .delete-btn { padding: 3px 6px !important; font-size: 0.8em !important; }
		}
	</style>
	<table class="admin-table">
		<thead>
			<tr>
				<th>Username</th>
				<th>Name</th>
				<th class="created-col">Created</th>
				<th class="center">Admin</th>
				<th class="center">Member</th>
				<th class="center">Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, user := range users {
		createdStr := user.Created.Format("2006-01-02")

		// Don't allow deleting yourself
		deleteButton := ""
		if user.ID != acc.ID {
			deleteButton = `<form method="POST" style="display: inline;" onsubmit="return confirm('Delete user ` + user.ID + `?');">
				<input type="hidden" name="action" value="delete">
				<input type="hidden" name="user_id" value="` + user.ID + `">
				<button type="submit" class="delete-btn" style="background: #dc3545; color: white; border: none; padding: 5px 10px; border-radius: 3px; cursor: pointer;">Delete</button>
			</form>`
		}

		content += `
			<tr>
				<td><strong><a href="/@` + user.ID + `" style="color: inherit; text-decoration: none;">` + user.ID + `</a></strong></td>
				<td>` + user.Name + `</td>
				<td class="created-col">` + createdStr + `</td>
				<td class="center">
					<form method="POST" style="display: inline;">
						<input type="hidden" name="action" value="toggle_admin">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<input type="checkbox" ` + func() string {
			if user.Admin {
				return "checked"
			}
			return ""
		}() + ` onchange="this.form.submit()" style="cursor: pointer; width: 18px; height: 18px;">
					</form>
				</td>
				<td class="center">
					<form method="POST" style="display: inline;">
						<input type="hidden" name="action" value="toggle_member">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<input type="checkbox" ` + func() string {
			if user.Member {
				return "checked"
			}
			return ""
		}() + ` onchange="this.form.submit()" style="cursor: pointer; width: 18px; height: 18px;">
					</form>
				</td>
				<td class="center">
					` + deleteButton + `
				</td>
			</tr>`
	}

	content += `
		</tbody>
	</table>
	<br>
	<p><a href="/admin/blocklist">Mail Blocklist</a> | <a href="/moderate">Moderation Queue</a></p>`

	html := app.RenderHTMLForRequest("Admin", "User Management", content, r)
	w.Write([]byte(html))
}

// BlocklistHandler shows and manages the mail blocklist
func BlocklistHandler(w http.ResponseWriter, r *http.Request) {
	// Check if user is admin
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil || !acc.Admin {
		http.Error(w, "Forbidden - Admin access required", http.StatusForbidden)
		return
	}

	// Handle POST requests for blocklist actions
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
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
	
	<style>
		.blocklist-section { margin: 20px 0; }
		.blocklist-table { width: 100%; border-collapse: collapse; margin: 10px 0; }
		.blocklist-table th { text-align: left; padding: 10px; border-bottom: 2px solid #ddd; }
		.blocklist-table td { padding: 10px; border-bottom: 1px solid #eee; }
		.block-form { margin: 20px 0; padding: 15px; background: #f5f5f5; border-radius: 5px; }
		.block-form input[type="text"] { padding: 8px; width: 300px; max-width: 100%; }
		.block-form button { padding: 8px 15px; background: #dc3545; color: white; border: none; border-radius: 3px; cursor: pointer; margin-left: 10px; }
		.unblock-btn { background: #28a745; color: white; border: none; padding: 5px 10px; border-radius: 3px; cursor: pointer; }
	</style>

	<div class="blocklist-section">
		<h3>Blocked Emails (` + fmt.Sprintf("%d", len(bl.Emails)) + `)</h3>
		<div class="block-form">
			<form method="POST">
				<input type="hidden" name="action" value="block_email">
				<input type="text" name="email" placeholder="email@example.com or *@domain.com" required>
				<button type="submit">Block Email</button>
			</form>
			<p style="font-size: 0.9em; color: #666; margin-top: 5px;">Use *@domain.com to block entire domain</p>
		</div>`

	if len(bl.Emails) > 0 {
		content += `<table class="blacklist-table">
			<thead>
				<tr>
					<th>Email</th>
					<th style="width: 100px; text-align: center;">Action</th>
				</tr>
			</thead>
			<tbody>`

		for _, email := range bl.Emails {
			content += `
				<tr>
					<td><code>` + email + `</code></td>
					<td style="text-align: center;">
						<form method="POST" style="display: inline;">
							<input type="hidden" name="action" value="unblock_email">
							<input type="hidden" name="email" value="` + email + `">
							<button type="submit" class="unblock-btn">Unblock</button>
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
					<th style="width: 100px; text-align: center;">Action</th>
				</tr>
			</thead>
			<tbody>`

		for _, ip := range bl.IPs {
			content += `
				<tr>
					<td><code>` + ip + `</code></td>
					<td style="text-align: center;">
						<form method="POST" style="display: inline;">
							<input type="hidden" name="action" value="unblock_ip">
							<input type="hidden" name="ip" value="` + ip + `">
							<button type="submit" class="unblock-btn">Unblock</button>
						</form>
					</td>
				</tr>`
		}

		content += `</tbody></table>`
	} else {
		content += `<p>No blocked IPs</p>`
	}

	content += `</div>
	<div style="margin-top: 30px;">
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
