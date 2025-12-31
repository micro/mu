package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mu/auth"
)

// TokenHandler manages Personal Access Tokens (PATs)
// GET /token - List all tokens for the authenticated user
// POST /token - Create a new token
// DELETE /token?id={id} - Delete a token
func TokenHandler(w http.ResponseWriter, r *http.Request) {
	// Must be authenticated via session (not PAT)
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// PAT tokens can't manage other PAT tokens (must use session)
	if sess.Type != "account" {
		http.Error(w, "PAT tokens cannot manage other tokens. Please use session authentication.", http.StatusForbidden)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case "GET":
		// Check if JSON API request
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			handleListTokensJSON(w, r, acc.ID)
		} else {
			handleTokenPage(w, r, acc.ID)
		}
	case "POST":
		handleCreateToken(w, r, acc.ID)
	case "DELETE":
		handleDeleteToken(w, r, acc.ID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTokenPage(w http.ResponseWriter, r *http.Request, accountID string) {
	tokens := auth.ListTokens(accountID)

	var tokenRows string
	for _, token := range tokens {
		expiresStr := "Never"
		if !token.ExpiresAt.IsZero() {
			expiresStr = TimeAgo(token.ExpiresAt)
		}
		lastUsedStr := "Never"
		if !token.LastUsed.IsZero() {
			lastUsedStr = TimeAgo(token.LastUsed)
		}

		perms := strings.Join(token.Permissions, ", ")

		tokenRows += fmt.Sprintf(`
			<tr>
				<td style="padding: 12px; border-bottom: 1px solid #eee;">%s</td>
				<td style="padding: 12px; border-bottom: 1px solid #eee;">%s</td>
				<td style="padding: 12px; border-bottom: 1px solid #eee;">%s</td>
				<td style="padding: 12px; border-bottom: 1px solid #eee;">%s</td>
				<td style="padding: 12px; border-bottom: 1px solid #eee;">
					<form method="POST" action="/token?id=%s" style="display: inline;" onsubmit="return confirm('Delete this token?');">
						<input type="hidden" name="_method" value="DELETE">
						<button type="submit" style="background: #dc3545; color: white; border: none; padding: 5px 10px; cursor: pointer; border-radius: 3px;">Delete</button>
					</form>
				</td>
			</tr>`,
			token.Name,
			perms,
			lastUsedStr,
			expiresStr,
			token.ID,
		)
	}

	if tokenRows == "" {
		tokenRows = `<tr><td colspan="5" style="padding: 20px; text-align: center; color: #666;">No tokens yet. Create one below.</td></tr>`
	}

	content := fmt.Sprintf(`
		<div style="max-width: 900px;">
			<h2>API Tokens</h2>
			<p>Personal Access Tokens (PATs) for API authentication. Use with <code>Authorization: Bearer TOKEN</code> header.</p>
			
			<div id="token-result" style="display: none; margin: 20px 0; padding: 15px; background: #d4edda; border: 1px solid #c3e6cb; border-radius: 5px;">
				<strong>✓ Token Created!</strong>
				<p style="margin: 10px 0;">Copy this token now - you won't see it again:</p>
				<div style="background: #f8f9fa; padding: 10px; border-radius: 3px; font-family: monospace; word-break: break-all;" id="new-token"></div>
			</div>

			<h3 style="margin-top: 30px;">Your Tokens</h3>
			<table style="width: 100%%; border-collapse: collapse; margin: 20px 0;">
				<thead>
					<tr style="background: #f8f9fa;">
						<th style="padding: 12px; text-align: left; border-bottom: 2px solid #dee2e6;">Name</th>
						<th style="padding: 12px; text-align: left; border-bottom: 2px solid #dee2e6;">Permissions</th>
						<th style="padding: 12px; text-align: left; border-bottom: 2px solid #dee2e6;">Last Used</th>
						<th style="padding: 12px; text-align: left; border-bottom: 2px solid #dee2e6;">Expires</th>
						<th style="padding: 12px; text-align: left; border-bottom: 2px solid #dee2e6;">Actions</th>
					</tr>
				</thead>
				<tbody>
					%s
				</tbody>
			</table>

			<h3 style="margin-top: 40px;">Create New Token</h3>
			<form id="create-token-form" onsubmit="createToken(event)" style="background: #f8f9fa; padding: 20px; border-radius: 5px; margin: 20px 0;">
				<div style="margin-bottom: 15px;">
					<label style="display: block; margin-bottom: 5px; font-weight: bold;">Token Name *</label>
					<input type="text" name="name" required placeholder="e.g., CI/CD Pipeline" 
					       style="width: 100%%; padding: 8px; border: 1px solid #ccc; border-radius: 3px;">
					<small style="color: #666;">Descriptive name to identify this token</small>
				</div>
				
				<div style="margin-bottom: 15px;">
					<label style="display: block; margin-bottom: 5px; font-weight: bold;">Expiration</label>
					<select name="expires_in" style="width: 100%%; padding: 8px; border: 1px solid #ccc; border-radius: 3px;">
						<option value="0">Never</option>
						<option value="7">7 days</option>
						<option value="30">30 days</option>
						<option value="90" selected>90 days</option>
						<option value="365">1 year</option>
					</select>
				</div>
				
				<button type="submit" style="background: #007bff; color: white; border: none; padding: 10px 20px; cursor: pointer; border-radius: 3px; font-size: 16px;">
					Generate Token
				</button>
			</form>

			<p style="margin-top: 20px;"><a href="/account">← Back to Account</a> · <a href="/api">API Docs</a></p>
		</div>

		<script>
		async function createToken(e) {
			e.preventDefault();
			const form = e.target;
			const data = {
				name: form.name.value,
				expires_in: parseInt(form.expires_in.value),
				permissions: ['read', 'write']
			};

			const res = await fetch('/token', {
				method: 'POST',
				headers: {'Content-Type': 'application/json'},
				body: JSON.stringify(data)
			});

			const result = await res.json();
			if (result.success) {
				document.getElementById('new-token').textContent = result.token;
				document.getElementById('token-result').style.display = 'block';
				setTimeout(() => location.reload(), 5000);
			} else {
				alert('Failed to create token');
			}
		}
		</script>
	`, tokenRows)

	html := RenderHTML("API Tokens", "Manage your API tokens", content)
	w.Write([]byte(html))
}

func handleListTokensJSON(w http.ResponseWriter, r *http.Request, accountID string) {
	tokens := auth.ListTokens(accountID)

	// Don't expose the actual token values
	type TokenInfo struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		Created     time.Time `json:"created"`
		LastUsed    time.Time `json:"last_used"`
		ExpiresAt   time.Time `json:"expires_at,omitempty"`
		Permissions []string  `json:"permissions"`
	}

	var response []TokenInfo
	for _, token := range tokens {
		response = append(response, TokenInfo{
			ID:          token.ID,
			Name:        token.Name,
			Created:     token.Created,
			LastUsed:    token.LastUsed,
			ExpiresAt:   token.ExpiresAt,
			Permissions: token.Permissions,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tokens": response,
	})
}

func handleCreateToken(w http.ResponseWriter, r *http.Request, accountID string) {
	isJSON := strings.Contains(r.Header.Get("Content-Type"), "application/json")

	var name string
	var permissions []string
	var expiresIn int // days

	if isJSON {
		var req struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
			ExpiresIn   int      `json:"expires_in"` // days, 0 = never
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(req.Name)
		permissions = req.Permissions
		expiresIn = req.ExpiresIn
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(r.FormValue("name"))
		permStr := r.FormValue("permissions")
		if permStr != "" {
			permissions = strings.Split(permStr, ",")
			for i := range permissions {
				permissions[i] = strings.TrimSpace(permissions[i])
			}
		}
		// Parse expires_in from form
		if exp := r.FormValue("expires_in"); exp != "" {
			var err error
			_, err = time.Parse("2006-01-02", exp)
			if err == nil {
				expiresIn = 365 // Default to 1 year if date format provided
			}
		}
	}

	// Validate
	if name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	// Default permissions if none provided
	if len(permissions) == 0 {
		permissions = []string{"read", "write"}
	}

	// Calculate expiration
	var expiresAt time.Time
	if expiresIn > 0 {
		expiresAt = time.Now().AddDate(0, 0, expiresIn)
	}

	// Create the token
	token, rawToken, err := auth.CreateToken(accountID, name, permissions, expiresAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the token info including the raw token (only time it's shown)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"id":          token.ID,
		"name":        token.Name,
		"token":       rawToken, // Only returned once!
		"created":     token.Created,
		"expires_at":  token.ExpiresAt,
		"permissions": token.Permissions,
		"message":     "Save this token now. You won't be able to see it again!",
	})
}

func handleDeleteToken(w http.ResponseWriter, r *http.Request, accountID string) {
	tokenID := r.URL.Query().Get("id")
	if tokenID == "" {
		http.Error(w, "Token ID required", http.StatusBadRequest)
		return
	}

	err := auth.DeleteToken(tokenID, accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Token deleted successfully",
	})
}
