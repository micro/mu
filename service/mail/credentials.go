package mail

// What a mail client puts in the username and password boxes.
//
// Two protocols ask this question — IMAP to open a mailbox, submission to send
// — and they must answer it identically. They did not for as long as there was
// only one: IMAP compared the username against the account's *display* name,
// which is free text and not an identifier, so everybody who signed in with
// Google was refused. Writing the check twice would have been writing that bug
// twice.
//
// The password is an access token. Mu has no password — sign-in is a passkey or
// a link — so a token stands in, which is the app-password pattern and
// revocable on its own. See imap.go.

import (
	"errors"
	"strings"

	"mu/internal/auth"
)

// errBadCredentials is the only failure this returns.
//
// One error for both halves, on purpose. Which of the username and the token
// was wrong is information about somebody else's account, and a caller that
// distinguished them would leak it in whatever it said next.
var errBadCredentials = errors.New("that username or token was not accepted")

// accountForToken resolves a mail client's username and access token.
//
// The rule moved to internal/auth when a second protocol needed it — XMPP has
// the same login, and services never import each other, so a copy here would
// have been two implementations that agree until one is changed. This stays as
// the name the mail code calls it by.
func accountForToken(user, pass string) (*auth.Account, error) {
	acc, err := auth.AccountForToken(user, pass)
	if err != nil {
		return nil, errBadCredentials
	}
	return acc, nil
}

// ownsAddress reports whether this account may send as this address.
//
// The check that stops a token being a licence to forge. A token authenticates
// an account and says nothing about which address that account may put in MAIL
// FROM, so without this anybody holding one could send as anybody on the
// domain — including the addresses that carry password resets and sign-in
// links.
//
// A plus alias is the same person: asim+research@ is asim, and an agent's
// address is the one a mail client is most likely to be replying from.
func ownsAddress(acc *auth.Account, addr string) bool {
	if acc == nil {
		return false
	}
	addr = strings.TrimSpace(strings.Trim(addr, "<>"))
	at := strings.LastIndex(addr, "@")
	if at <= 0 {
		return false
	}
	local, domain := addr[:at], addr[at+1:]
	if !strings.EqualFold(domain, ConfiguredDomain()) {
		return false
	}
	local, _ = SplitAlias(local)
	return strings.EqualFold(local, acc.ID)
}
