package account

// Granting Mu access to something in a Google account.
//
// Two jobs, deliberately in two places, because they are asked at different
// moments by people in different states of mind.
//
// The *ask* belongs at the point of need. Google calls this incremental
// authorisation, and it is right for a reason about people rather than
// protocol: the consent screen for "read your calendar" is a much bigger
// request than "know who you are", and a product that bundles them into the
// signup button trains everyone to decline at the door. So nobody is asked at
// signup. The invitation appears on /events when a calendar answer was
// computed without one, and on /contacts when a name could not be resolved.
// Somebody who never asks a calendar question never sees a calendar consent
// screen.
//
// The *inventory* belongs in one place. Once several of these exist, a
// disconnect button that lives only on the page that happens to use the grant
// is a permission nobody can audit — you would have to remember which services
// you had connected in order to find out which services you had connected. So
// /account lists everything granted, with a way to withdraw each. Ask where it
// is earned; review where it is expected.

import (
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/app"

	"mu/internal/auth"
	"mu/internal/google"
)

// grant describes one thing Mu can be given access to.
type grant struct {
	scope string
	ret   string // where to send the person afterwards
	verb  string // what to call it in a redirect status
}

// grants are the capabilities that can be asked for, keyed by the path segment
// that asks for them.
var grants = map[string]grant{
	"gmail":    {scope: google.GmailScope, ret: "/account", verb: "Gmail"},
	"drive":    {scope: google.DriveScope, ret: "/account", verb: "Drive"},
	"calendar": {scope: google.CalendarScope, ret: "/events", verb: "calendar"},
	"contacts": {scope: google.ContactsScope, ret: "/contacts", verb: "contacts"},
}

// GoogleGrantConnect starts a grant: /oauth2/google/<what>.
func GoogleGrantConnect(w http.ResponseWriter, r *http.Request) {
	what := strings.Trim(strings.TrimPrefix(r.URL.Path, "/oauth2/google/"), "/")
	g, ok := grants[what]
	if !ok {
		app.NotFound(w, r, "No such connection")
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !GoogleConfigured() {
		http.Redirect(w, r, g.ret, http.StatusSeeOther)
		return
	}

	http.SetCookie(w, &http.Cookie{Name: "g_owner", Value: acc.ID, Path: "/", MaxAge: 600, HttpOnly: true, Secure: requestSecure(r), SameSite: http.SameSiteLaxMode})
	state := randToken(16)
	secure := requestSecure(r)
	http.SetCookie(w, &http.Cookie{
		Name: "g_state", Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	// Tells the shared callback which grant came back, and so where to return
	// to. A cookie rather than the state parameter because it never leaves the
	// browser, and http-only so nothing on the page can rewrite the return.
	http.SetCookie(w, &http.Cookie{
		Name: "g_grant", Value: what, Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})

	q := url.Values{}
	q.Set("client_id", googleClientID())
	q.Set("redirect_uri", googleRedirectURI(r))
	q.Set("response_type", "code")
	q.Set("scope", "openid email "+g.scope)
	q.Set("state", state)
	// offline, because the point is answering "am I free Thursday" inside a
	// scheduled morning brief at 7am, when nobody is holding a browser open.
	q.Set("access_type", "offline")
	// consent, because Google returns a refresh token only on a fresh grant —
	// without this, connecting a second thing yields a token Mu cannot refresh,
	// and the connection dies an hour later.
	q.Set("prompt", "consent")
	// So that granting contacts does not cost somebody the calendar they
	// already attached.
	q.Set("include_granted_scopes", "true")

	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode(), http.StatusSeeOther)
}

// GoogleGrantDisconnect withdraws every grant. POST only: forgetting a grant is
// a state change, and the CSRF middleware guards the methods that make one.
//
// All-or-nothing because Google's revoke endpoint is: revoking the refresh
// token takes the whole grant with it, and there is no way to hand back one
// scope. Offering a per-scope button that quietly dropped everything would be
// worse than saying so.
func GoogleGrantDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid form token")
		return
	}
	google.Disconnect(acc.ID)

	ret := "/account"
	if v := strings.TrimSpace(r.FormValue("return")); strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") {
		ret = v
	}
	http.Redirect(w, r, ret+"?connection=disconnected", http.StatusSeeOther)
}

// finishGoogleGrant completes a grant from the shared callback.
func finishGoogleGrant(w http.ResponseWriter, r *http.Request, what, code string) {
	g, ok := grants[what]
	if !ok {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}

	owner, ownerErr := r.Cookie("g_owner")
	http.SetCookie(w, &http.Cookie{Name: "g_owner", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: requestSecure(r), SameSite: http.SameSiteLaxMode})
	if ownerErr != nil || owner.Value != acc.ID {
		app.Forbidden(w, r, "The account changed during Google connection. Please try again.")
		return
	}
	tok, err := googleExchangeFull(code, googleRedirectURI(r))
	if err != nil {
		app.Log("auth", "google %s exchange failed for %s: %v", g.verb, acc.ID, err)
		http.Redirect(w, r, g.ret+"?connection=failed", http.StatusSeeOther)
		return
	}
	// The consent screen lets somebody approve sign-in and decline the tickbox
	// that matters. That is a choice, not a failure, and it must not be stored
	// as a connection that then answers nothing.
	if !scopeGranted(tok.Scope, g.scope) {
		http.Redirect(w, r, g.ret+"?connection=declined", http.StatusSeeOther)
		return
	}
	if tok.RefreshToken == "" {
		app.Log("auth", "google %s grant for %s carried no refresh token", g.verb, acc.ID)
		http.Redirect(w, r, g.ret+"?connection=failed", http.StatusSeeOther)
		return
	}

	info, err := googleUserInfo(tok.AccessToken)
	if err != nil || info == nil || info.Email == "" || !info.EmailVerified {
		http.Redirect(w, r, g.ret+"?connection=failed", http.StatusSeeOther)
		return
	}
	google.Store(acc.ID, info.Email, tok.RefreshToken, strings.Fields(tok.Scope))
	http.Redirect(w, r, g.ret+"?connection=connected", http.StatusSeeOther)
}

// scopeGranted checks for an exact scope in the space-separated set Google
// returned. Substring matching would have said yes to a scope that merely
// contained the one asked for.
func scopeGranted(granted, want string) bool {
	for _, s := range strings.Fields(granted) {
		if s == want {
			return true
		}
	}
	return false
}

// googleToken is a token response, including the parts sign-in throws away.
type googleToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// googleExchangeFull trades a code for the whole token response. Sign-in uses
// googleExchange, which keeps only the access token because a grant it will
// never reuse is a credential it should not hold.
func googleExchangeFull(code, redirectURI string) (*googleToken, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", googleClientID())
	form.Set("client_secret", googleClientSecret())
	form.Set("redirect_uri", redirectURI)
	form.Set("grant_type", "authorization_code")

	req, _ := http.NewRequest(http.MethodPost, "https://oauth2.googleapis.com/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var t googleToken
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	if t.AccessToken == "" {
		return nil, fmt.Errorf("no access token (%s)", t.Error)
	}
	return &t, nil
}

// renderGoogleCard is everything about Google in one card: whether you can sign
// in with it, and what of it this account has handed over, with a way to take
// that back.
//
// It was two cards. One headed "Google", which was only about sign-in, and
// directly under it one headed "Connected accounts", which was only about
// Google. Somebody reading the card called Google reasonably expected the
// connections to be in it, and they were in the next card down under a name
// that did not say Google at all.
//
// Shown even when nothing is granted, because "nothing" is the answer somebody
// checking their permissions wants to be able to read. A card that appears only
// once you have granted something can never be used to confirm you have not.
func renderGoogleCard(r *http.Request, acc *auth.Account, status string) string {
	if !GoogleConfigured() || acc == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="card" id="google"><h4>Google</h4>` + googleSignIn(r, acc) + `<h5>Service access</h5><p>Read-only access for your assistant.</p>`)

	switch status {
	case "disconnected":
		b.WriteString(`<p class="text-sm text-success">Service access removed. Revocation was requested from Google.</p>`)
	case "failed", "declined":
		b.WriteString(`<p>Google access was not connected. Please try again.</p>`)
	case "connected":
		b.WriteString(`<p class="text-sm text-success">Connected.</p>`)
	}

	grantsCount := len(google.Grants(acc.ID))
	if grantsCount == 0 {
		b.WriteString(`<p>No services connected</p>`)
	} else {
		fmt.Fprintf(&b, `<p>%d services connected</p>`, grantsCount)
	}
	open := ""
	if status != "" {
		open = " open"
	}
	b.WriteString(`<details class="disclosure"` + open + `><summary>Manage service access</summary>`)
	b.WriteString(`<div class="connection-list">`)
	for _, item := range []struct{ key, label, purpose string }{
		{"gmail", "Gmail", "Find and read recent email."},
		{"calendar", "Calendar", "Check your plans and availability."},
		{"contacts", "Contacts", "Look up people you know."},
		{"drive", "Drive", "Find and read documents."},
	} {
		b.WriteString(`<div class="connection-row"><div><strong>` + item.label + `</strong><p>` + item.purpose + `</p></div>`)
		if google.HasScope(acc.ID, grants[item.key].scope) {
			b.WriteString(`<span class="connection-state">Connected</span>`)
		} else {
			b.WriteString(`<a class="btn" aria-label="Connect ` + item.label + `" href="/oauth2/google/` + item.key + `">Connect</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div><p class="text-sm text-muted">Information used to answer you is sent to this instance’s configured AI provider.</p><details class="disclosure"><summary>What Micro can access</summary><p>Your assistant and scheduled tasks can use these connections. Mail and files are not bulk imported. Gmail is limited to messages received in the last seven days; searches return headers, and reading a message shares up to 8,000 characters. Recent messages may quote older correspondence.</p></details>`)

	list := google.Grants(acc.ID)
	if len(list) == 0 {
		b.WriteString(`<p class="text-sm text-muted mt-2">No Gmail, Calendar, Contacts or Drive access.</p></details></div>`)
		return b.String()
	}

	// Which Google account the access came from. Usually the one you sign in
	// with and not necessarily — they are two separate grants and a person can
	// have two accounts, so this says whose rather than assuming.
	who := list[0].Email
	if who != "" {
		b.WriteString(`<p class="text-sm text-muted mt-2">Granted from <strong>` +
			htmlpkg.EscapeString(who) + `</strong>. All read-only.</p>`)
	}

	b.WriteString(`<div class="action-block"><form method="POST" action="/oauth2/google/disconnect" class="form-action m-0">` +
		`<input type="hidden" name="_csrf" value="` + htmlpkg.EscapeString(auth.CSRFToken(r)) + `">` +
		`<input type="hidden" name="return" value="/account">` +
		`<button type="submit" class="btn-plain text-sm">Disconnect services</button>` +
		`</form>`)
	b.WriteString(`<p class="action-note">Removes all service connections above. Google sign-in is unchanged.</p></div>`)
	b.WriteString(`</details></div>`)
	return b.String()
}
