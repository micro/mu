package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
)

// OAuthClient represents a registered OAuth client (e.g. Claude Code).
type OAuthClient struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	CreatedAt    time.Time `json:"created_at"`
}

// OAuthCode represents a pending authorization code.
type OAuthCode struct {
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

// RegisterOAuthClient creates a new OAuth client.
func RegisterOAuthClient(name string, redirectURIs []string) *OAuthClient {
	oauthMu.Lock()
	defer oauthMu.Unlock()

	id := generateRandomString(24)
	secret := generateRandomString(48)

	client := &OAuthClient{
		ClientID:     id,
		ClientSecret: secret,
		Name:         name,
		RedirectURIs: redirectURIs,
		CreatedAt:    time.Now(),
	}
	oauthClients[id] = client
	saveOAuthClients()
	return client
}

// GetOAuthClient returns a client by ID.
func GetOAuthClient(clientID string) *OAuthClient {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	return oauthClients[clientID]
}

// GetAllOAuthClients returns all registered clients.
func GetAllOAuthClients() []*OAuthClient {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	var list []*OAuthClient
	for _, c := range oauthClients {
		list = append(list, c)
	}
	return list
}

// DeleteOAuthClient removes a client.
func DeleteOAuthClient(clientID string) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	delete(oauthClients, clientID)
	saveOAuthClients()
}

// CreateAuthorizationCode creates a code for the OAuth flow.
func CreateAuthorizationCode(clientID, accountID, redirectURI, codeChallenge, codeChallengeMethod string) string {
	code := generateRandomString(32)

	oauthMu.Lock()
	oauthCodes[code] = &OAuthCode{
		Code:                code,
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

	// Create a session token for this account
	sess, err := CreateSession(authCode.AccountID)
	if err != nil {
		return "", err
	}
	return sess.Token, nil
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

// OAuthMetadataHandler serves /.well-known/oauth-authorization-server
func OAuthMetadataHandler(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "mu.xyz") {
		scheme = "http"
	}
	issuer := scheme + "://" + r.Host

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"issuer":                             issuer,
		"authorization_endpoint":             issuer + "/oauth/authorize",
		"token_endpoint":                     issuer + "/oauth/token",
		"registration_endpoint":              issuer + "/oauth/register",
		"scopes_supported":                   []string{"read", "write"},
		"response_types_supported":           []string{"code"},
		"grant_types_supported":              []string{"authorization_code"},
		"code_challenge_methods_supported":    []string{"S256", "plain"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

// OAuthResourceHandler serves /.well-known/oauth-protected-resource
func OAuthResourceHandler(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && !strings.Contains(r.Host, "mu.xyz") {
		scheme = "http"
	}
	issuer := scheme + "://" + r.Host

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"resource":                issuer,
		"authorization_servers":   []string{issuer},
		"scopes_supported":       []string{"read", "write"},
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

	client := RegisterOAuthClient(req.ClientName, req.RedirectURIs)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"client_id":                client.ClientID,
		"client_secret":            client.ClientSecret,
		"client_name":              client.Name,
		"redirect_uris":            client.RedirectURIs,
		"grant_types":              []string{"authorization_code"},
		"response_types":           []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// OAuthAuthorizeHandler handles GET /oauth/authorize — shows login form or redirects.
func OAuthAuthorizeHandler(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	codeChallenge := r.URL.Query().Get("code_challenge")
	codeChallengeMethod := r.URL.Query().Get("code_challenge_method")

	if clientID == "" {
		http.Error(w, "client_id required", 400)
		return
	}

	// Check if already logged in
	sess, _ := TrySession(r)
	if sess != nil {
		// Already authenticated — issue code immediately
		code := CreateAuthorizationCode(clientID, sess.Account, redirectURI, codeChallenge, codeChallengeMethod)
		redirect := redirectURI + "?code=" + code
		if state != "" {
			redirect += "&state=" + state
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	// Show login form
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Authorize</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{font-family:'Nunito Sans',sans-serif;max-width:400px;margin:50px auto;padding:0 20px}
h2{margin-bottom:4px}
p{color:#666;font-size:14px}
input{width:100%%;padding:10px;margin:6px 0;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box;font-family:inherit}
button{width:100%%;padding:10px;background:#000;color:#fff;border:none;border-radius:6px;font-size:14px;cursor:pointer;font-family:inherit;margin-top:8px}
.error{color:#c00;font-size:13px}
</style></head><body>
<h2>Authorize</h2>
<p>Sign in to grant access to your account.</p>
<form method="POST" action="/oauth/authorize">
<input type="hidden" name="client_id" value="%s">
<input type="hidden" name="redirect_uri" value="%s">
<input type="hidden" name="state" value="%s">
<input type="hidden" name="code_challenge" value="%s">
<input type="hidden" name="code_challenge_method" value="%s">
<input type="text" name="username" placeholder="Username" required autofocus>
<input type="password" name="password" placeholder="Password" required>
<button type="submit">Sign In & Authorize</button>
</form>
</body></html>`,
		clientID, redirectURI, state, codeChallenge, codeChallengeMethod)
}

// OAuthAuthorizePostHandler handles POST /oauth/authorize — validates credentials and redirects.
func OAuthAuthorizePostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		OAuthAuthorizeHandler(w, r)
		return
	}

	r.ParseForm()
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Validate credentials
	_, err := Login(username, password)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>Authorize</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{font-family:'Nunito Sans',sans-serif;max-width:400px;margin:50px auto;padding:0 20px}
input{width:100%%;padding:10px;margin:6px 0;border:1px solid #ddd;border-radius:6px;font-size:14px;box-sizing:border-box;font-family:inherit}
button{width:100%%;padding:10px;background:#000;color:#fff;border:none;border-radius:6px;font-size:14px;cursor:pointer;font-family:inherit;margin-top:8px}
.error{color:#c00;font-size:13px}
</style></head><body>
<h2>Authorize</h2>
<p class="error">Invalid username or password.</p>
<form method="POST" action="/oauth/authorize">
<input type="hidden" name="client_id" value="%s">
<input type="hidden" name="redirect_uri" value="%s">
<input type="hidden" name="state" value="%s">
<input type="hidden" name="code_challenge" value="%s">
<input type="hidden" name="code_challenge_method" value="%s">
<input type="text" name="username" placeholder="Username" value="%s" required autofocus>
<input type="password" name="password" placeholder="Password" required>
<button type="submit">Sign In & Authorize</button>
</form>
</body></html>`,
			clientID, redirectURI, state, codeChallenge, codeChallengeMethod, username)
		return
	}

	// Issue authorization code
	code := CreateAuthorizationCode(clientID, username, redirectURI, codeChallenge, codeChallengeMethod)
	redirect := redirectURI + "?code=" + code
	if state != "" {
		redirect += "&state=" + state
	}
	http.Redirect(w, r, redirect, http.StatusFound)
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
