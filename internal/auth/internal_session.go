package auth

// Sessions for calls that start inside this process.
//
// api.ExecuteToolAs mints a session so a tool can be dispatched as somebody,
// and it did that with CreateSession — which writes the session to the accounts
// store. Every Discord slash command, every Telegram message, every agent tool
// call therefore left a permanent credential in sessions.json, and rewrote the
// whole file to do it. Sessions are only ever deleted on logout or account
// deletion, so nothing removed them: the file grew without bound and every line
// in it was a working login token.
//
// A call that begins and ends inside this process does not need any of that. It
// needs an identity for the duration of one dispatch and then it needs to
// disappear.

import (
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
)

// InternalSession mints a session that lives only in memory and only until the
// caller ends it. Nothing is written to disk, so a tool call costs no I/O and
// leaves no credential behind.
//
// The caller must end it — `defer EndSession(sess.Token)` — or it stays in the
// map for the life of the process, which is a leak with a shorter fuse than the
// one this replaced but a leak all the same.
func InternalSession(id string) (*Session, error) {
	mutex.Lock()
	defer mutex.Unlock()

	if _, ok := accounts[id]; !ok {
		return nil, errors.New("account does not exist")
	}
	guid := uuid.New().String()
	sess := &Session{
		ID:      guid,
		Type:    "internal",
		Token:   base64.StdEncoding.EncodeToString([]byte(guid)),
		Account: id,
		Created: time.Now(),
	}
	sessions[guid] = sess
	return sess, nil
}

// EndSession drops a session without touching disk. Safe to call on a token
// that is already gone.
func EndSession(tk string) {
	dec, err := base64.StdEncoding.DecodeString(tk)
	if err != nil {
		return
	}
	id, err := uuid.Parse(string(dec))
	if err != nil {
		return
	}
	mutex.Lock()
	delete(sessions, id.String())
	mutex.Unlock()
}
