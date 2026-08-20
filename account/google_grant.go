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
	if _, _, err := auth.RequireSession(r); err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	if !GoogleConfigured() {
		http.Redirect(w, r, g.ret, http.StatusSeeOther)
		return
	}

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

	email := ""
	if info, err := googleUserInfo(tok.AccessToken); err == nil {
		email = info.Email
	}
	google.Store(acc.ID, email, tok.RefreshToken, strings.Fields(tok.Scope))
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

// renderConnectionsCard is the audit screen: everything this account has handed
// over, in one place, with a way to take it back.
//
// Shown even when empty, because "nothing" is the answer somebody checking
// their permissions wants to be able to read. A card that appears only once you
// have granted something can never be used to confirm you have not.
func renderConnectionsCard(r *http.Request, acc *auth.Account, status string) string {
	if !GoogleConfigured() || acc == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h4>Connected accounts</h4>`)

	switch status {
	case "disconnected":
		b.WriteString(`<p class="text-sm text-success">Disconnected. The access was revoked at Google and forgotten here.</p>`)
	case "connected":
		b.WriteString(`<p class="text-sm text-success">Connected.</p>`)
	}

	list := google.Grants(acc.ID)
	if len(list) == 0 {
		b.WriteString(`<p class="text-sm text-muted">Mu has no access to anything in your Google account. ` +
			`You can attach your calendar from <a href="/events">events</a> or your contacts from ` +
			`<a href="/contacts">contacts</a>, and it will be listed here.</p></div>`)
		return b.String()
	}

	who := list[0].Email
	if who != "" {
		b.WriteString(`<p class="text-sm text-muted">Granted from <strong>` +
			htmlpkg.EscapeString(who) + `</strong>. All read-only.</p>`)
	}
	b.WriteString(`<ul class="mt-2 mb-3 indent">`)
	for _, g := range list {
		b.WriteString(`<li class="text-sm">` + htmlpkg.EscapeString(google.Label(g.Scope)) + `</li>`)
	}
	b.WriteString(`</ul>`)

	b.WriteString(`<form method="POST" action="/oauth2/google/disconnect" class="m-0">` +
		`<input type="hidden" name="_csrf" value="` + htmlpkg.EscapeString(auth.CSRFToken(r)) + `">` +
		`<input type="hidden" name="return" value="/account">` +
		`<button type="submit" class="btn-plain text-sm">Disconnect Google</button>` +
		`</form>`)
	b.WriteString(`<p class="text-sm text-muted mt-2">Revoking takes all of it: ` +
		`Google withdraws the whole grant at once, so there is no way to hand back one and keep another.</p>`)
	b.WriteString(`</div>`)
	return b.String()
}
