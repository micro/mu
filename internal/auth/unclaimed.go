package auth

// Claiming an account that was opened on somebody's behalf.
//
// There used to be a front door here. Somebody wrote to agent@ from an address
// nobody had heard of, and rather than drop it this opened them an account —
// unclaimed, no password, holding the conversation, with a small allowance of
// turns before they were invited to sign up. internal/trial held the other half,
// an instance-wide daily ceiling, because an allowance per sender address is
// unbounded in aggregate.
//
// It is gone. Not to save money: a free front door is recovered from somewhere,
// and the usual somewhere is the person walking through it. The product sells
// tools and the agent comes with an account, so a stranger signs up — which
// takes less effort than the email did. service/mail drops mail from senders
// with no account, silently, as it did before any of this.
//
// What remains is Claim, because unclaimed accounts already exist on instances
// that ran the old path, and an invitation that adopts one is how the
// conversation survives signing up. Deleting it would strand real data with no
// way to reach it.

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"mu/internal/data"
)

// Claim turns an unclaimed account into a real one: a chosen username and a
// password, keeping everything it already holds.
//
// The id changes, which is why this is here rather than three calls at the call
// site — an account id is the key of the map, the owner of every message in
// internal/thread and the scope of every note, so moving one is a rename across
// the instance and has to happen in one place. Renamed rather than copied
// because copying would leave the conversation behind, which is the one thing
// this design exists to keep.
func Claim(oldID, newID, secret string) error {
	if newID == "" {
		return errors.New("no username")
	}
	// The same rule signup answers to. Claiming was the door that did not ask:
	// an account created from an inbound address could be claimed under any
	// name at all, reserved ones included, which would have handed somebody
	// admin@ or agent@ for the cost of an email.
	if reason := ValidateUsername(newID); reason != "" {
		return errors.New(reason)
	}
	mutex.Lock()
	acc, ok := accounts[oldID]
	if !ok {
		mutex.Unlock()
		return errors.New("no such account")
	}
	if !acc.Unclaimed {
		mutex.Unlock()
		return errors.New("that account has already been claimed")
	}
	if _, taken := accounts[newID]; taken && newID != oldID {
		mutex.Unlock()
		return errors.New("that username is taken")
	}
	mutex.Unlock()

	// Outside the lock: hashing is deliberately slow and Create holds the same
	// mutex.
	hashed, err := hashSecret(secret)
	if err != nil {
		return err
	}

	mutex.Lock()
	defer mutex.Unlock()
	acc.Secret = hashed
	acc.SecretSet = true
	acc.Unclaimed = false
	acc.Turns = 0
	if newID != oldID {
		delete(accounts, oldID)
		acc.ID = newID
		accounts[newID] = acc
	}
	data.SaveJSON("accounts.json", accounts)

	// Everything filed under the old id follows it. Registered by the packages
	// that own records rather than reached into from here, because this package
	// sits underneath them — see Renamed.
	for _, rename := range renamers {
		rename(oldID, newID)
	}
	return nil
}

// renamers are the stores that hold something keyed on an account id.
//
// internal/thread has the conversations, and it is the reason claiming is worth
// doing at all: the exchange somebody had by mail is waiting for them when they
// first sign in. Registered rather than imported because those packages are
// above this one.
var renamers []func(oldID, newID string)

// Renamed registers a store to be told when an account id changes.
func Renamed(f func(oldID, newID string)) { renamers = append(renamers, f) }

// hashSecret is what Create does to a password, on its own so claiming an
// account and creating one cannot drift into two different hashes.
func hashSecret(secret string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), 10)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
