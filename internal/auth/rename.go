package auth

// Changing your username.
//
// 189 people signed up and none of them chose the name they got. Google signup
// derives one from the email local part, which is how micro.mu came to have
// hlorahulr, a30006179 and wa400601 — names that pass every rule and are
// nobody's name. The page under them said "your username is the one in
// addresses and links and does not change", and it was telling the truth.
//
// On a network where the username is the local part of your address, the path
// to your page and the way somebody mentions you, an identity assigned by a
// string-mangling function is the wrong default. Claim already knew how to
// move an id across every store that keys on one; it was reachable only by
// accounts nobody had signed up for. This is the same move, for everybody.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mu/internal/data"
)

// renameEvery is how long to wait between changes, after the first.
//
// The first is free and deliberately so: a name derived from an email address
// was assigned rather than chosen, so replacing it is not a change of name but
// the first exercise of one. After that the interval is here to stop the two
// things a free rename buys — cycling through names to squat them, and wearing
// somebody else's for an afternoon.
const renameEvery = 30 * 24 * time.Hour

// Rename moves an account to a username its owner picked.
//
// The old name is not released. A username is a mailbox, so handing a vacated
// one to the next person who asks hands them the mail still being sent to it —
// password resets included. It stays on the account that left it, in Former,
// and nothing else may take it.
func Rename(oldID, newID string) error {
	newID = strings.ToLower(strings.TrimSpace(newID))
	if newID == "" {
		return errors.New("no username")
	}
	if newID == oldID {
		return errors.New("that is already your username")
	}
	if reason := ValidateUsername(newID); reason != "" {
		return errors.New(reason)
	}

	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[oldID]
	if !ok {
		return errors.New("no such account")
	}
	if !acc.Renamed.IsZero() {
		if wait := time.Until(acc.Renamed.Add(renameEvery)); wait > 0 {
			return fmt.Errorf("you can change your username again in %d days",
				int(wait.Hours()/24)+1)
		}
	}
	if reason := availableLocked(newID); reason != "" {
		return errors.New(reason)
	}

	delete(accounts, oldID)
	acc.ID = newID
	acc.Former = append(acc.Former, oldID)
	acc.Renamed = time.Now()
	accounts[newID] = acc
	data.SaveJSON("accounts.json", accounts)

	// Everything filed under the old id follows it — the conversations above
	// all, which is what makes this a rename rather than a new account. See
	// Renamed for why the stores register rather than get imported.
	for _, rename := range renamers {
		rename(oldID, newID)
	}
	return nil
}

// availableLocked says why a username cannot be taken, or "" if it can.
// Caller holds the mutex.
//
// Two ways to be unavailable and they are not the same: somebody is using it,
// or somebody used to. The second is the one that is easy to miss and is the
// reason this function exists rather than a map lookup at each call site.
func availableLocked(id string) string {
	if _, taken := accounts[id]; taken {
		return "that username is taken"
	}
	for _, acc := range accounts {
		for _, was := range acc.Former {
			if was == id {
				return "that username is not available"
			}
		}
	}
	return ""
}

// CanRenameAt is when this account may next change its username, or the zero
// time if it may now. For a page that would rather not offer a button that
// only says no.
func CanRenameAt(id string) time.Time {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok || acc.Renamed.IsZero() {
		return time.Time{}
	}
	at := acc.Renamed.Add(renameEvery)
	if time.Now().After(at) {
		return time.Time{}
	}
	return at
}
