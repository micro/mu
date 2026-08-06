// Package google is Mu's client for the Google APIs a signed-in person can
// grant it access to — starting with their calendar.
//
// This is deliberately the *only* place a Google user credential is held, and
// it is separate from internal/app's sign-in code for one reason: sign-in
// answers "who is this", and it needs nothing but an id token it can throw
// away. Reading somebody's calendar is a standing grant, so it needs a refresh
// token kept at rest, and a thing kept at rest wants one owner.
//
// Why this exists at all, given that Mu runs its own services rather than
// wrapping other people's: events is already Mu's own calendar, and its Spec
// promises "what is scheduled, and when you are free". It could answer neither
// honestly, because the events it knows about are the ones Mu created — a
// person's real week is in the calendar they already use. This is not a wrapper
// standing in for a service Mu runs; it is the missing half of one.
//
// Read-only, on purpose. Mu writes events into its own store and offers an .ics
// to add them; it does not need permission to change a Google calendar to
// answer the two questions above, and asking for less is the difference between
// a consent screen someone accepts and one they close.
package google

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"mu/internal/data"
	"mu/internal/settings"
)

// CalendarScope is what Mu asks for: enough to list events and query free/busy,
// and not enough to change anything.
const CalendarScope = "https://www.googleapis.com/auth/calendar.readonly"

const storeKey = "google_connections.json"

// httpClient is shared; every call here is a small JSON request.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// Connection is a standing grant from one account to read their Google data.
//
// RefreshToken is a live credential. It is written owner-only by the data
// package, never logged, and never leaves this package — callers get answers,
// not tokens.
type Connection struct {
	AccountID    string    `json:"account_id"`
	Email        string    `json:"email,omitempty"`
	RefreshToken string    `json:"refresh_token"`
	Scopes       []string  `json:"scopes,omitempty"`
	Connected    time.Time `json:"connected"`
}

var (
	mu    sync.RWMutex
	conns = map[string]*Connection{}

	// access caches short-lived access tokens so a page that asks twice does
	// not spend two round trips on the same second.
	access = map[string]cachedToken{}
)

type cachedToken struct {
	token   string
	expires time.Time
}

// Load reads stored connections. Called once at startup.
func Load() {
	mu.Lock()
	defer mu.Unlock()
	var list []*Connection
	if err := data.LoadJSON(storeKey, &list); err == nil {
		for _, c := range list {
			if c != nil && c.AccountID != "" && c.RefreshToken != "" {
				conns[c.AccountID] = c
			}
		}
	}
}

// save persists connections. Callers hold mu.
func save() {
	list := make([]*Connection, 0, len(conns))
	for _, c := range conns {
		list = append(list, c)
	}
	_ = data.SaveJSON(storeKey, list)
}

// Connected reports whether an account has granted anything at all.
func Connected(accountID string) bool {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := conns[accountID]
	return ok && c.RefreshToken != ""
}

// HasScope reports whether an account's grant covers one particular thing.
//
// Per-scope rather than per-account, because these are separate decisions made
// at separate moments: somebody who attached a calendar has not thereby agreed
// to hand over their address book, and a UI that treats one grant as permission
// for the next is the pattern this whole flow exists to avoid.
func HasScope(accountID, scope string) bool {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := conns[accountID]
	if !ok || c.RefreshToken == "" {
		return false
	}
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// dropScope forgets one capability without dropping the whole grant — for when
// Google answers 403, meaning the person withdrew that scope at their end.
// Continuing to show them as connected to something that answers nothing is the
// one state worse than being disconnected.
func dropScope(accountID, scope string) {
	mu.Lock()
	defer mu.Unlock()
	c, ok := conns[accountID]
	if !ok {
		return
	}
	kept := c.Scopes[:0]
	for _, s := range c.Scopes {
		if s != scope {
			kept = append(kept, s)
		}
	}
	c.Scopes = kept
	save()
}

// Grant is one capability an account has handed over, for showing them the
// whole list in one place.
type Grant struct {
	Scope string
	Email string
}

// Grants is everything this account has granted, so there can be a single
// screen answering "what does Mu have access to". A permission that can only be
// found on the page that happens to use it is a permission nobody audits.
func Grants(accountID string) []Grant {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := conns[accountID]
	if !ok || c.RefreshToken == "" {
		return nil
	}
	out := make([]Grant, 0, len(c.Scopes))
	for _, s := range c.Scopes {
		// openid and email come along with every grant as the price of knowing
		// which Google account it is. They are not capabilities anybody chose,
		// so listing them as such would pad the audit with noise.
		if s == "openid" || s == "email" || s == "profile" ||
			strings.HasSuffix(s, "auth/userinfo.email") || strings.HasSuffix(s, "auth/userinfo.profile") {
			continue
		}
		out = append(out, Grant{Scope: s, Email: c.Email})
	}
	return out
}

// Label names a scope the way a person would, for the account page. Unknown
// scopes fall back to the raw string rather than being hidden — an audit screen
// that silently omits what it does not recognise is worse than an ugly one.
func Label(scope string) string {
	switch scope {
	case CalendarScope:
		return "Google Calendar (read-only)"
	case ContactsScope:
		return "Google Contacts (read-only)"
	}
	return scope
}

// ConnectedEmail is the Google address behind the grant, for showing someone
// which account they connected. Empty when not connected.
func ConnectedEmail(accountID string) string {
	mu.RLock()
	defer mu.RUnlock()
	if c, ok := conns[accountID]; ok {
		return c.Email
	}
	return ""
}

// Store records a grant. An empty refresh token is ignored rather than stored:
// Google only returns one on the first consent, and overwriting a good token
// with "" would silently disconnect someone who re-authorised.
func Store(accountID, email, refreshToken string, scopes []string) {
	if accountID == "" || refreshToken == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	conns[accountID] = &Connection{
		AccountID:    accountID,
		Email:        email,
		RefreshToken: refreshToken,
		Scopes:       scopes,
		Connected:    time.Now(),
	}
	delete(access, accountID)
	save()
}

// Disconnect withdraws an account's grant: revoked at Google, then forgotten
// here.
//
// Both halves, because "disconnect" has to mean disconnected. Forgetting the
// token locally would leave Mu listed in the person's Google account as an app
// with standing access to their calendar — access it can no longer use, but
// which they would have to go and remove themselves to be rid of. A product
// that keeps its name on that list after you pressed Disconnect is lying by
// omission.
//
// The revoke is best-effort and the local forget is not: if Google is
// unreachable, Mu still drops the credential. Holding somebody's token because
// a third party timed out is the one outcome that must not happen.
func Disconnect(accountID string) {
	mu.Lock()
	token := ""
	if c, ok := conns[accountID]; ok {
		token = c.RefreshToken
	}
	delete(conns, accountID)
	delete(access, accountID)
	save()
	mu.Unlock()

	// An instance with no Google credentials cannot have obtained this grant
	// through a live flow, so there is nothing at Google's end to tell. This
	// also keeps tests off the network.
	if token != "" && Configured() {
		revoke(token)
	}
}

// forget drops a grant locally without calling Google — for the case where the
// grant is already dead at Google's end, so revoking it would be a round trip
// to be told what Mu just learned.
func forget(accountID string) {
	mu.Lock()
	defer mu.Unlock()
	delete(conns, accountID)
	delete(access, accountID)
	save()
}

// revokeEndpoint is Google's, and a variable so a test can watch what is sent
// to it without reaching the internet.
var revokeEndpoint = "https://oauth2.googleapis.com/revoke"

// revoke tells Google the grant is over. Revoking a refresh token takes the
// whole grant with it.
func revoke(refreshToken string) {
	form := url.Values{}
	form.Set("token", refreshToken)
	req, err := http.NewRequest(http.MethodPost, revokeEndpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func clientID() string     { return strings.TrimSpace(settings.Get("GOOGLE_CLIENT_ID")) }
func clientSecret() string { return strings.TrimSpace(settings.Get("GOOGLE_CLIENT_SECRET")) }

// Configured reports whether Google OAuth is set up on this instance at all.
// A self-hosted Mu with no Google credentials should never show a connect
// button that cannot work.
func Configured() bool { return clientID() != "" && clientSecret() != "" }

// accessToken returns a usable access token for an account, refreshing when the
// cached one is spent.
func accessToken(accountID string) (string, error) {
	mu.RLock()
	if t, ok := access[accountID]; ok && time.Now().Before(t.expires) {
		mu.RUnlock()
		return t.token, nil
	}
	c := conns[accountID]
	mu.RUnlock()

	if c == nil || c.RefreshToken == "" {
		return "", ErrNotConnected
	}
	if !Configured() {
		return "", fmt.Errorf("google is not configured on this instance")
	}

	form := url.Values{}
	form.Set("client_id", clientID())
	form.Set("client_secret", clientSecret())
	form.Set("refresh_token", c.RefreshToken)
	form.Set("grant_type", "refresh_token")

	req, _ := http.NewRequest(http.MethodPost, "https://oauth2.googleapis.com/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.AccessToken == "" {
		// invalid_grant means the person revoked access at Google's end, or the
		// grant expired. Holding a dead credential helps nobody, and keeping it
		// would leave the UI claiming a connection that cannot answer.
		if t.Error == "invalid_grant" {
			forget(accountID)
			return "", ErrNotConnected
		}
		return "", fmt.Errorf("could not refresh google access (%s)", t.Error)
	}

	ttl := time.Duration(t.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	mu.Lock()
	// A minute of headroom, so a token fetched here is not spent mid-request.
	access[accountID] = cachedToken{token: t.AccessToken, expires: time.Now().Add(ttl - time.Minute)}
	mu.Unlock()
	return t.AccessToken, nil
}

// ErrNotConnected is returned when an account has not granted access. Callers
// treat it as "nothing to add", not as a failure — a person who never connected
// a calendar is not experiencing an error.
var ErrNotConnected = fmt.Errorf("google calendar not connected")

// Period is a stretch of booked time. Deliberately not the events service's
// Slot type: this package must not depend on a service.
type Period struct {
	Start time.Time
	End   time.Time
}

// Busy returns the account's booked periods in a window, from their primary
// calendar.
//
// Primary only, which is a real limit worth naming: somebody keeping work and
// personal calendars separate will get an answer computed from one of them, and
// "you are free" is exactly the answer that must not be over-confident. Widening
// this means listing calendarList and passing every id here.
//
// freeBusy rather than events.list, because "when am I free" needs only the
// shape of the week. It returns times and no titles, so the narrower question
// is answered with the narrower data.
func Busy(accountID string, from, to time.Time) ([]Period, error) {
	token, err := accessToken(accountID)
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string]any{
		"timeMin": from.Format(time.RFC3339),
		"timeMax": to.Format(time.RFC3339),
		"items":   []map[string]string{{"id": "primary"}},
	})
	req, _ := http.NewRequest(http.MethodPost,
		"https://www.googleapis.com/calendar/v3/freeBusy", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google calendar freebusy: %s", resp.Status)
	}

	var out struct {
		Calendars map[string]struct {
			Busy []struct {
				Start string `json:"start"`
				End   string `json:"end"`
			} `json:"busy"`
		} `json:"calendars"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var periods []Period
	for _, cal := range out.Calendars {
		for _, b := range cal.Busy {
			start, err1 := time.Parse(time.RFC3339, b.Start)
			end, err2 := time.Parse(time.RFC3339, b.End)
			if err1 != nil || err2 != nil {
				continue
			}
			periods = append(periods, Period{Start: start, End: end})
		}
	}
	return periods, nil
}

// Entry is one event on the person's real calendar.
type Entry struct {
	Title    string
	Start    time.Time
	End      time.Time
	Location string
	AllDay   bool
}

// Events lists what is actually scheduled in a window.
//
// singleEvents expands recurrence, so a weekly standup arrives as the instances
// a person would recognise rather than as one rule they would have to apply
// themselves.
func Events(accountID string, from, to time.Time, limit int) ([]Entry, error) {
	token, err := accessToken(accountID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 25
	}

	q := url.Values{}
	q.Set("timeMin", from.Format(time.RFC3339))
	q.Set("timeMax", to.Format(time.RFC3339))
	q.Set("singleEvents", "true")
	q.Set("orderBy", "startTime")
	q.Set("maxResults", fmt.Sprint(limit))

	req, _ := http.NewRequest(http.MethodGet,
		"https://www.googleapis.com/calendar/v3/calendars/primary/events?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google calendar events: %s", resp.Status)
	}

	var out struct {
		Items []struct {
			Summary  string `json:"summary"`
			Location string `json:"location"`
			Status   string `json:"status"`
			Start    struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var entries []Entry
	for _, it := range out.Items {
		if it.Status == "cancelled" {
			continue
		}
		e := Entry{Title: strings.TrimSpace(it.Summary), Location: strings.TrimSpace(it.Location)}
		if e.Title == "" {
			e.Title = "(no title)"
		}
		switch {
		case it.Start.DateTime != "":
			e.Start, _ = time.Parse(time.RFC3339, it.Start.DateTime)
			e.End, _ = time.Parse(time.RFC3339, it.End.DateTime)
		case it.Start.Date != "":
			// An all-day event has a date and no time. Parsed in local time so
			// "today" means the reader's today.
			e.AllDay = true
			e.Start, _ = time.ParseInLocation("2006-01-02", it.Start.Date, time.Local)
			e.End, _ = time.ParseInLocation("2006-01-02", it.End.Date, time.Local)
		}
		if e.Start.IsZero() {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}
