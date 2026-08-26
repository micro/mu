package auth

// Setting a password, which nothing could do until now.
//
// The word "password reset" appears in five comments across this repository —
// internal/auth/address.go says the sign-in address is "where a password reset
// goes", micro.go warns about mailing one to an agent, and /account prints the
// sentence under the email field. None of them were describing anything. There
// was no reset, no change and no set: a password was written once at signup and
// was thereafter a fact about the account that nobody could alter.
//
// For an account made through Google that is worse than an inconvenience. It is
// created with Secret: randToken(24) — a password nobody has ever seen, bcrypt
// hashed like any other. So CheckSecret does not report "this account has no
// password"; it reports that the one you typed is wrong, because a hash of a
// random string is a real hash. The wallet export page asks for that password,
// refuses whatever is typed, and advises setting one first, which was not
// possible. The key was unreachable to its owner.

import (
	"errors"
	"strings"

	"mu/internal/data"
)

// MinSecret is the shortest password this instance accepts.
//
// Six, because that is what the signup form has always said and asked for, and
// a second number here would be a second rule for one thing.
const MinSecret = 6

// SetSecret replaces an account's password.
//
// No current password is required, and the session is the authority instead.
// That is not a shortcut: whoever holds the session can already read this
// account's mail, spend its credits, change the address a reset would go to and
// delete it. Setting a password grants nothing that was not already held, and
// requiring the old one would lock out exactly the accounts that need this —
// the ones whose password is a random string they were never told.
//
// It does not sign anybody out. The username is unchanged, so the session still
// names somebody real; ending it would be theatre with a re-login attached.
func SetSecret(id, secret string) error {
	if len(strings.TrimSpace(secret)) < MinSecret {
		return errors.New("a password must be at least 6 characters")
	}

	hashed, err := hashSecret(secret)
	if err != nil {
		return err
	}

	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	if !ok {
		return errors.New("account does not exist")
	}
	acc.Secret = hashed
	acc.SecretSet = true
	data.SaveJSON("accounts.json", accounts)
	return nil
}

// HasSecret reports whether this account has a password its owner chose.
//
// Not "is Secret non-empty", which is true of every account including the ones
// created with a random one. SecretSet is recorded when somebody picks a
// password, so it answers the question the page actually asks: is there a
// password here that you could type.
//
// False for accounts that predate the flag, including ones that really do have
// a chosen password. That is the safe direction to be wrong in: the page offers
// to set one, and setting one over a password you already had is what the form
// is for anyway.
func HasSecret(id string) bool {
	mutex.Lock()
	defer mutex.Unlock()

	acc, ok := accounts[id]
	return ok && acc.SecretSet
}
