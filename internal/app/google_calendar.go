package app

// Connecting a Google Calendar.
//
// Separate from sign-in, and asked for separately. Google calls this
// incremental authorisation and it is the right shape for a reason that is
// about people rather than protocol: the consent screen for "read your
// calendar" is a much bigger ask than "know who you are", and a product that
// bundles them into the signup button trains everyone to decline at the door.
//
// So nobody is asked at signup. The ask appears where the answer is needed —
// the events page, and the reply the agent gives when someone asks what they
// have on. Somebody who never asks a calendar question never sees a consent
// screen, and the rate at which the ask is accepted is then a real signal about
// whether this was worth building.
//
// include_granted_scopes keeps the earlier sign-in grant alive, so connecting a
// calendar does not quietly cost someone the ability to sign in.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mu/internal/auth"
	"mu/internal/google"
)

// GoogleCalendarConnect starts the calendar grant for the logged-in account.
func GoogleCalendarConnect(w http.ResponseWriter, r *http.Request) {
	if _, _, err := auth.RequireSession(r); err != nil {
		RedirectToLogin(w, r)
		return
	}
	if !GoogleConfigured() {
		http.Redirect(w, r, "/events", http.StatusSeeOther)
		return
	}

	state := randToken(16)
	secure := requestSecure(r)
	http.SetCookie(w, &http.Cookie{
		Name: "g_state", Value: state, Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	// Tells the shared callback this is a calendar grant rather than a sign-in.
	http.SetCookie(w, &http.Cookie{
		Name: "g_cal", Value: "1", Path: "/", MaxAge: 600,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})

	q := url.Values{}
	q.Set("client_id", googleClientID())
	q.Set("redirect_uri", googleRedirectURI(r))
	q.Set("response_type", "code")
	q.Set("scope", "openid email "+google.CalendarScope)
	q.Set("state", state)
	// offline, because the point is answering "am I free Thursday" in a
	// scheduled morning brief at 7am when nobody is holding a browser open.
	q.Set("access_type", "offline")
	// consent, because Google returns a refresh token only on a fresh grant —
	// without this, a second connect after a disconnect yields a token Mu
	// cannot refresh, and the connection dies an hour later.
	q.Set("prompt", "consent")
	q.Set("include_granted_scopes", "true")

	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+q.Encode(), http.StatusSeeOther)
}

// GoogleCalendarDisconnect drops the stored grant.
func GoogleCalendarDisconnect(w http.ResponseWriter, r *http.Request) {
	// POST only. Forgetting a grant is a state change, and the CSRF middleware
	// only guards the methods that are supposed to make one.
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/events", http.StatusSeeOther)
		return
	}
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		RedirectToLogin(w, r)
		return
	}
	google.Disconnect(acc.ID)
	http.Redirect(w, r, "/events?calendar=disconnected", http.StatusSeeOther)
}

// finishGoogleCalendar completes a calendar grant from the shared callback.
func finishGoogleCalendar(w http.ResponseWriter, r *http.Request, code string) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		RedirectToLogin(w, r)
		return
	}

	tok, err := googleExchangeFull(code, googleRedirectURI(r))
	if err != nil {
		Log("auth", "google calendar exchange failed for %s: %v", acc.ID, err)
		http.Redirect(w, r, "/events?calendar=failed", http.StatusSeeOther)
		return
	}
	if !strings.Contains(tok.Scope, "calendar") {
		// The consent screen lets someone approve sign-in and decline the
		// calendar tickbox. That is a choice, not a failure, and it must not be
		// stored as a connection that then returns nothing.
		http.Redirect(w, r, "/events?calendar=declined", http.StatusSeeOther)
		return
	}
	if tok.RefreshToken == "" {
		Log("auth", "google calendar grant for %s carried no refresh token", acc.ID)
		http.Redirect(w, r, "/events?calendar=failed", http.StatusSeeOther)
		return
	}

	email := ""
	if info, err := googleUserInfo(tok.AccessToken); err == nil {
		email = info.Email
	}
	google.Store(acc.ID, email, tok.RefreshToken, strings.Fields(tok.Scope))
	http.Redirect(w, r, "/events?calendar=connected", http.StatusSeeOther)
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
