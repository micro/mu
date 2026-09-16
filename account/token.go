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

	// Credential creation requires a verified or explicitly approved account.
	if r.Method == http.MethodPost {
		r.ParseForm()
		creating := r.URL.Query().Get("create_client") == "1" || r.FormValue("_method") != "DELETE"
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
	// Handle OAuth client actions
	if r.Method == "POST" {
		r.ParseForm()
		if r.URL.Query().Get("create_client") == "1" {
			http.NotFound(w, r)
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

	var sb strings.Builder

	// API credentials contains tokens and registered OAuth clients. Tokens come
	// first because they are the usual reason to open this page.
	sb.WriteString(`<h3>Client tokens</h3>`)
	sb.WriteString(`<p class="text-secondary text-sm">Use your username and a token as the password in your IMAP or XMPP client.</p>`)

	sb.WriteString(`<div id="token-result" class="success-panel d-none">`)
	sb.WriteString(`<strong>Token Created</strong><p>Copy this token now — you won't see it again:</p>`)
	sb.WriteString(`<pre id="new-token" ></pre></div>`)

	// Created, beside Last Used.
	//
	// The table showed a name and three dates, none of them the one people
	// reach for: "when did this appear". So a token whose Last Used had just
	// moved read as a token that had just been issued, which is an alarming
	// thing to misread about a credential.
	// "May reach", not "Permissions".
	//
	// The column read Permissions and rendered token.Permissions verbatim,
	// which is two different things in one field: a hardcoded ["read","write"]
	// the create form always sent and nothing on the instance enforces, and the
	// real scope, stored with a service: prefix — so the cell said
	// "read, write, service:news, service:markets" and the only settable half
	// was the one whose prefix was showing. Nothing could set the other half,
	// which is exactly what it looked like.
	// .data-table.stacked, not .token-table. There is no .token-table anywhere
	// in mu.css — it was a class name invented at the call site, so both tables
	// on this page were unstyled browser defaults, and on a phone six columns
	// squashed to a few characters each.
	sb.WriteString(`<table class="data-table stacked"><thead><tr><th>Name</th><th>Access</th><th>Created</th><th>Last Used</th><th>Expires</th><th></th></tr></thead><tbody>`)
	tokens := auth.ListTokens(accountID)
	if len(tokens) == 0 {
		sb.WriteString(`<tr><td colspan="6" class="p-5 text-center text-secondary">No tokens yet.</td></tr>`)
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
			<form method="POST" action="/token?id=%s" class="form-action d-inline" onsubmit="return confirm('Delete?')">
			<input type="hidden" name="_method" value="DELETE">%s<button type="submit" class="text-sm">Delete</button></form></td></tr>`,
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
	sb.WriteString(`<h4 class="mt-5">Create a token</h4>`)
	sb.WriteString(`<form id="create-token-form" class="form" onsubmit="createToken(event)">`)
	sb.WriteString(app.Field{
		Name: "name", Label: "Name", Placeholder: "e.g. CI/CD", Required: true, Wide: true,
	}.HTML())
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

	// ForRequest, not RenderHTML: the latter hard-codes a nil account, so every
	// part of the chrome that depends on knowing who is signed in — the nav,
	// the account menu, the balance — went missing on a page you can only
	// reach by being signed in. Same bug /account had.
	app.Respond(w, r, app.Response{Title: "Client access", Description: "Tokens for IMAP and XMPP clients", HTML: sb.String()})
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
	var expiresIn int // days

	if app.SendsJSON(r) {
		var req struct {
			Access      string   `json:"access"`
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

	// Default permissions if none provided
	if len(permissions) == 0 {
		permissions = []string{"read", "write"}
	}

	// The scope, if one was chosen. Written into the token's permissions as
	// service:<name>, which is the one form the MCP boundary enforces — so a
	// token made here is confined exactly as one made on /agents is, and the
	// tools/list it reads is its own rather than the whole instance.
	if named := auth.ScopeFor(validScope); len(named) > 0 {
		permissions = append(permissions, named...)
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

// tokenScope is what a token may reach, for the list.
//
// The service names it was confined to, or the truth when it was not: an
// unscoped token reaches everything the account can, which is the thing worth
// saying out loud on the page that hands out credentials.
func tokenScope(t *auth.Token) string {
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
		return htmlpkg.EscapeString(strings.Join(capabilities, ", "))
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
