package auth

// Attaching a chat channel to an account.
//
// The obvious way to do this is "link <username> <password>", and all three
// channels used to. It is the wrong way. A password typed into WhatsApp is a
// password written into the user's own message history, into the recipient's,
// and into a third party's servers — durably, in plaintext, somewhere none of
// us control and none of us can delete it from. The user cannot unsay it, and
// the credential it exposes is the one that opens everything else.
//
// A one-time code inverts that. It is issued to a caller who has *already*
// proved who they are — they are signed in, looking at their own account page —
// so the chat message carries a claim rather than a secret. It is worth nothing
// after five minutes, worth nothing after one use, and worth nothing to anyone
// who did not just click the button.
//
// Codes live in memory only. A restart invalidates every outstanding one, which
// is the right failure: five minutes of inconvenience against durable storage
// of something that opens an account.

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// LinkCodeTTL is how long an issued code stays valid.
const LinkCodeTTL = 5 * time.Minute

type linkCode struct {
	account   string
	expiresAt time.Time
}

var (
	linkCodeMu sync.Mutex
	linkCodes  = map[string]*linkCode{}
)

// now is swappable so expiry can be tested without sleeping.
var now = time.Now

// GenerateLinkCode issues a one-time code that attaches a chat channel to an
// account. The caller must already be authenticated — this proves nothing on
// its own, it only carries a claim that was proved on the web.
//
// One code per account: issuing a second retires the first, so a code left in a
// scrollback by a user who clicked twice is already dead.
func GenerateLinkCode(accountID string) string {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return ""
	}

	linkCodeMu.Lock()
	defer linkCodeMu.Unlock()

	for k, v := range linkCodes {
		if v.account == accountID || now().After(v.expiresAt) {
			delete(linkCodes, k)
		}
	}

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Without randomness there is no code. Returning "" is refused by
		// RedeemLinkCode, so a failure here cannot become a guessable code.
		return ""
	}
	code := hex.EncodeToString(b)
	linkCodes[code] = &linkCode{account: accountID, expiresAt: now().Add(LinkCodeTTL)}
	return code
}

// RedeemLinkCode consumes a code and returns the account it was issued for.
//
// Any code is spent by the attempt, valid or not: a channel that reports "not
// yours, try again" is a channel someone can guess against.
func RedeemLinkCode(code string) (string, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "", false
	}

	linkCodeMu.Lock()
	defer linkCodeMu.Unlock()

	lc, ok := linkCodes[code]
	if !ok {
		return "", false
	}
	delete(linkCodes, code)
	if now().After(lc.expiresAt) {
		return "", false
	}
	return lc.account, true
}
