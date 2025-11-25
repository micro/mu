package admin

import (
	"fmt"
	"net/http"
	"sort"

	"mu/app"
	"mu/auth"
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
				<td><strong>` + user.ID + `</strong></td>
				<td>` + user.Name + `</td>
				<td class="created-col">` + createdStr + `</td>
				<td class="center">
					<form method="POST" style="display: inline;">
						<input type="hidden" name="action" value="toggle_admin">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<input type="checkbox" ` + func() string { if user.Admin { return "checked" }; return "" }() + ` onchange="this.form.submit()" style="cursor: pointer; width: 18px; height: 18px;">
					</form>
				</td>
				<td class="center">
					<form method="POST" style="display: inline;">
						<input type="hidden" name="action" value="toggle_member">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<input type="checkbox" ` + func() string { if user.Member { return "checked" }; return "" }() + ` onchange="this.form.submit()" style="cursor: pointer; width: 18px; height: 18px;">
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
	<p><a href="/moderate">Moderation Queue</a></p>`

	html := app.RenderHTML("Admin", "User Management", content)
	w.Write([]byte(html))
}
