package account

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
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
		app.Unauthorized(w, r)
		return
	}

	// PAT tokens can't manage other PAT tokens (must use session)
	if sess.Type != "account" {
		app.Forbidden(w, r, "PAT tokens cannot manage other tokens. Please use session authentication.")
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
			// Where this client will receive its authorization code.
			//
			// The form asked for a name and nothing else, and registered the
			// client with no address at all — so every client made here was a
			// dead record: there was nowhere a code could correctly be sent.
			// Nothing noticed, because the flow did not read the registry. Five
			// of them accumulated on micro.mu before the check that reads it
			// made them visible.
			redirect := strings.TrimSpace(r.FormValue("redirect_uri"))
			if redirect == "" {
				redirect = "http://localhost:0/callback"
			}
			if !auth.RegisterableRedirect(redirect) {
				app.BadRequest(w, r, "The redirect URL must be https, or http on localhost: "+redirect)
				return
			}
			client := auth.RegisterOAuthClient(acc.ID, name, []string{redirect})
			// Store credentials in session flash (not URL)
			setFlash(sess.ID, "client_id", client.ClientID)
			setFlash(sess.ID, "client_secret", client.ClientSecret)
			http.Redirect(w, r, "/token?created=1", http.StatusSeeOther)
			return
		}
		if clientID := r.URL.Query().Get("delete_client"); clientID != "" && r.FormValue("_method") == "DELETE" {
			auth.DeleteOAuthClient(clientID, acc.ID) //nolint:errcheck — a refused delete redirects to a list that still shows it
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
	// Yours, and only yours.
	//
	// This asked for every client on the instance. Anyone signed in saw the
	// names other people's MCP clients had registered under, their client ids
	// and when they appeared — with a Delete beside each that worked. A client
	// that registered itself at /oauth/register has no owner to compare
	// against, so it belongs to nobody and appears here for nobody.
	sb.WriteString(`<h3>OAuth Clients</h3>`)
	sb.WriteString(`<p style="color:#666;font-size:13px">For connecting Claude, MCP clients, or ` +
		`other apps via OAuth 2.1. Clients that register themselves when they connect do not ` +
		`appear here — they belong to no account, and nothing needs doing about them.</p>`)

	if newClientID != "" {
		sb.WriteString(fmt.Sprintf(`<div style="margin:15px 0;padding:15px;background:#d4edda;border:1px solid #c3e6cb;border-radius:6px;overflow:hidden">
			<strong>Client Created</strong>
			<p>Copy these now — the secret won't be shown again.</p>
			<p><strong>Client ID:</strong><br><code style="font-size:12px;word-break:break-all">%s</code></p>
			<p><strong>Client Secret:</strong><br><code style="font-size:12px;word-break:break-all">%s</code></p>
		</div>`, newClientID, newClientSecret))
	}

	sb.WriteString(`<table class="token-table"><thead><tr><th>Name</th><th>Client ID</th><th>Created</th><th></th></tr></thead><tbody>`)
	oauthClients := auth.OAuthClientsFor(accountID)
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
	// The address is half of what a client is. Without it there is nowhere a
	// code may be sent, and a client registered without one can never complete
	// a sign-in — which is what every client made on this form used to be.
	sb.WriteString(`<div style="margin-bottom:10px"><input type="text" name="redirect_uri" ` +
		`placeholder="Redirect URL, e.g. https://example.com/callback" style="width:100%;box-sizing:border-box"></div>`)
	sb.WriteString(`<p style="color:#666;font-size:12px;margin:0 0 10px">Where the client receives ` +
		`its code. Must be https, or http on localhost. Left empty it is ` +
		`<code>http://localhost:0/callback</code>, which suits a command-line or desktop client.</p>`)
	sb.WriteString(`<button type="submit">Create Client</button></form>`)

	sb.WriteString(`<hr style="margin:30px 0;border:none;border-top:1px solid #eee">`)

	// === Personal Access Tokens ===
	sb.WriteString(`<h3>Personal Access Tokens</h3>`)
	sb.WriteString(`<p style="color:#666;font-size:13px">For API authentication. Use with <code>Authorization: Bearer TOKEN</code> header.</p>`)

	sb.WriteString(`<div id="token-result" style="display:none;margin:20px 0;padding:15px;background:#d4edda;border:1px solid #c3e6cb;border-radius:5px">`)
	sb.WriteString(`<strong>Token Created</strong><p>Copy this token now — you won't see it again:</p>`)
	sb.WriteString(`<pre id="new-token" style="background:#fff;padding:10px;border:1px solid #c3e6cb;border-radius:3px;overflow-x:auto;white-space:pre-wrap;word-break:break-all"></pre></div>`)

	// Created, beside Last Used.
	//
	// The table showed a name and three dates, none of them the one people
	// reach for: "when did this appear". So a token whose Last Used had just
	// moved read as a token that had just been issued, which is an alarming
	// thing to misread about a credential.
	sb.WriteString(`<table class="token-table"><thead><tr><th>Name</th><th>Permissions</th><th>Created</th><th>Last Used</th><th>Expires</th><th></th></tr></thead><tbody>`)
	tokens := auth.ListTokens(accountID)
	if len(tokens) == 0 {
		sb.WriteString(`<tr><td colspan="6" style="padding:20px;text-align:center;color:#666">No tokens yet.</td></tr>`)
	}
	for _, token := range tokens {
		expires := "Never"
		if !token.ExpiresAt.IsZero() {
			expires = app.TimeAgo(token.ExpiresAt)
		}
		lastUsed := "Never"
		if !token.LastUsed.IsZero() {
			lastUsed = app.TimeAgo(token.LastUsed)
		}
		created := "Unknown"
		if !token.Created.IsZero() {
			created = app.TimeAgo(token.Created)
		}
		sb.WriteString(fmt.Sprintf(`<tr><td data-label="Name">%s</td><td data-label="Permissions">%s</td><td data-label="Created">%s</td><td data-label="Last Used">%s</td><td data-label="Expires">%s</td><td>
			<form method="POST" action="/token?id=%s" style="display:inline" onsubmit="return confirm('Delete?')">
			<input type="hidden" name="_method" value="DELETE"><button type="submit" style="font-size:13px">Delete</button></form></td></tr>`,
			token.Name, strings.Join(token.Permissions, ", "), created, lastUsed, expires, token.ID))
	}
	sb.WriteString(`</tbody></table>`)

	sb.WriteString(`<h4 style="margin-top:20px">Create Token</h4>`)
	sb.WriteString(`<form id="create-token-form" onsubmit="createToken(event)">`)
	sb.WriteString(`<div style="margin-bottom:10px"><input type="text" name="name" required placeholder="e.g. CI/CD"></div>`)
	sb.WriteString(`<div style="margin-bottom:10px"><select name="expires_in">`)
	sb.WriteString(`<option value="0">Never</option><option value="7">7 days</option><option value="30">30 days</option>`)
	sb.WriteString(`<option value="90" selected>90 days</option><option value="365">1 year</option></select></div>`)

	// What it may reach, on the page that hands out the credential.
	//
	// This page had a name and an expiry and nothing else, so every token
	// created here carried the whole account: eighty-odd tools, the mail, the
	// wallet. The scoped path existed on /agents and the README pointed here —
	// so the documented road was the unsafe one and the safe one was
	// undocumented. Same control, same meaning, on both pages now.
	sb.WriteString(`<div id="tok-scope"><p class="tok-scope-head">What may it reach?</p>`)
	sb.WriteString(`<p class="tok-scope-sub">Choose nothing and it reaches everything you can — ` +
		`which is rarely what you meant for a credential you are about to paste somewhere.</p>`)
	sb.WriteString(`<div class="tok-chips">`)
	for _, sp := range tokenScopeChoices() {
		sb.WriteString(`<label class="tok-chip"><input type="checkbox" name="services" value="` +
			htmlpkg.EscapeString(sp.Name) + `"><span>` + htmlpkg.EscapeString(sp.NavLabel()) + `</span></label>`)
	}
	sb.WriteString(`</div></div>`)

	sb.WriteString(`<button type="submit">Generate Token</button></form>`)
	sb.WriteString(tokenScopeCSS)

	sb.WriteString(`<p style="margin-top:20px"><a href="/account">← Account</a> · <a href="/mcp">MCP server</a></p>`)

	sb.WriteString(`<script>
async function createToken(e) {
	e.preventDefault();
	var form = e.target;
	var res = await fetch('/token', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({name: form.name.value, expires_in: parseInt(form.expires_in.value),
			permissions: ['read', 'write'],
			services: Array.from(form.querySelectorAll('input[name="services"]:checked')).map(function(c){return c.value})})
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

	// ForRequest, not RenderHTML: the latter hard-codes a nil account, so every
	// part of the chrome that depends on knowing who is signed in — the nav,
	// the account menu, the balance — went missing on a page you can only
	// reach by being signed in. Same bug /account had.
	html := app.RenderHTMLForRequest("API Credentials", "Manage API credentials", sb.String(), r)
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

func parseTokenPermissions(permStr string) []string {
	if permStr == "" {
		return nil
	}

	parts := strings.Split(permStr, ",")
	permissions := make([]string, 0, len(parts))
	for _, part := range parts {
		perm := strings.TrimSpace(part)
		if perm != "" {
			permissions = append(permissions, perm)
		}
	}
	return permissions
}

func parseTokenExpiresIn(exp string) int {
	exp = strings.TrimSpace(exp)
	if exp == "" {
		return 0
	}

	if days, err := strconv.Atoi(exp); err == nil {
		if days > 0 {
			return days
		}
		return 0
	}

	if _, err := time.Parse("2006-01-02", exp); err == nil {
		return 365
	}

	return 0
}

func handleCreateToken(w http.ResponseWriter, r *http.Request, accountID string) {
	var name string
	var permissions []string
	var scope []string
	var expiresIn int // days

	if app.SendsJSON(r) {
		var req struct {
			Name        string   `json:"name"`
			Services    []string `json:"services"`
			Permissions []string `json:"permissions"`
			ExpiresIn   int      `json:"expires_in"` // days, 0 = never
		}
		if err := app.DecodeJSON(r, &req); err != nil {
			app.RespondError(w, http.StatusBadRequest, "invalid json")
			return
		}
		name = strings.TrimSpace(req.Name)
		permissions = req.Permissions
		expiresIn = req.ExpiresIn
		scope = req.Services
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(r.FormValue("name"))
		permissions = parseTokenPermissions(r.FormValue("permissions"))
		expiresIn = parseTokenExpiresIn(r.FormValue("expires_in"))
		scope = r.Form["services"]
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

	// The scope, if one was chosen. Written into the token's permissions as
	// service:<name>, which is the one form the MCP boundary enforces — so a
	// token made here is confined exactly as one made on /agents is, and the
	// tools/list it reads is its own rather than the whole instance.
	if named := auth.ScopeFor(validScopeNames(scope)); len(named) > 0 {
		permissions = append(permissions, named...)
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

// tokenScopeChoices is every service a token can be confined to, by label.
func tokenScopeChoices() []service.Spec {
	specs := service.Specs()
	sort.Slice(specs, func(i, j int) bool { return specs[i].NavLabel() < specs[j].NavLabel() })
	return specs
}

// validScopeNames drops anything that is not a registered service, so a stray
// value cannot widen a scope or create an entry that matches nothing.
func validScopeNames(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range in {
		n = strings.ToLower(strings.TrimSpace(n))
		if n == "" || seen[n] {
			continue
		}
		if _, known := service.SpecFor(n); !known {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

const tokenScopeCSS = `<style>
#tok-scope{border-top:1px solid #eee;padding-top:12px;margin:14px 0}
.tok-scope-head{font-size:13px;font-weight:600;margin:0 0 2px}
.tok-scope-sub{font-size:12px;color:#999;margin:0 0 8px;max-width:520px}
.tok-chips{display:flex;flex-wrap:wrap;gap:6px}
.tok-chip input{position:absolute;opacity:0;width:0;height:0}
.tok-chip span{display:block;border:1px solid #ddd;border-radius:999px;padding:5px 12px;
  cursor:pointer;font-size:13px;color:#444;white-space:nowrap}
.tok-chip span:hover{border-color:#bbb}
.tok-chip input:checked+span{background:#111;border-color:#111;color:#fff}
.tok-chip input:focus-visible+span{outline:2px solid #111;outline-offset:2px}
</style>`
