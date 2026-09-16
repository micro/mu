package google

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mu-google-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// reset clears state between tests.
func reset() {
	mu.Lock()
	conns = map[string]*Connection{}
	access = map[string]cachedToken{}
	mu.Unlock()
}

func TestAGrantIsRememberedAndCanBeWithdrawn(t *testing.T) {
	reset()

	if Connected("someone") {
		t.Error("an account that never granted anything reads as connected")
	}

	Store("someone", "them@example.com", "refresh-abc", []string{CalendarScope})
	if !Connected("someone") {
		t.Fatal("a stored grant did not take")
	}
	if got := ConnectedEmail("someone"); got != "them@example.com" {
		t.Errorf("connected email is %q", got)
	}

	Disconnect("someone")
	if Connected("someone") {
		t.Error("a withdrawn grant is still held")
	}
	if got := ConnectedEmail("someone"); got != "" {
		t.Errorf("a withdrawn grant left an email behind: %q", got)
	}
}

// Google returns a refresh token only on a fresh consent. Storing an empty one
// over a good one would silently disconnect somebody who just re-authorised —
// the failure would show up an hour later, when the access token expired.
func TestAnEmptyRefreshTokenNeverClobbersAGoodOne(t *testing.T) {
	reset()

	Store("someone", "them@example.com", "refresh-abc", []string{CalendarScope})
	Store("someone", "them@example.com", "", []string{CalendarScope})

	if !Connected("someone") {
		t.Error("a re-authorisation with no new refresh token dropped the grant")
	}
}

// Mu asks to read a calendar and nothing more. The scope string is the entire
// contract with the person granting it, so it is worth a test that fails loudly
// if it ever widens.
func TestTheScopeAskedForIsReadOnly(t *testing.T) {
	if CalendarScope != "https://www.googleapis.com/auth/calendar.readonly" {
		t.Errorf("the calendar scope is %q", CalendarScope)
	}
}

// An account with no grant is not an error condition — it is the normal state
// of almost everybody, and callers rely on the distinction to stay quiet.
func TestReadingWithoutAGrantSaysNotConnected(t *testing.T) {
	reset()
	now := time.Now()

	if _, err := Busy("stranger", now, now.Add(time.Hour)); err != ErrNotConnected {
		t.Errorf("Busy without a grant returned %v", err)
	}
	if _, err := Events("stranger", now, now.Add(time.Hour), 10); err != ErrNotConnected {
		t.Errorf("Events without a grant returned %v", err)
	}
}

// Grants survive a restart, or connecting once would mean connecting daily.
func TestGrantsSurviveARestart(t *testing.T) {
	reset()
	Store("persist", "them@example.com", "refresh-xyz", []string{CalendarScope})

	reset() // as if the process had gone away
	if Connected("persist") {
		t.Fatal("the test's own reset did not clear memory")
	}

	Load()
	if !Connected("persist") {
		t.Error("a stored grant did not survive a restart")
	}
}

// Disconnect has to mean disconnected. Forgetting the token locally would leave
// Mu listed in the person's Google account as an app with standing access to
// their calendar — access it could no longer use, but which they would have to
// go and remove themselves. Pressing Disconnect must do that for them.
func TestDisconnectRevokesAtGoogleAndNotJustHere(t *testing.T) {
	reset()

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.Form.Get("token")
	}))
	defer srv.Close()

	prev := revokeEndpoint
	revokeEndpoint = srv.URL
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Cleanup(func() { revokeEndpoint = prev })

	Store("leaver", "them@example.com", "refresh-to-revoke", []string{CalendarScope})
	Disconnect("leaver")

	if got != "refresh-to-revoke" {
		t.Errorf("the grant was not revoked at Google (endpoint saw %q)", got)
	}
	if Connected("leaver") {
		t.Error("the grant was revoked but still held locally")
	}
}

// Google being unreachable must never leave Mu holding somebody's credential.
func TestDisconnectDropsTheTokenEvenIfGoogleIsDown(t *testing.T) {
	reset()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	srv.Close() // closed: connecting to it fails outright

	prev := revokeEndpoint
	revokeEndpoint = srv.URL
	t.Setenv("GOOGLE_CLIENT_ID", "test-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Cleanup(func() { revokeEndpoint = prev })

	Store("stranded", "them@example.com", "refresh-abc", []string{CalendarScope})
	Disconnect("stranded")

	if Connected("stranded") {
		t.Error("a failed revoke left Mu holding the token")
	}
}

// Grants are per-scope, because they are separate decisions made at separate
// moments. Somebody who attached a calendar has not thereby agreed to hand over
// their address book, and a UI that treats one grant as permission for the next
// is the pattern this whole flow exists to avoid.
func TestGrantsAreCheckedPerScope(t *testing.T) {
	reset()

	Store("partial", "them@example.com", "refresh-abc", []string{CalendarScope})

	if !HasScope("partial", CalendarScope) {
		t.Error("the granted scope reads as missing")
	}
	if HasScope("partial", ContactsScope) {
		t.Error("granting a calendar was treated as granting an address book")
	}
	if HasScope("stranger", CalendarScope) {
		t.Error("an account with no grant at all reads as having one")
	}
}

// The audit screen lists what was actually chosen. openid/email ride along with
// every grant as the price of knowing which Google account it is, so listing
// them as capabilities would pad the list with things nobody decided.
func TestTheGrantListIsWhatSomebodyActuallyChose(t *testing.T) {
	reset()

	Store("auditor", "them@example.com", "refresh-abc",
		[]string{"openid", "email", CalendarScope, ContactsScope})

	list := Grants("auditor")
	if len(list) != 2 {
		t.Fatalf("the audit list is %+v, want just the two real capabilities", list)
	}
	for _, g := range list {
		if g.Scope == "openid" || g.Scope == "email" {
			t.Errorf("sign-in scope listed as a capability: %+v", g)
		}
		if Label(g.Scope) == g.Scope {
			t.Errorf("scope %q has no human label", g.Scope)
		}
	}

	if got := Grants("stranger"); len(got) != 0 {
		t.Errorf("an account with no grant listed %+v", got)
	}
}

// When Google answers 403 the person withdrew that scope at their end. Showing
// them as still connected to something that answers nothing is the one state
// worse than being disconnected.
func TestALostScopeIsForgottenWithoutDroppingTheRest(t *testing.T) {
	reset()

	Store("shrinking", "them@example.com", "refresh-abc", []string{CalendarScope, ContactsScope})
	dropScope("shrinking", ContactsScope)

	if HasScope("shrinking", ContactsScope) {
		t.Error("a withdrawn scope is still claimed")
	}
	if !HasScope("shrinking", CalendarScope) {
		t.Error("losing one scope dropped another")
	}
	if !Connected("shrinking") {
		t.Error("losing one scope dropped the whole grant")
	}
}
