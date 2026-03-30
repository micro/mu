package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/auth"
)

// Flash storage — one-time values shown after redirect, then deleted.
var (
	flashMu   sync.Mutex
	flashData = map[string]string{} // "sessionID:key" → value
)

func setFlash(sessionID, key, value string) {
	flashMu.Lock()
	flashData[sessionID+":"+key] = value
	flashMu.Unlock()
}

func getFlash(sessionID, key string) string {
	flashMu.Lock()
	defer flashMu.Unlock()
	k := sessionID + ":" + key
	v := flashData[k]
	delete(flashData, k)
	return v
}

// TokenHandler manages Personal Access Tokens (PATs)
// GET /token - List all tokens for the authenticated user
// POST /token - Create a new token
// DELETE /token?id={id} - Delete a token
func TokenHandler(w http.ResponseWriter, r *http.Request) {
	// Must be authenticated via session (not PAT)
	sess, acc, err := auth.RequireSession(r)
	if err != nil {
		Unauthorized(w, r)
		return
	}

	// PAT tokens can't manage other PAT tokens (must use session)
	if sess.Type != "account" {
		Forbidden(w, r, "PAT tokens cannot manage other tokens. Please use session authentication.")
		return
	}

	// Handle OAuth client actions
	if r.Method == "POST" {
		r.ParseForm()
		if r.URL.Query().Get("create_client") == "1" {
			name := r.FormValue("client_name")
			if name == "" {
				name = "MCP Client"
			}
			client := auth.RegisterOAuthClient(name, []string{})
			// Store credentials in session flash (not URL)
			setFlash(sess.ID, "client_id", client.ClientID)
			setFlash(sess.ID, "client_secret", client.ClientSecret)
			http.Redirect(w, r, "/token?created=1", http.StatusSeeOther)
			return
		}
		if clientID := r.URL.Query().Get("delete_client"); clientID != "" && r.FormValue("_method") == "DELETE" {
			auth.DeleteOAuthClient(clientID)
			http.Redirect(w, r, "/token", http.StatusSeeOther)
			return
		}
		if r.FormValue("_method") == "DELETE" {
			handleDeleteToken(w, r, acc.ID)
			return
		}
	}

	switch r.Method {
	case "GET":
		// Check if JSON API request
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			handleListTokensJSON(w, r, acc.ID)
		} else {
			handleTokenPage(w, r, acc.ID, sess.ID)
		}
	case "POST":
		handleCreateToken(w, r, acc.ID)
	case "DELETE":
		handleDeleteToken(w, r, acc.ID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleTokenPage(w http.ResponseWriter, r *http.Request, accountID, sessionID string) {
	newClientID := getFlash(sessionID, "client_id")
	newClientSecret := getFlash(sessionID, "client_secret")

	var sb strings.Builder

	// Mobile-friendly table styles
	sb.WriteString(`<style>
.token-table { width:100%; border-collapse:collapse; font-size:14px; }
.token-table th { text-align:left; padding:8px; border-bottom:2px solid #eee; font-size:13px; color:#555; }
.token-table td { padding:8px; border-bottom:1px solid #f5f5f5; vertical-align:top; }
.token-table code { font-size:11px; word-break:break-all; }
@media (max-width: 640px) {
  .token-table thead { display:none; }
  .token-table tr { display:block; padding:12px 0; border-bottom:1px solid #eee; }
  .token-table td { display:block; padding:4px 0; border:none; }
  .token-table td:before { content:attr(data-label); font-weight:600; font-size:12px; color:#888; display:block; margin-bottom:2px; }
}
</style>`)

	// === OAuth Clients ===
	sb.WriteString(`<h3>OAuth Clients</h3>`)
	sb.WriteString(`<p style="color:#666;font-size:13px">For connecting Claude, MCP clients, or other apps via OAuth 2.1.</p>`)

	if newClientID != "" {
		sb.WriteString(fmt.Sprintf(`<div style="margin:15px 0;padding:15px;background:#d4edda;border:1px solid #c3e6cb;border-radius:6px;overflow:hidden">
			<strong>Client Created</strong>
			<p>Copy these now — the secret won't be shown again.</p>
			<p><strong>Client ID:</strong><br><code style="font-size:12px;word-break:break-all">%s</code></p>
			<p><strong>Client Secret:</strong><br><code style="font-size:12px;word-break:break-all">%s</code></p>
		</div>`, newClientID, newClientSecret))
	}

	sb.WriteString(`<table class="token-table"><thead><tr><th>Name</th><th>Client ID</th><th>Created</th><th></th></tr></thead><tbody>`)
	oauthClients := auth.GetAllOAuthClients()
	if len(oauthClients) == 0 {
		sb.WriteString(`<tr><td colspan="4" style="padding:20px;text-align:center;color:#666">No OAuth clients yet.</td></tr>`)
	}
	for _, c := range oauthClients {
		sb.WriteString(fmt.Sprintf(`<tr><td data-label="Name">%s</td><td data-label="Client ID"><code>%s</code></td><td data-label="Created">%s</td><td>
			<form method="POST" action="/token?delete_client=%s" style="display:inline" onsubmit="return confirm('Delete?')">
			<input type="hidden" name="_method" value="DELETE"><button type="submit" style="font-size:13px">Delete</button></form></td></tr>`,
			c.Name, c.ClientID, c.CreatedAt.Format("2 Jan 2006"), c.ClientID))
	}
	sb.WriteString(`</tbody></table>`)

	sb.WriteString(`<h4 style="margin-top:20px">Create OAuth Client</h4>`)
	sb.WriteString(`<form method="POST" action="/token?create_client=1">`)
	sb.WriteString(`<div style="margin-bottom:10px"><input type="text" name="client_name" placeholder="e.g. Claude" required></div>`)
	sb.WriteString(`<button type="submit">Create Client</button></form>`)

	sb.WriteString(`<hr style="margin:30px 0;border:none;border-top:1px solid #eee">`)

	// === Personal Access Tokens ===
	sb.WriteString(`<h3>Personal Access Tokens</h3>`)
	sb.WriteString(`<p style="color:#666;font-size:13px">For API authentication. Use with <code>Authorization: Bearer TOKEN</code> header.</p>`)

	sb.WriteString(`<div id="token-result" style="display:none;margin:20px 0;padding:15px;background:#d4edda;border:1px solid #c3e6cb;border-radius:5px">`)
	sb.WriteString(`<strong>Token Created</strong><p>Copy this token now — you won't see it again:</p>`)
	sb.WriteString(`<pre id="new-token" style="background:#fff;padding:10px;border:1px solid #c3e6cb;border-radius:3px;overflow-x:auto;white-space:pre-wrap;word-break:break-all"></pre></div>`)

	sb.WriteString(`<table class="token-table"><thead><tr><th>Name</th><th>Permissions</th><th>Last Used</th><th>Expires</th><th></th></tr></thead><tbody>`)
	tokens := auth.ListTokens(accountID)
	if len(tokens) == 0 {
		sb.WriteString(`<tr><td colspan="5" style="padding:20px;text-align:center;color:#666">No tokens yet.</td></tr>`)
	}
	for _, token := range tokens {
		expires := "Never"
		if !token.ExpiresAt.IsZero() {
			expires = TimeAgo(token.ExpiresAt)
		}
		lastUsed := "Never"
		if !token.LastUsed.IsZero() {
			lastUsed = TimeAgo(token.LastUsed)
		}
		sb.WriteString(fmt.Sprintf(`<tr><td data-label="Name">%s</td><td data-label="Permissions">%s</td><td data-label="Last Used">%s</td><td data-label="Expires">%s</td><td>
			<form method="POST" action="/token?id=%s" style="display:inline" onsubmit="return confirm('Delete?')">
			<input type="hidden" name="_method" value="DELETE"><button type="submit" style="font-size:13px">Delete</button></form></td></tr>`,
			token.Name, strings.Join(token.Permissions, ", "), lastUsed, expires, token.ID))
	}
	sb.WriteString(`</tbody></table>`)

	sb.WriteString(`<h4 style="margin-top:20px">Create Token</h4>`)
	sb.WriteString(`<form id="create-token-form" onsubmit="createToken(event)">`)
	sb.WriteString(`<div style="margin-bottom:10px"><input type="text" name="name" required placeholder="e.g. CI/CD"></div>`)
	sb.WriteString(`<div style="margin-bottom:10px"><select name="expires_in">`)
	sb.WriteString(`<option value="0">Never</option><option value="7">7 days</option><option value="30">30 days</option>`)
	sb.WriteString(`<option value="90" selected>90 days</option><option value="365">1 year</option></select></div>`)
	sb.WriteString(`<button type="submit">Generate Token</button></form>`)

	sb.WriteString(`<p style="margin-top:20px"><a href="/account">← Account</a> · <a href="/api">API Docs</a></p>`)

	sb.WriteString(`<script>
async function createToken(e) {
	e.preventDefault();
	var form = e.target;
	var res = await fetch('/token', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({name: form.name.value, expires_in: parseInt(form.expires_in.value), permissions: ['read', 'write']})
	});
	var result = await res.json();
	if (result.success) {
		document.getElementById('new-token').textContent = result.token;
		document.getElementById('token-result').style.display = 'block';
		setTimeout(function() { location.reload(); }, 5000);
	} else {
		alert('Failed to create token');
	}
}
</script>`)

	html := RenderHTML("API Credentials", "Manage API credentials", sb.String())
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
	var name string
	var permissions []string
	var expiresIn int // days

	if SendsJSON(r) {
		var req struct {
			Name        string   `json:"name"`
			Permissions []string `json:"permissions"`
			ExpiresIn   int      `json:"expires_in"` // days, 0 = never
		}
		if err := DecodeJSON(r, &req); err != nil {
			RespondError(w, http.StatusBadRequest, "invalid json")
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
	// Support both /token/{id} path style and /token?id={id} query style
	tokenID := strings.TrimPrefix(r.URL.Path, "/token/")
	if tokenID == "" || tokenID == r.URL.Path {
		tokenID = r.URL.Query().Get("id")
	}
	if tokenID == "" {
		http.Error(w, "Token ID required", http.StatusBadRequest)
		return
	}

	err := auth.DeleteToken(tokenID, accountID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Check if JSON request
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Token deleted successfully",
		})
	} else {
		// Redirect back to token page for form submission
		http.Redirect(w, r, "/token", http.StatusSeeOther)
	}
}
