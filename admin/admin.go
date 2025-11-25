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
	<table style="width: 100%; border-collapse: collapse;">
		<thead>
			<tr>
				<th style="text-align: left; padding: 10px; border-bottom: 2px solid #ddd;">Username</th>
				<th style="text-align: left; padding: 10px; border-bottom: 2px solid #ddd;">Name</th>
				<th style="text-align: left; padding: 10px; border-bottom: 2px solid #ddd;">Created</th>
				<th style="text-align: center; padding: 10px; border-bottom: 2px solid #ddd;">Admin</th>
				<th style="text-align: center; padding: 10px; border-bottom: 2px solid #ddd;">Member</th>
				<th style="text-align: center; padding: 10px; border-bottom: 2px solid #ddd;">Actions</th>
			</tr>
		</thead>
		<tbody>`

	for _, user := range users {
		adminBadge := ""
		if user.Admin {
			adminBadge = "✓"
		}
		
		memberBadge := ""
		if user.Member {
			memberBadge = "✓"
		}

		createdStr := user.Created.Format("2006-01-02")
		
		// Don't allow deleting yourself
		deleteButton := ""
		if user.ID != acc.ID {
			deleteButton = `<form method="POST" style="display: inline;" onsubmit="return confirm('Delete user ` + user.ID + `?');">
				<input type="hidden" name="action" value="delete">
				<input type="hidden" name="user_id" value="` + user.ID + `">
				<button type="submit" style="background: #dc3545; color: white; border: none; padding: 5px 10px; border-radius: 3px; cursor: pointer;">Delete</button>
			</form>`
		}

		content += `
			<tr>
				<td style="padding: 10px; border-bottom: 1px solid #eee;"><strong>` + user.ID + `</strong></td>
				<td style="padding: 10px; border-bottom: 1px solid #eee;">` + user.Name + `</td>
				<td style="padding: 10px; border-bottom: 1px solid #eee;">` + createdStr + `</td>
				<td style="text-align: center; padding: 10px; border-bottom: 1px solid #eee;">
					<form method="POST" style="display: inline;">
						<input type="hidden" name="action" value="toggle_admin">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<button type="submit" style="border: none; background: none; cursor: pointer; font-size: 1.2em;">` + adminBadge + `</button>
					</form>
				</td>
				<td style="text-align: center; padding: 10px; border-bottom: 1px solid #eee;">
					<form method="POST" style="display: inline;">
						<input type="hidden" name="action" value="toggle_member">
						<input type="hidden" name="user_id" value="` + user.ID + `">
						<button type="submit" style="border: none; background: none; cursor: pointer; font-size: 1.2em;">` + memberBadge + `</button>
					</form>
				</td>
				<td style="text-align: center; padding: 10px; border-bottom: 1px solid #eee;">
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
