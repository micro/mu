package google

import (
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
