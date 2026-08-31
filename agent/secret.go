package agent

// A freshly minted token, handed to the next page without going through the URL.
//
// Issuing a token is a POST that redirects to the page that asked, and the
// secret has to survive that hop or the round trip ends with the token nowhere.
// It travelled as `?secret=…`, which put a bearer credential in the URL bar: in
// the browser's history, in whatever the history syncs to, and — the one we
// cannot do anything about from in here — in the access log of whatever
// terminates TLS in front of us. Caddy and nginx both log the full URI by
// default, and the normal self-hosted install has one of them in front, so a
// token that grants API access to somebody's agent was being written to
// /var/log in plaintext on the owner's own box.
//
// Our own request log records r.URL.Path and not RequestURI, so it never
// reached that one. That is exactly why this is worth stating: three of the four
// places a URL comes to rest are somebody else's, and the only reliable fix is
// not to put the secret in the URL.
//
// # In memory, once, briefly
//
// Not persisted. A secret written to disk so a page can render it is a secret
// with a second home, and the whole point of showing it once is that we do not
// keep it. If the process restarts between the POST and the GET, the token
// exists and was not shown — which is recoverable by issuing another, and is a
// better failure than the alternative.
//
// Single read, because the panel is "here it is, copy it now". A refresh that
// showed it again would make the browser's back button a way to recover a
// credential from a page somebody had already closed.

import (
	"sync"
	"time"
)

// secretTTL is how long a minted token waits to be collected.
//
// The gap it has to cover is one redirect, which is milliseconds. A minute is
// generous enough that a slow round trip never loses one, and short enough that
// an abandoned tab does not leave a live credential in memory all day.
const secretTTL = time.Minute

type heldSecret struct {
	value string
	at    time.Time
}

var (
	secretMu sync.Mutex
	held     = map[string]heldSecret{}
)

// secretKey scopes a held secret to one owner and one agent.
//
// Both halves, because the page that collects it is addressed by agent id and
// the owner is who is allowed to see it. Keyed on the agent alone, two people
// issuing a token for their own agent of the same name would collect each
// other's.
func secretKey(owner, agentID string) string { return owner + "\x00" + agentID }

// stashSecret holds a minted token for the redirect that follows.
func stashSecret(owner, agentID, secret string) {
	if owner == "" || agentID == "" || secret == "" {
		return
	}
	secretMu.Lock()
	defer secretMu.Unlock()
	sweepSecrets()
	held[secretKey(owner, agentID)] = heldSecret{value: secret, at: time.Now()}
}

// takeSecret collects it, once.
//
// Empty when there is nothing waiting, which is the normal case: every render
// of the page after the first one, and every arrival that did not just issue a
// token.
func takeSecret(owner, agentID string) string {
	if owner == "" || agentID == "" {
		return ""
	}
	secretMu.Lock()
	defer secretMu.Unlock()
	sweepSecrets()
	k := secretKey(owner, agentID)
	s, ok := held[k]
	if !ok {
		return ""
	}
	delete(held, k)
	return s.value
}

// sweepSecrets drops the ones nobody came back for. Caller holds secretMu.
//
// On every access rather than on a ticker: the map holds at most one entry per
// token issued in the last minute, so there is never enough in it for a scan to
// cost anything, and a goroutine would be machinery for a map that is usually
// empty.
func sweepSecrets() {
	cutoff := time.Now().Add(-secretTTL)
	for k, s := range held {
		if s.at.Before(cutoff) {
			delete(held, k)
		}
	}
}
