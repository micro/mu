package auth

// Signing in with a username and an access token.
//
// Every protocol door does this: IMAP, SMTP submission, and now XMPP. It was
// service/mail's accountForToken, which was the right place while mail was the
// only thing with a login — and the wrong one the moment a second service
// needed it, because services never import each other.
//
// It belongs here rather than being written twice. Who an account is, and
// whether a credential proves it, is exactly what this package is for; the
// alternative was a second copy in service/chat that agrees until one of them
// is changed.

import (
	"errors"
	"strings"
)

// ErrBadCredentials is what every door says, whatever was wrong.
//
// One error for a missing username, a bad token and a token belonging to
// somebody else, because telling them apart tells somebody guessing which half
// they got right.
var ErrBadCredentials = errors.New("wrong username or access token")

// AccountForToken resolves a client's username and access token.
//
// The username may be a bare username, a full address, or a plus address: all
// three are what a person has in front of them when filling in a client, and
// asim+research@ is asim's own account — so refusing it would be refusing the
// address the product told them to use.
func AccountForToken(user, pass string) (*Account, error) {
	if strings.TrimSpace(user) == "" || pass == "" {
		return nil, ErrBadCredentials
	}

	local := user
	if i := strings.Index(local, "@"); i > 0 {
		local = local[:i]
	}
	// The part before any plus. Written here rather than borrowed from the
	// mail service's SplitAlias, which is the sideways import this move
	// exists to avoid.
	if i := strings.Index(local, "+"); i > 0 {
		local = local[:i]
	}

	accountID, err := ValidatePAT(pass)
	if err != nil || accountID == "" {
		return nil, ErrBadCredentials
	}
	acc, err := GetAccount(accountID)
	if err != nil || acc == nil {
		return nil, ErrBadCredentials
	}
	// Against the ID, which is the username. Name is a display name — free
	// text, not unique, and whatever the Google profile said. See
	// AccountByUsername, where the same mistake is written up.
	if !strings.EqualFold(acc.ID, local) {
		return nil, ErrBadCredentials
	}
	return acc, nil
}
