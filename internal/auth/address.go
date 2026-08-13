package auth

// Which email addresses an account has proved it can read.
//
// This lives with the account rather than in a service because it is already
// half here: acc.Email plus acc.EmailVerified is the same claim, made by
// clicking a link, and service/mail routes on it — mail from your own verified
// address is never spam, and it is the only claim strong enough to say whose
// mail arrived at the shared agent mailbox. Splitting "the first address you
// proved" from "the second" across two packages would leave the question of
// whether an address is yours with two answers.
//
// The difference between the two is worth keeping. **Email is the account's
// address**: one of them, where a password reset goes, changed at /account by
// clicking a link, and nothing below may touch it. **Addresses are the rest**:
// as many as you like, proved by a code, and what they buy is being recognised
// — your agent answering mail you send from your work address as readily as
// from the one you signed up with.
//
// This is not where email_verify's answers go. That tool checks whether one of
// the *caller's* users can read their own mail, and the caller's users are not
// accounts here; nothing reaches this list unless somebody asks for it, about
// an address they read themselves. The distinction matters because the one rule
// below that verification does not have — one address to one account — only
// makes sense on this side of it.
//
// That asymmetry is deliberate and it is a security boundary. Adding an address
// here cannot take an account over, because recovery reads Email alone; if this
// list ever became a way in, the code flow in service/email would be a way to
// re-point somebody's account at an address of your choosing.

import (
	"errors"
	"strings"
)

// NormaliseAddress reduces an address to one spelling, or to empty if it cannot
// be read as an address at all.
//
// Two spellings of one address are two addresses to every lookup there is, so
// this runs before anything is stored or compared — the same reason
// phone.Normalise exists. Deliberately not a full RFC 5322 parse: what is
// wanted is the plain form a person types, and anything cleverer would accept
// spellings that no lookup here would ever match.
func NormaliseAddress(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.ContainsAny(s, " \t\r\n<>,;\"'") {
		return ""
	}
	at := strings.Index(s, "@")
	if at <= 0 || strings.Count(s, "@") != 1 {
		return ""
	}
	domain := s[at+1:]
	if !strings.Contains(domain, ".") ||
		strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") ||
		strings.Contains(domain, "..") {
		return ""
	}
	return s
}

// Verified is every address this account record has proved is its own.
//
// A method on the record, so anything already holding one — an inbound filter
// deciding whose mail this is — can ask without a second lookup. The account's
// own email comes first when it has been verified, because it is the one a
// person means when they say "my email".
func (a *Account) Verified() []string {
	if a == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s = NormaliseAddress(s); s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if a.EmailVerified {
		add(a.Email)
	}
	for _, s := range a.Addresses {
		add(s)
	}
	return out
}

// Owns reports whether this account record has proved it can read an address.
func (a *Account) Owns(addr string) bool {
	addr = NormaliseAddress(addr)
	if a == nil || addr == "" {
		return false
	}
	for _, s := range a.Verified() {
		if s == addr {
			return true
		}
	}
	return false
}

// VerifiedAddresses is every address this account has proved is its own.
func VerifiedAddresses(id string) []string {
	mutex.Lock()
	defer mutex.Unlock()
	return accounts[id].Verified()
}

// OwnsAddress reports whether this account has proved it can read this address.
func OwnsAddress(id, addr string) bool {
	if id == "" {
		return false
	}
	mutex.Lock()
	defer mutex.Unlock()
	return accounts[id].Owns(addr)
}

// AddVerifiedAddress records that an account has proved it can read an address.
//
// Whoever calls this is asserting the proof happened. There is none of it here
// on purpose: the challenge belongs to whichever channel carried it, and this
// package has no way to send an email.
//
// One account per address, and this is the only place that rule holds. It is
// here because AccountForAddress decides whose mail arrived and two claimants
// make that a coin toss. Verification itself has no such rule and must not:
// two products may both have a user at the same address, and they do.
func AddVerifiedAddress(id, addr string) error {
	addr = NormaliseAddress(addr)
	if addr == "" {
		return errors.New("that does not look like an email address")
	}

	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return errors.New("account not found")
	}
	for _, other := range accounts {
		if other.ID != id && other.Owns(addr) {
			return errors.New("another account here has already proved that address is theirs")
		}
	}
	if acc.Owns(addr) {
		return nil
	}
	acc.Addresses = append(acc.Addresses, addr)
	return saveAccountsUnlocked()
}

// RemoveVerifiedAddress drops an address this account had proved.
//
// Reversible for the reason phone.Forget is: an address is not yours forever,
// people leave jobs, and somebody who has given one up should be able to say so
// without an argument. The account's own email is not removable here — that is
// the address it signs in with, and it is changed at /account by verifying a
// new one, which is a different operation with a different consequence.
func RemoveVerifiedAddress(id, addr string) error {
	addr = NormaliseAddress(addr)
	if addr == "" {
		return errors.New("that does not look like an email address")
	}

	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return errors.New("account not found")
	}
	if acc.EmailVerified && NormaliseAddress(acc.Email) == addr {
		return errors.New("that is the address on the account — change it at /account")
	}
	kept := acc.Addresses[:0]
	found := false
	for _, a := range acc.Addresses {
		if NormaliseAddress(a) == addr {
			found = true
			continue
		}
		kept = append(kept, a)
	}
	if !found {
		return nil
	}
	acc.Addresses = kept
	return saveAccountsUnlocked()
}

// AccountForAddress is the account that proved it can read this address, or nil.
func AccountForAddress(addr string) *Account {
	addr = NormaliseAddress(addr)
	if addr == "" {
		return nil
	}
	mutex.Lock()
	defer mutex.Unlock()
	for _, acc := range accounts {
		if acc.Owns(addr) {
			return acc
		}
	}
	return nil
}
