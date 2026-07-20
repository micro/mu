package app

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/auth"
	"mu/internal/settings"
)

// Google OAuth (Mu as an OAuth *client* of Google) — distinct from the /oauth/*
// routes, where Mu is an OAuth *server* for MCP clients. Secrets are read from
// settings/env at request time and never embedded or logged.

var oauthHTTP = &http.Client{Timeout: 12 * time.Second}

func googleClientID() string     { return strings.TrimSpace(settings.Get("GOOGLE_CLIENT_ID")) }
func googleClientSecret() string { return strings.TrimSpace(settings.Get("GOOGLE_CLIENT_SECRET")) }

// GoogleConfigured reports whether Google sign-in is available.
func GoogleConfigured() bool { return googleClientID() != "" && googleClientSecret() != "" }

func googleRedirectURI(r *http.Request) string {
	if v := strings.TrimSpace(settings.Get("GOOGLE_REDIRECT_URI")); v != "" {
		return v
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/oauth2/callback"
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = crand.Read(b)
	return hex.EncodeToString(b)
}

func requestSecure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// GoogleLogin starts sign-in via Google (find-or-create an account).
func GoogleLogin(w http.ResponseWriter, r *http.Request) { startGoogle(w, r, false) }

// GoogleConnect links Google to the *current* logged-in account so they can
// sign in with Google afterwards. Requires a session.
func GoogleConnect(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireSession(r); err != nil {
		RedirectToLogin(w, r)
		return
	}
	startGoogle(w, r, true)
}

// startGoogle sets the CSRF state cookie (and, in link mode, a g_link cookie so
// the callback knows to link rather than sign in) and bounces to Google.
func startGoogle(w http.ResponseWriter, r *http.Request, link bool) {
	if !GoogleConfigured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	state := randToken(16)
	secure := requestSecure(r)
	http.SetCookie(w, &http.Cookie{
		Name: "g_state", Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	linkVal := ""
	linkAge := -1
	if link {
		linkVal, linkAge = "1", 600
	}
	http.SetCookie(w, &http.Cookie{
		Name: "g_link", Value: linkVal, Path: "/", MaxAge: linkAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{}
	q.Set("client_id", googleClientID())
	q.Set("redirect_uri", googleRedirectURI(r))
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("access_type", "online")
	q.Set("prompt", "select_account")
	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode(), http.StatusSeeOther)
}

// GoogleCallback handles Google's redirect: verify state, exchange the code,
// fetch the user, find-or-create their account, and start a session.
func GoogleCallback(w http.ResponseWriter, r *http.Request) {
	if !GoogleConfigured() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// CSRF: the state cookie set at /oauth2/google must match the query param.
	sc, err := r.Cookie("g_state")
	if err != nil || sc.Value == "" || sc.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid sign-in state, please try again", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "g_state", Value: "", Path: "/", MaxAge: -1})

	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	token, err := googleExchange(r.URL.Query().Get("code"), googleRedirectURI(r))
	if err != nil {
		Log("auth", "google token exchange failed: %v", err)
		http.Error(w, "Google sign-in failed, please try again", http.StatusBadGateway)
		return
	}
	info, err := googleUserInfo(token)
	if err != nil || info.Email == "" {
		Log("auth", "google userinfo failed: %v", err)
		http.Error(w, "Google sign-in failed, please try again", http.StatusBadGateway)
		return
	}

	// Link mode: attach this Google identity to the current account instead of
	// signing in / creating one.
	if c, cerr := r.Cookie("g_link"); cerr == nil && c.Value == "1" {
		http.SetCookie(w, &http.Cookie{Name: "g_link", Value: "", Path: "/", MaxAge: -1})
		linkGoogleToCurrentAccount(w, r, info)
		return
	}

	acc := findOrCreateGoogleAccount(info)
	if acc == nil {
		http.Error(w, "Could not create your account", http.StatusInternalServerError)
		return
	}
	if acc.Banned {
		http.Error(w, "This account is not available", http.StatusForbidden)
		return
	}

	sess, err := auth.CreateSession(acc.ID)
	if err != nil {
		http.Error(w, "Session error, please try again", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: sess.Token, Path: "/", MaxAge: 2592000,
		HttpOnly: true, Secure: requestSecure(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/home", http.StatusFound)
}

// googleExchange trades an authorization code for an access token.
func googleExchange(code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", googleClientID())
	form.Set("client_secret", googleClientSecret())
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	req, _ := http.NewRequest(http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var t struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.AccessToken == "" {
		return "", fmt.Errorf("no access token (%s)", t.Error)
	}
	return t.AccessToken, nil
}

type googleUser struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// googleUserInfo fetches the signed-in user's profile from the OIDC endpoint.
func googleUserInfo(accessToken string) (*googleUser, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var u googleUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}

// findOrCreateGoogleAccount links by email, or provisions a new account with a
// username derived from the email. Google users have a random secret (they sign
// in via Google, not a password).
func findOrCreateGoogleAccount(info *googleUser) *auth.Account {
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if acc, err := auth.GetAccountByEmail(email); err == nil && acc != nil {
		return acc
	}
	id := uniqueUsernameFromEmail(email)
	name := strings.TrimSpace(info.Name)
	if name == "" {
		name = id
	}
	err := auth.Create(&auth.Account{
		ID:              id,
		Name:            name,
		Secret:          randToken(24),
		Email:           email,
		EmailVerified:   true,
		EmailVerifiedAt: time.Now(),
		Created:         time.Now(),
	})
	if err != nil {
		Log("auth", "google account create failed: %v", err)
		return nil
	}
	acc, _ := auth.GetAccount(id)
	return acc
}

// uniqueUsernameFromEmail builds a valid, unused username from an email address.
func uniqueUsernameFromEmail(email string) string {
	local := email
	if i := strings.IndexByte(email, '@'); i > 0 {
		local = email[:i]
	}
	var b strings.Builder
	for _, c := range strings.ToLower(local) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteByte(byte(c))
		}
	}
	base := strings.TrimLeft(b.String(), "0123456789_")
	if len(base) < 4 {
		base += "user"
	}
	if len(base) > 20 {
		base = base[:20]
	}
	if base == "" {
		base = "user"
	}
	candidate := base
	for i := 0; i < 10000; i++ {
		if i > 0 {
			candidate = fmt.Sprintf("%s%d", base, i)
			if len(candidate) > 24 {
				candidate = candidate[:24]
			}
		}
		if _, err := auth.GetAccount(candidate); err == nil {
			continue // taken
		}
		if reason := auth.ValidateUsername(candidate); reason != "" {
			continue // reserved/invalid
		}
		return candidate
	}
	return base + randToken(2)
}

// linkGoogleToCurrentAccount attaches the Google identity to the logged-in
// account by setting its verified email — so future Google sign-ins resolve
// here. Refuses if another account already owns that email.
func linkGoogleToCurrentAccount(w http.ResponseWriter, r *http.Request, info *googleUser) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		RedirectToLogin(w, r)
		return
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if other, e := auth.GetAccountByEmail(email); e == nil && other != nil && other.ID != acc.ID {
		http.Error(w, "That Google account ("+email+") is already linked to another Mu account (@"+other.ID+"). Delete or unlink that account first, then connect.", http.StatusConflict)
		return
	}
	acc.Email = email
	acc.EmailVerified = true
	acc.EmailVerifiedAt = time.Now()
	if err := auth.UpdateAccount(acc); err != nil {
		Log("auth", "google link failed for %s: %v", acc.ID, err)
		http.Error(w, "Could not link your Google account, please try again", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account?linked=google", http.StatusFound)
}

// googleGlyph is the multicolour Google "G" mark.
func googleGlyph() string {
	return `<svg width="18" height="18" viewBox="0 0 48 48" style="vertical-align:middle;flex:none"><path fill="#4285F4" d="M45.1 24.5c0-1.6-.1-3.1-.4-4.5H24v8.5h11.8c-.5 2.7-2 5-4.4 6.6v5.5h7.1c4.1-3.8 6.6-9.4 6.6-16.1z"/><path fill="#34A853" d="M24 46c5.9 0 10.9-2 14.5-5.3l-7.1-5.5c-2 1.3-4.5 2.1-7.4 2.1-5.7 0-10.5-3.8-12.2-9h-7.3v5.7C8 40.3 15.4 46 24 46z"/><path fill="#FBBC05" d="M11.8 28.3c-.4-1.3-.7-2.7-.7-4.3s.3-3 .7-4.3v-5.7H4.5C3 17.1 2 20.4 2 24s1 6.9 2.5 10l7.3-5.7z"/><path fill="#EA4335" d="M24 10.7c3.2 0 6.1 1.1 8.4 3.3l6.3-6.3C34.9 4.1 29.9 2 24 2 15.4 2 8 7.7 4.5 14l7.3 5.7c1.7-5.2 6.5-9 12.2-9z"/></svg>`
}

// googleButtonHTML returns the "Continue with Google" button, or "" when Google
// sign-in isn't configured.
func googleButtonHTML(text string) string {
	if !GoogleConfigured() {
		return ""
	}
	return `<a href="/oauth2/google" style="display:flex;align-items:center;justify-content:center;gap:10px;border:1px solid #ddd;border-radius:6px;padding:10px 12px;text-decoration:none;color:#111;font-weight:600;margin:0 0 14px">` + googleGlyph() + `<span>` + text + `</span></a>
<div style="text-align:center;color:#999;font-size:13px;margin:0 0 14px">or</div>`
}

// renderGoogleCard shows the Google link state on the account page and a Connect
// button. Shown only when Google sign-in is configured.
func renderGoogleCard(acc *auth.Account) string {
	if !GoogleConfigured() {
		return ""
	}
	if acc.EmailVerified && acc.Email != "" {
		return `<div class="card"><h4>Google</h4><p>You can sign in with Google using <strong>` + htmlpkg.EscapeString(acc.Email) + `</strong>.</p></div>`
	}
	return `<div class="card"><h4>Connect Google</h4>
<p class="text-sm text-muted">Link Google so you can sign in with it next time. This just sets your verified email — it doesn't change your username or password.</p>
<a href="/oauth2/google/connect" style="display:inline-flex;align-items:center;gap:8px;border:1px solid #ddd;border-radius:6px;padding:8px 14px;text-decoration:none;color:#111;font-weight:600;margin-top:8px">` + googleGlyph() + ` Connect Google</a>
</div>`
}

// loginPage renders the login template with the Google button injected above
// the form (when configured). Mirrors the fmt.Sprintf(LoginTemplate, ...) shape.
func loginPage(redirectParam, errHTML string) string {
	html := fmt.Sprintf(LoginTemplate, redirectParam, errHTML)
	if btn := googleButtonHTML("Continue with Google"); btn != "" {
		html = strings.Replace(html, `<h1>Login</h1>`, `<h1>Login</h1>`+btn, 1)
	}
	return html
}
