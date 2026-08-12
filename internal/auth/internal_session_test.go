package auth

import "testing"

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
