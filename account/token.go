package account

import (
	"encoding/json"
	"errors"
	"fmt"
	htmlpkg "html"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/sshaccess"
)

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

	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodGet && r.URL.Path == "/token" && !app.WantsJSON(r) {
		target := "/account/tokens"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	// Credential creation requires a verified or explicitly approved account.
	if r.Method == http.MethodPost {
		r.ParseForm()
		creating := r.URL.Query().Get("create_client") == "1" || (r.FormValue("_method") != "DELETE" && r.FormValue("removekey") == "")
		if creating {
			if err := auth.CheckCredentialAccess(acc.ID); err != nil {
				app.Forbidden(w, r, err.Error())
				return
			}
			if err := auth.CheckPostRate(acc.ID); err != nil {
				app.TooManyRequests(w, r, err.Error())
				return
			}
		}
	}
	if r.Method == http.MethodPost && (r.FormValue("sshkey") != "" || r.FormValue("removekey") != "") {
		if !auth.ValidCSRF(r) {
			app.Forbidden(w, r, "Reopen API access and try again.")
			return
		}
		var keyErr error
		if key := r.FormValue("sshkey"); key != "" {
			_, keyErr = sshaccess.Register(acc.ID, key, r.FormValue("keyname"))
		} else {
			keyErr = auth.RemoveSSHKey(acc.ID, r.FormValue("removekey"))
		}
		if keyErr != nil {
			app.RespondError(w, http.StatusBadRequest, keyErr.Error())
			return
		}
		http.Redirect(w, r, "/account/tokens", http.StatusSeeOther)
		return
	}
	// Handle OAuth client actions
	if r.Method == "POST" {
		r.ParseForm()
		if r.URL.Query().Get("create_client") == "1" {
			createOAuthClient(w, r, acc.ID)
			return
		}
		if clientID := r.URL.Query().Get("delete_client"); clientID != "" && r.FormValue("_method") == "DELETE" {
			if !auth.StrictCSRF(r) {
				app.Forbidden(w, r, "Reload and try again.")
				return
			}
			if err := auth.DeleteOAuthClient(clientID, acc.ID); err != nil {
				app.Forbidden(w, r, err.Error())
				return
			}
			http.Redirect(w, r, "/account/tokens", http.StatusSeeOther)
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

	var sb strings.Builder
	sb.WriteString(Navigation("/account") + `<div class="account-access">`)

	// API credentials contains tokens and registered OAuth clients. Tokens come
	// first because they are the usual reason to open this page.
	sb.WriteString(`<h2>API tokens</h2>`)
	sb.WriteString(`<p class="text-secondary text-sm"><a href="/developers">API and MCP reference</a> · <a href="/account/billing">Billing</a></p>`)

	sb.WriteString(`<div id="token-result" class="success-panel d-none">`)
	sb.WriteString(`<strong>Token Created</strong><p>Copy this token now — you won't see it again:</p>`)
	sb.WriteString(`<pre id="new-token" ></pre></div>`)

	// Keep permissions and dates readable at every width.
	sb.WriteString(`<table class="data-table stacked credential-table"><thead><tr><th>Name</th><th>Access</th><th>Created</th><th>Last used</th><th>Expires</th><th></th></tr></thead><tbody>`)
	tokens := auth.ListTokens(accountID)
	var developerTokens []*auth.Token
	for _, token := range tokens {
		if !appPassword(token) {
			developerTokens = append(developerTokens, token)
		}
	}
	tokens = developerTokens
	if len(tokens) == 0 {
		sb.WriteString(`<tr><td colspan="6">No API tokens yet.</td></tr>`)
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
		sb.WriteString(fmt.Sprintf(`<tr><td data-label="Name">%s</td><td data-label="Access">%s</td><td data-label="Created">%s</td><td data-label="Last used">%s</td><td data-label="Expires">%s</td><td>
			<form method="POST" action="/account/tokens?id=%s" class="form-action d-inline" onsubmit="return confirm('Delete?')">
			<input type="hidden" name="_method" value="DELETE">%s<button type="submit" class="text-sm">Revoke</button></form></td></tr>`,
			htmlpkg.EscapeString(token.Name), tokenScope(token), created, lastUsed, expires, token.ID, app.CSRFField(auth.CSRFToken(r))))
	}
	sb.WriteString(`</tbody></table>`)

	// Every field says what it is.
	//
	// The form was a heading and then three unlabelled controls, so "Create
	// Token" was sitting directly above the name box doing a field label's job
	// — read as one, it is the wrong word. A placeholder is not a label either:
	// it disappears the moment you type, and "e.g. CI/CD" over an empty box is
	// the only thing that ever said what the box was for.
	agentAccess := r.URL.Query().Get("access") != "services"
	serviceAccess := r.URL.Query().Get("access") == "services"
	checked := ""
	if agentAccess {
		checked = " checked"
	}
	sb.WriteString(`<h4 class="mt-5">Create a token</h4>`)
	sb.WriteString(`<form id="create-token-form" class="form" onsubmit="createToken(event)">`)
	sb.WriteString(app.Field{
		Name: "name", Label: "Name", Placeholder: "e.g. My script", Required: true, Wide: true,
	}.HTML())
	sb.WriteString(app.Field{Name: "client", Label: "Access", Options: []app.Option{{Value: "api", Label: "Assistant API / MCP", On: agentAccess}, {Value: "services", Label: "Selected services API / MCP", On: serviceAccess}}}.HTML())
	sb.WriteString(`<fieldset class="scope-fields" data-token-access="api" hidden><legend>API capabilities</legend><div class="choices"><label class="choice"><input type="checkbox" name="capability" value="api:agent"` + checked + `>Agents</label><label class="choice"><input type="checkbox" name="capability" value="api:inbox">Inbox</label><label class="choice"><input type="checkbox" name="capability" value="api:work">Background jobs</label></div><p class="text-muted text-sm">Agent access can run any of your account’s agents with their configured tools; it is not limited to one named agent. Choose Services instead to restrict a client to specific capabilities.</p><label class="choice"><input type="checkbox" name="api_write"` + checked + `>Allow actions (required to ask agents or start jobs)</label></fieldset>`)
	sb.WriteString(`<fieldset class="scope-fields" data-token-access="services" hidden><legend>Allowed services</legend><p class="text-muted text-sm">Only selected services are accessible, including their actions. This does not grant agent execution or Inbox API access.</p><div class="choices">`)
	for _, spec := range service.Specs() {
		sb.WriteString(`<label class="choice"><input type="checkbox" name="services" value="` + htmlpkg.EscapeString(spec.Name) + `">` + htmlpkg.EscapeString(spec.NavLabel()) + `</label>`)
	}
	sb.WriteString(`</div></fieldset>`)
	sb.WriteString(app.Field{
		Name: "expires_in", Label: "Expires", Options: []app.Option{
			{Value: "0", Label: "Never"},
			{Value: "7", Label: "7 days"},
			{Value: "30", Label: "30 days"},
			{Value: "90", Label: "90 days", On: true},
			{Value: "365", Label: "1 year"},
		},
	}.HTML())

	sb.WriteString(`<div class="form-actions"><button type="submit">Create token</button></div></form>`)

	sb.WriteString(`<p>For mail and chat apps, use <a href="/account/connections">App passwords</a>.</p>`)
	sb.WriteString(oauthClients(r, accountID))
	sb.WriteString(sshaccess.Card(r, accountID, "/account/tokens", "SSH and SFTP", "SSH and SFTP use an SSH key, not an access token. Add your public key below. Use sftp in place of ssh and -P in place of -p to connect to files.", "ssh"))

	// ForRequest, not RenderHTML: the latter hard-codes a nil account, so every
	// part of the chrome that depends on knowing who is signed in — the nav,
	// the account menu, the balance — went missing on a page you can only
	// reach by being signed in. Same bug /account had.
	app.Respond(w, r, app.Response{Title: "API access", Description: "API tokens and developer connections", HTML: sb.String() + `</div>`})
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
	var scopeMode string
	var access string
	var client string
	var expiresIn int // days

	if app.SendsJSON(r) {
		var req struct {
			Access      string   `json:"access"`
			Client      string   `json:"client"`
			ScopeMode   string   `json:"scope_mode"`
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
		scopeMode = req.ScopeMode
		access = req.Access
		client = req.Client
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}
		name = strings.TrimSpace(r.FormValue("name"))
		permissions = parseTokenPermissions(r.FormValue("permissions"))
		expiresIn = parseTokenExpiresIn(r.FormValue("expires_in"))
		scope = r.Form["services"]
		scopeMode = r.FormValue("scope_mode")
		access = r.FormValue("access")
		client = r.FormValue("client")
	}

	// Validate
	if name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	if scopeMode != "" && scopeMode != "all" && scopeMode != "select" {
		app.RespondError(w, http.StatusBadRequest, "Choose All or Select")
		return
	}
	if scopeMode == "all" {
		scope = nil
	}
	if access == "services" && scopeMode == "all" {
		for _, sp := range service.Specs() {
			scope = append(scope, sp.Name)
		}
		scopeMode = "select"
	}
	validScope := validScopeNames(scope)
	if (scopeMode == "select" || len(scope) > 0) && len(validScope) == 0 {
		app.RespondError(w, http.StatusBadRequest, "Select at least one valid service")
		return
	}

	// Default permissions for legacy callers; API selections are explicit.
	if len(permissions) == 0 && client != "api" {
		permissions = []string{"read", "write"}
	}

	// The scope, if one was chosen. Written into the token's permissions as
	// service:<name>, which is the one form the MCP boundary enforces — so a
	// token made here is confined exactly as one made on /agents is, and the
	// tools/list it reads is its own rather than the whole instance.
	if named := auth.ScopeFor(validScope); len(named) > 0 {
		permissions = append(permissions, named...)
	}

	if client != "" {
		requested := permissions
		permissions = []string{"read", "write"}
		switch client {
		case "mail":
			permissions = append(permissions, "protocol:mail")
		case "chat":
			permissions = append(permissions, "protocol:chat")
		case "both":
			permissions = append(permissions, "protocol:mail", "protocol:chat")
		case "api":
			permissions = []string{"read"}
			selected := 0
			for _, p := range requested {
				switch p {
				case "api:agent", "api:inbox", "api:work":
					permissions = append(permissions, p)
					selected++
				case "write":
					permissions = append(permissions, p)
				}
			}
			if selected == 0 {
				app.RespondError(w, http.StatusBadRequest, "Select at least one API capability")
				return
			}
		case "services":
			if len(validScope) == 0 {
				app.RespondError(w, http.StatusBadRequest, "Select at least one service")
				return
			}
			permissions = append(permissions, auth.ScopeFor(validScope)...)
		default:
			app.RespondError(w, http.StatusBadRequest, "Choose a valid access type")
			return
		}
	}

	// Calculate expiration
	var expiresAt time.Time
	if expiresIn > 0 {
		expiresAt = time.Now().AddDate(0, 0, expiresIn)
	}

	// Create the token
	token, rawToken, err := auth.CreateToken(accountID, name, permissions, expiresAt)
	if errors.Is(err, auth.ErrCredentialLimit) {
		app.TooManyRequests(w, r, err.Error())
		return
	}
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
		http.Redirect(w, r, "/account/tokens", http.StatusSeeOther)
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

// tokenScope is what a token may reach, for the list.
//
// The service names it was confined to, or the truth when it was not: an
// unscoped token reaches everything the account can, which is the thing worth
// saying out loud on the page that hands out credentials.
func tokenScope(t *auth.Token) string {
	var clients []string
	for _, permission := range t.Permissions {
		switch permission {
		case "protocol:mail":
			clients = append(clients, "Mail (IMAP/SMTP)")
		case "protocol:chat":
			clients = append(clients, "Chat (XMPP)")
		}
	}
	if len(clients) > 0 {
		return strings.Join(clients, ", ")
	}
	names := t.Services()
	if len(names) == 0 && t.Scoped() {
		var capabilities []string
		for _, p := range t.Permissions {
			if strings.HasPrefix(p, "api:") {
				capabilities = append(capabilities, strings.TrimPrefix(p, "api:"))
			}
		}
		if len(capabilities) == 0 {
			return "None"
		}
		suffix := " (read only)"
		if t.HasPermission("write") {
			suffix = " (read and actions)"
		}
		return htmlpkg.EscapeString(strings.Join(capabilities, ", ") + suffix)
	}
	if len(names) == 0 {
		return "All"
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, service.Label(n))
	}
	return htmlpkg.EscapeString(strings.Join(out, ", "))
}
