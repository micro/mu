package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"mu/internal/service"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/internal/origin"

	"mu/internal/data"
)

// OAuthClient represents a registered OAuth client (e.g. Claude Code).
type OAuthClient struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
	Name         string    `json:"name"`
	RedirectURIs []string  `json:"redirect_uris"`
	CreatedAt    time.Time `json:"created_at"`

	// Account is who registered it here, and is empty for one that registered
	// itself at /oauth/register — dynamic registration is anonymous by
	// specification, so there is nobody to record.
	//
	// The field exists because there was no owner at all, and /token listed
	// every client on the instance to every signed-in person, with a working
	// Delete beside each. You could read the names other people's MCP clients
	// had chosen and remove their registrations.
	Account string `json:"account,omitempty"`
}

// OAuthCode represents a pending authorization code.
type OAuthCode struct {
	Permissions         []string
	Code                string
	ClientID            string
	AccountID           string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	ExpiresAt           time.Time
}

var (
	oauthMu      sync.Mutex
	oauthClients = map[string]*OAuthClient{}
	oauthCodes   = map[string]*OAuthCode{}
)

func init() {
	b, _ := data.LoadFile("oauth_clients.json")
	if len(b) > 0 {
		json.Unmarshal(b, &oauthClients)
	}
}

func saveOAuthClients() {
	data.SaveJSON("oauth_clients.json", oauthClients)
}

// RegisterOAuthClient creates a client owned by an account.
//
// account is empty only for anonymous dynamic registration, which has nobody to
// attribute it to. Everything that registers on somebody's behalf passes it, and
// what is unowned is what /token must never show.
func RegisterOAuthClient(account, name string, redirectURIs []string) *OAuthClient {
	oauthMu.Lock()
	defer oauthMu.Unlock()

	return registerOAuthClient(account, name, redirectURIs)
}

// RegisterOwnedOAuthClient atomically bounds clients created through account settings.
func RegisterOwnedOAuthClient(account, name string, redirectURIs []string) (*OAuthClient, error) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	count := 0
	for _, client := range oauthClients {
		if client.Account == account {
			count++
		}
	}
	if count >= 10 {
		return nil, ErrCredentialLimit
	}
	return registerOAuthClient(account, name, redirectURIs), nil
}

// Caller holds oauthMu.
func registerOAuthClient(account, name string, redirectURIs []string) *OAuthClient {
	id := generateRandomString(24)
	secret := generateRandomString(48)

	client := &OAuthClient{
		ClientID:     id,
		ClientSecret: secret,
		Name:         name,
		RedirectURIs: redirectURIs,
		CreatedAt:    time.Now(),
		Account:      account,
	}
	oauthClients[id] = client
	saveOAuthClients()
	return client
}

// OAuthClientsFor is one account's clients, newest first.
//
// This is what a person's own page asks for. It used to ask for all of them —
// see AllOAuthClients, which is now admin-only and named so that using it on a
// user's page reads as the mistake it is.
func OAuthClientsFor(account string) []*OAuthClient {
	if account == "" {
		return nil
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()
	var list []*OAuthClient
	for _, c := range oauthClients {
		if c.Account == account {
			list = append(list, c)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list
}

// GetOAuthClient returns one by id, or nil. The prefix stays because
// OAuthClient is the type — see the naming rules in AGENTS.md.
func GetOAuthClient(clientID string) *OAuthClient {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	return oauthClients[clientID]
}

// AllOAuthClients is every client on the instance, newest first — the operator's
// view, and nobody else's. Callers must be admin.
func AllOAuthClients() []*OAuthClient {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	var list []*OAuthClient
	for _, c := range oauthClients {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list
}

// DeleteOAuthClient removes one of an account's own clients.
//
// It took a client id and nothing else, so the Delete button beside every client
// on /token worked on every client on the instance. Ownership is the whole point
// of the second argument.
//
// An error rather than a bool, like DeleteToken beside it: a verb is an
// instruction, and an instruction read inside an if is the wrong shape — see
// TestAPredicateIsNotAnInstruction.
func DeleteOAuthClient(clientID, account string) error {
	if clientID == "" || account == "" {
		return errors.New("no such client")
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()
	c := oauthClients[clientID]
	if c == nil || c.Account != account {
		return errors.New("no such client")
	}
	delete(oauthClients, clientID)
	saveOAuthClients()
	return nil
}

// SetOAuthRedirects replaces a client's registered addresses.
//
// For the ones made before the form asked for an address, which is every client
// the /token form ever created: they hold a client_id somebody may have pasted
// into a config, so being able to give them an address is worth more than
// making them start again under a new id.
func SetOAuthRedirects(clientID string, uris []string) error {
	if len(uris) == 0 {
		return errors.New("a client needs somewhere to receive its code")
	}
	for _, u := range uris {
		if !RegisterableRedirect(u) {
			return errors.New("must be https, or http on a loopback address: " + u)
		}
	}
	oauthMu.Lock()
	defer oauthMu.Unlock()
	c := oauthClients[clientID]
	if c == nil {
		return ErrUnknownClient
	}
	c.RedirectURIs = uris
	saveOAuthClients()
	return nil
}

// ForceDeleteOAuthClient removes any client, owner or not. The operator's
// version, for the ones that registered themselves and belong to nobody.
func ForceDeleteOAuthClient(clientID string) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	delete(oauthClients, clientID)
	saveOAuthClients()
}

// CreateAuthorizationCode creates a code for the OAuth flow.
func CreateAuthorizationCode(clientID, accountID, redirectURI, codeChallenge, codeChallengeMethod string, permissions ...string) string {
	code := generateRandomString(32)

	oauthMu.Lock()
	oauthCodes[code] = &OAuthCode{
		Code:                code,
		Permissions:         append([]string(nil), permissions...),
		ClientID:            clientID,
		AccountID:           accountID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}
	oauthMu.Unlock()

	return code
}

// ExchangeAuthorizationCode exchanges a code for an access token.
// Validates PKCE code_verifier against the stored code_challenge.
func ExchangeAuthorizationCode(code, clientID, redirectURI, codeVerifier string) (string, error) {
	oauthMu.Lock()
	authCode, ok := oauthCodes[code]
	if ok {
		delete(oauthCodes, code) // one-time use
	}
	oauthMu.Unlock()

	if !ok {
		return "", errors.New("invalid authorization code")
	}
	if time.Now().After(authCode.ExpiresAt) {
		return "", errors.New("authorization code expired")
	}
	if authCode.ClientID != clientID {
		return "", errors.New("client_id mismatch")
	}
	if authCode.RedirectURI != redirectURI {
		return "", errors.New("redirect_uri mismatch")
	}

	// Validate PKCE
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			return "", errors.New("code_verifier required")
		}
		if !validatePKCE(codeVerifier, authCode.CodeChallenge, authCode.CodeChallengeMethod) {
			return "", errors.New("invalid code_verifier")
		}
	}

	if len(authCode.Permissions) == 0 {
		return "", errors.New("authorization has no approved permissions; reconnect")
	}
	_, raw, err := CreateToken(authCode.AccountID, "OAuth: "+clientID, authCode.Permissions, time.Now().Add(24*time.Hour))
	return raw, err
}

// validatePKCE checks the code_verifier against the code_challenge.
func validatePKCE(verifier, challenge, method string) bool {
	if method == "" || method == "S256" {
		h := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(h[:])
		return computed == challenge
	}
	if method == "plain" {
		return verifier == challenge
	}
	return false
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

func getIssuer(r *http.Request) string { return origin.URL(r) }

// OAuthMetadataHandler serves /.well-known/oauth-authorization-server
func OAuthMetadataHandler(w http.ResponseWriter, r *http.Request) {
	issuer := getIssuer(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/oauth/authorize",
		"token_endpoint":                        issuer + "/oauth/token",
		"registration_endpoint":                 issuer + "/oauth/register",
		"scopes_supported":                      []string{"read", "write"},
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256", "plain"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

// OAuthResourceHandler serves /.well-known/oauth-protected-resource
func OAuthResourceHandler(w http.ResponseWriter, r *http.Request) {
	issuer := getIssuer(r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resource":              issuer,
		"authorization_servers": []string{issuer},
		"scopes_supported":      []string{"read", "write"},
	})
}

// OAuthRegisterHandler handles dynamic client registration (POST /oauth/register).
func OAuthRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, 400)
		return
	}
	if req.ClientName == "" {
		req.ClientName = "MCP Client"
	}
	if len(req.RedirectURIs) == 0 {
		req.RedirectURIs = []string{"http://localhost:0/callback"}
	}
	// Localhost or HTTPS, which is what the MCP authorization spec requires.
	// An http:// address anywhere else is an authorization code travelling in
	// clear text, and accepting one now means enforcing it later would lock the
	// client out — better to refuse it while it is still the client's problem.
	for _, uri := range req.RedirectURIs {
		if !RegisterableRedirect(uri) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "invalid_redirect_uri",
				"error_description": "redirect_uris must be https, or http on a " +
					"loopback address: " + uri,
			})
			return
		}
	}

	// No owner: dynamic registration is anonymous by specification, so there is
	// nobody to attribute this to. That is exactly why it must not appear on
	// anybody's /token — see OAuthClient.Account.
	client := RegisterOAuthClient("", req.ClientName, req.RedirectURIs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":                  client.ClientID,
		"client_secret":              client.ClientSecret,
		"client_name":                client.Name,
		"redirect_uris":              client.RedirectURIs,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// oauthConsent renders explicit, session-bound approval using the shared stylesheet.
func oauthConsent(w http.ResponseWriter, r *http.Request, clientID, redirectURI string) {
	e := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><title>Connect to Micro</title><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/mu.css"></head><body><main><h1>Connect to Micro</h1>`)
	oauthMu.Lock()
	name := clientID
	if client := oauthClients[clientID]; client != nil {
		name = client.Name
	}
	oauthMu.Unlock()
	b.WriteString(`<p>Choose what <strong>` + e(name) + `</strong> may access. Access expires after 24 hours and can be revoked in <a href="/token">Client access</a>.</p><form method="POST" action="/oauth/authorize" class="form-col">`)
	values := map[string]string{"client_id": clientID, "redirect_uri": redirectURI, "state": r.URL.Query().Get("state"), "code_challenge": r.URL.Query().Get("code_challenge"), "code_challenge_method": r.URL.Query().Get("code_challenge_method"), "_csrf": CSRFToken(r)}
	for k, v := range values {
		b.WriteString(`<input type="hidden" name="` + k + `" value="` + e(v) + `">`)
	}
	b.WriteString(`<label>Access type<select name="access"><option value="services">Selected services</option><option value="api">Assistant API</option></select></label><fieldset><legend>Services</legend><p>Select only the services this client needs. These selections apply to Selected services access.</p><div class="choices">`)
	specs := service.Specs()
	sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
	for _, sp := range specs {
		b.WriteString(`<label class="choice"><input type="checkbox" name="service" value="` + e(sp.Name) + `">` + e(sp.Name) + `</label>`)
	}
	b.WriteString(`</div></fieldset><fieldset><legend>Assistant API</legend><p>These selections apply to Assistant API access. Agents can use their configured tools across your account.</p><div class="choices">`)
	for _, cap := range []string{"agent", "inbox", "work"} {
		b.WriteString(`<label class="choice"><input type="checkbox" name="capability" value="` + cap + `">` + cap + `</label>`)
	}
	b.WriteString(`</div></fieldset><label class="choice"><input type="checkbox" name="write" value="yes">Allow actions, including building apps and asking agents</label><p>Reading is allowed for the selected services or capabilities. Service access does not grant agent access.</p><div class="form-actions"><button type="submit">Allow access</button><a href="/">Cancel</a></div></form></main></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(b.String()))
}

func authorizationRedirect(redirectURI, code, state string) string {
	u, _ := url.Parse(redirectURI) // already validated against the registered client
	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func OAuthAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")

	if clientID == "" {
		http.Error(w, "client_id required", 400)
		return
	}

	// Where the code may be sent, decided here rather than taken from the
	// query string. This branch below issues one immediately to anybody who
	// already has a session, so an unchecked redirect_uri meant a single click
	// on a crafted link handed over that account's code. See oauth_redirect.go.
	redirectURI, err := RedirectFor(clientID, redirectURI)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Check if already logged in
	sess, _ := TrySession(r)
	if sess != nil {
		oauthConsent(w, r, clientID, redirectURI)
		return
	}

	// All sign-in methods return to this validated authorization request.
	http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
}

// OAuthAuthorizePostHandler handles POST /oauth/authorize — validates credentials and redirects.
func OAuthAuthorizePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		OAuthAuthorizeHandler(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	session, _ := TrySession(r)
	if session == nil {
		http.Error(w, "Sign in again", http.StatusUnauthorized)
		return
	}
	if !StrictCSRF(r) {
		http.Error(w, "Reopen the authorization page and try again", http.StatusForbidden)
		return
	}
	r.ParseForm()
	clientID := r.PostFormValue("client_id")
	redirectURI, err := RedirectFor(clientID, r.PostFormValue("redirect_uri"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	permissions := []string{"read"}
	if r.PostFormValue("write") == "yes" {
		permissions = append(permissions, "write")
	}
	selected := 0
	switch r.PostFormValue("access") {
	case "services":
		valid := map[string]bool{}
		for _, sp := range service.Specs() {
			valid[sp.Name] = true
		}
		for _, name := range r.PostForm["service"] {
			if !valid[name] {
				http.Error(w, "Unknown service", 400)
				return
			}
			permissions = append(permissions, ScopePrefix+name)
			selected++
		}
	case "api":
		for _, name := range r.PostForm["capability"] {
			if name != "agent" && name != "inbox" && name != "work" {
				http.Error(w, "Unknown capability", 400)
				return
			}
			permissions = append(permissions, "api:"+name)
			selected++
		}
	default:
		http.Error(w, "Choose an access type", 400)
		return
	}
	if selected == 0 {
		http.Error(w, "Select at least one service or capability for the chosen access type", 400)
		return
	}
	code := CreateAuthorizationCode(clientID, session.Account, redirectURI, r.PostFormValue("code_challenge"), r.PostFormValue("code_challenge_method"), permissions...)
	http.Redirect(w, r, authorizationRedirect(redirectURI, code, r.PostFormValue("state")), http.StatusSeeOther)
}

// OAuthTokenHandler handles POST /oauth/token — exchanges code for access token.
func OAuthTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}

	r.ParseForm()
	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	if grantType != "authorization_code" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
		return
	}

	token, err := ExchangeAuthorizationCode(code, clientID, redirectURI, codeVerifier)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant", "error_description": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   86400,
	})
}
