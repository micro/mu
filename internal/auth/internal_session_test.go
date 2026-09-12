package auth

import (
	"testing"

	"mu/internal/data"
)

// TestInternalSessionsLeaveNothingBehind is the leak this replaced.
//
// api.ExecuteToolAs minted a session with CreateSession, which writes to
// sessions.json and is only ever removed on logout or account deletion. Every
// Discord slash command, every Telegram message and every agent tool call left
// a permanent, working login token in that file — and rewrote the whole file to
// add it.
func TestInternalSessionsLeaveNothingBehind(t *testing.T) {
	mutex.Lock()
	prevAcc, prevSess := accounts, sessions
	accounts = map[string]*Account{"someone": {ID: "someone", Name: "Someone"}}
	sessions = map[string]*Session{}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts, sessions = prevAcc, prevSess
		mutex.Unlock()
	})

	sess, err := InternalSession("someone")
	if err != nil {
		t.Fatal(err)
	}
	// It works while it is held: a tool has to be dispatched as somebody.
	if got, err := ParseToken(sess.Token); err != nil || got.Account != "someone" {
		t.Fatalf("an internal session did not resolve: %v %v", got, err)
	}

	EndSession(sess.Token)
	if _, err := ParseToken(sess.Token); err == nil {
		t.Error("the token still works after the call ended — that is a " +
			"credential left behind by every tool call")
	}
	mutex.Lock()
	left := len(sessions)
	mutex.Unlock()
	if left != 0 {
		t.Errorf("%d sessions left in the map after one call", left)
	}

	// Ending twice, or ending nonsense, must not panic: a defer runs whatever
	// happened in between.
	EndSession(sess.Token)
	EndSession("not-a-token")

	if _, err := InternalSession("nobody"); err == nil {
		t.Error("minted a session for an account that does not exist")
	}
}

func TestInternalSessionDoesNotSurviveAConcurrentLoginSave(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mutex.Lock()
	previousAccounts, previousSessions := accounts, sessions
	accounts = map[string]*Account{"probe": {ID: "probe"}}
	sessions = map[string]*Session{}
	mutex.Unlock()
	t.Cleanup(func() {
		mutex.Lock()
		accounts, sessions = previousAccounts, previousSessions
		mutex.Unlock()
	})

	temporary, err := InternalSession("probe")
	if err != nil {
		t.Fatal(err)
	}
	defer EndSession(temporary.Token)
	login, err := CreateSession("probe")
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]*Session
	if err := data.LoadJSON("sessions.json", &saved); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved[temporary.ID]; ok {
		t.Fatal("an unrelated login persisted the in-flight internal session")
	}
	if _, ok := saved[login.ID]; !ok {
		t.Fatal("ordinary login was not saved")
	}
	// The internal identity remains valid for the rest of its dispatch.
	if _, err := ParseToken(temporary.Token); err != nil {
		t.Fatalf("saving logins invalidated the in-flight call: %v", err)
	}
}

func TestPersistedSessionsDiscardLegacyInternalCredentials(t *testing.T) {
	stored := map[string]*Session{
		"temporary": {ID: "temporary", Type: "internal"},
		"login":     {ID: "login", Type: "account"},
		"legacy":    {ID: "legacy"},
		"null":      nil,
	}
	loaded := persistentSessions(stored)
	if len(loaded) != 2 || loaded["login"] == nil || loaded["legacy"] == nil {
		t.Fatal("loading sessions did not preserve only durable login credentials")
	}
	if stored["temporary"] == nil {
		t.Fatal("filtering the disk representation mutated live internal sessions")
	}
}
