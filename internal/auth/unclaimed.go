package auth

// The account somebody has before they have an account.
//
// Somebody writes to agent@ from an address nobody here has heard of. Until now
// that message was dropped — silently, so a probe could not learn the address
// was live, which is a good reason for a bad outcome: the front page says "write
// to it and it answers", and for everybody without an account it did not.
//
// There has to be an account, because everything downstream is keyed on one.
// agent.Ask takes an account. internal/thread records against an account. Notes
// are scoped to an account. A parallel identity for strangers would mean
// threading a second concept through all of it, and then reconciling the two the
// day somebody signs up.
//
// So it is an account, unclaimed. It cannot sign in — no secret, no password
// that hashes to anything — it holds the conversation and the count of free
// turns, and signing up **claims it** rather than making a second one. That is
// the part worth having: the exchange somebody already had by email is waiting
// for them the first time they log in, rather than being a thing that happened
// to a different identity.
//
// # What this is not
//
// It is not a free tier and the difference is the same one agent/guest.go draws
// about guests: a per-sender allowance is unbounded in aggregate, because it
// costs whatever arrives. Whoever calls this is responsible for the ceiling as
// well as the count. See TurnsLeft.
//
// # Who may have one
//
// Only a sender whose mail authenticated — SPF or DKIM. Without that the sender
// address is whatever somebody typed, and "ten free turns per address" is an
// open model-call endpoint costing an operator real money per request. The check
// belongs at the door rather than here, because this package cannot see an SMTP
// session; Unclaimed is not to be called for an unauthenticated sender.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"mu/internal/data"
	"mu/internal/settings"
)

// FreeTurns is how many exchanges an unclaimed account gets before it is asked
// to sign up.
//
// A setting, not a constant in copy. What a demonstration is worth is an
// operator's decision, and the landing page deliberately stopped naming a
// number: a page that offers a daily allowance is describing a free plan, and
// fixes in writing something that is theirs to change. Nothing user-facing says
// ten; the mail at the end says "that is the free ones".
func FreeTurns() int {
	v := strings.TrimSpace(settings.Get("FREE_TURNS"))
	if v == "" {
		return 10
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 10
	}
	return n
}

// Unclaimed finds or creates the account behind an email address.
//
// Returns the existing account when the address already belongs to one, claimed
// or not — the second message from the same person continues the first
// conversation, which is the whole point of an agent that remembers.
//
// The caller must have established that the address is really the sender's. See
// the package comment.
func Unclaimed(addr string) (*Account, error) {
	addr = NormaliseAddress(addr)
	if addr == "" {
		return nil, errors.New("no address")
	}
	if acc := AccountForAddress(addr); acc != nil {
		return acc, nil
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Checked again under the lock: two messages from the same new sender can
	// arrive at once, and two accounts for one person is the failure this whole
	// thing exists to avoid.
	for _, acc := range accounts {
		if acc.Owns(addr) {
			return acc, nil
		}
	}

	id, err := unclaimedID(addr)
	if err != nil {
		return nil, err
	}

	acc := &Account{
		ID:      id,
		Name:    strings.SplitN(addr, "@", 2)[0],
		Created: time.Now(),
		Email:   addr,
		// Verified, because the mail authenticated: SPF or DKIM says the
		// sending domain vouches for it, which is the same standard of proof
		// the inbound whitelist already accepts. Marking it unverified would
		// mean asking somebody to prove an address they have just written from.
		EmailVerified:   true,
		EmailVerifiedAt: time.Now(),
		// No secret. bcrypt cannot match a hash of nothing against any input, so
		// Login refuses and there is no password to guess. Claiming happens
		// through the invite, which is the only way in.
		Unclaimed: true,
	}
	accounts[id] = acc
	data.SaveJSON("accounts.json", accounts)
	return acc, nil
}

// unclaimedID makes a username nobody will meet.
//
// Derived from the address, so it is recognisable to an operator reading the
// list, with random bytes on the end because two people are called asim and a
// collision here would join two strangers' conversations. It is renamed when the
// account is claimed and somebody chooses what to be called, so it never has to
// be pretty.
func unclaimedID(addr string) (string, error) {
	local := strings.SplitN(addr, "@", 2)[0]
	var clean strings.Builder
	for _, r := range strings.ToLower(local) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			clean.WriteRune(r)
		}
		if clean.Len() >= 12 {
			break
		}
	}
	stem := clean.String()
	if stem == "" {
		stem = "guest"
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return stem + "-" + hex.EncodeToString(b), nil
}

// TurnsLeft is how many free exchanges this account has before it is asked to
// sign up, and is meaningless for one that has been claimed.
//
// A claimed account is governed by credits like anything else, so this reports
// the allowance only while it is the thing that applies.
func TurnsLeft(id string) int {
	mutex.Lock()
	defer mutex.Unlock()
	acc, ok := accounts[id]
	if !ok || !acc.Unclaimed {
		return 0
	}
	if left := FreeTurns() - acc.Turns; left > 0 {
		return left
	}
	return 0
}

// SpendTurn records that an unclaimed account has used one, and reports whether
// that was the last.
//
// Counted after the answer rather than before it, so somebody's tenth question
// is answered and *then* they are told — a limit that swallows the message it
// was reached on is a limit that looks like a fault.
func SpendTurn(id string) (spent bool) {
	mutex.Lock()
	defer mutex.Unlock()
	acc, ok := accounts[id]
	if !ok || !acc.Unclaimed {
		return false
	}
	acc.Turns++
	data.SaveJSON("accounts.json", accounts)
	return acc.Turns >= FreeTurns()
}

// Invited records that the sign-up invitation has gone out, so it goes out once.
//
// Without it every message after the limit sends another one, which is the
// difference between an invitation and being harassed by a mail server.
func Invited(id string) bool {
	mutex.Lock()
	defer mutex.Unlock()
	acc, ok := accounts[id]
	if !ok {
		return false
	}
	return !acc.InvitedAt.IsZero()
}

// MarkInvited stamps when the invitation was sent.
func MarkInvited(id string) {
	mutex.Lock()
	defer mutex.Unlock()
	if acc, ok := accounts[id]; ok {
		acc.InvitedAt = time.Now()
		data.SaveJSON("accounts.json", accounts)
	}
}

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
