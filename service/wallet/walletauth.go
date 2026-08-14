package wallet

// Proving who you are with a key, without paying for the privilege.
//
// An agent's identity used to arrive only with a settled payment: pay, and
// callerIdentity knows you. That works for priced tools and fails completely
// for free ones — notes, files, docs, tasks, contacts and events cost nothing,
// so they never issue a 402, so no payment settles, so the caller has no
// identity and is refused. Thirty-odd tools an agent most needs were shut
// behind a door that only opened from the inside.
//
// So a wallet can now say who it is by signing, which costs nobody anything:
//
//	Authorization: Wallet <base64 json>
//
// carrying the address, when it was made, a nonce, and a signature over all
// three. One header on the first call — no nonce to fetch, no session to
// establish, no round trip. The frictionless part is not decoration: an agent
// that must make two calls before its first real one will simply not bother,
// and a scheme nobody adopts protects nothing.
//
// What stops a replay is the pair of conditions below: the message names this
// host and a moment in time, and a nonce is only ever accepted once. A stolen
// header is useless somewhere else, and useless here after five minutes.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthScheme is the Authorization prefix a wallet uses.
const AuthScheme = "Wallet"

// authWindow is how far a signature's timestamp may be from now.
//
// Generous enough for a clock that has drifted a little, short enough that a
// header captured off the wire is worthless by the time it is replayed.
const authWindow = 5 * time.Minute

// WalletAuth is what the caller sends, base64-encoded, after "Wallet ".
type WalletAuth struct {
	Address   string `json:"address"`
	Host      string `json:"host"`
	IssuedAt  int64  `json:"issuedAt"` // unix seconds
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

// authMessage is what actually gets signed.
//
// The host is in it so a signature made for one instance cannot be presented
// to another — without that, an operator who runs an instance could collect
// headers and replay them against micro.mu as their authors.
func authMessage(host, address string, issuedAt int64, nonce string) []byte {
	return []byte(fmt.Sprintf("mu-auth\nhost: %s\naddress: %s\nissuedAt: %d\nnonce: %s",
		strings.ToLower(strings.TrimSpace(host)),
		strings.ToLower(strings.TrimSpace(address)),
		issuedAt, nonce))
}

// SignAuth builds the Authorization header value for a wallet.
//
// Exported because the paying side is a client too: `mu agent` signs one of
// these on every call so that free, account-scoped tools work without anybody
// creating an account or thinking about it.
func SignAuth(bw *BaseWallet, host string) (string, error) {
	if bw == nil {
		return "", fmt.Errorf("no wallet")
	}
	key, err := hex.DecodeString(strings.TrimPrefix(bw.PrivateKey, "0x"))
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("bad wallet key")
	}

	nonceBytes := make([]byte, 16)
	if _, err := randRead(nonceBytes); err != nil {
		return "", err
	}
	auth := WalletAuth{
		Address:  bw.Address,
		Host:     strings.ToLower(strings.TrimSpace(host)),
		IssuedAt: time.Now().Unix(),
		Nonce:    toHex(nonceBytes),
	}

	sig, err := signHash(keccak256(authMessage(auth.Host, auth.Address, auth.IssuedAt, auth.Nonce)), key)
	if err != nil {
		return "", err
	}
	auth.Signature = "0x" + toHex(sig)

	b, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}
	return AuthScheme + " " + base64.StdEncoding.EncodeToString(b), nil
}

// VerifyAuth checks an Authorization header and returns the wallet address it
// proves, lowercased.
//
// Returns an error rather than an empty address on every failure, so a caller
// cannot mistake "not signed" for "signed by nobody".
func VerifyAuth(header, host string) (string, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header), AuthScheme))
	if raw == "" || !strings.HasPrefix(strings.TrimSpace(header), AuthScheme) {
		return "", fmt.Errorf("not a wallet authorization")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("authorization is not base64")
	}
	var auth WalletAuth
	if err := json.Unmarshal(data, &auth); err != nil {
		return "", fmt.Errorf("authorization is not JSON")
	}

	// Bound in time before anything expensive. A signature is cheap to make and
	// this is the check that stops an old one being useful.
	age := time.Since(time.Unix(auth.IssuedAt, 0))
	if age > authWindow || age < -authWindow {
		return "", fmt.Errorf("authorization is outside the %s window", authWindow)
	}
	// Bound in place. A signature for another host is somebody else's.
	if !strings.EqualFold(strings.TrimSpace(auth.Host), strings.TrimSpace(host)) {
		return "", fmt.Errorf("authorization was signed for %q, not %q", auth.Host, host)
	}
	if strings.TrimSpace(auth.Nonce) == "" {
		return "", fmt.Errorf("authorization has no nonce")
	}

	sig, err := hex.DecodeString(strings.TrimPrefix(auth.Signature, "0x"))
	if err != nil {
		return "", fmt.Errorf("signature is not hex")
	}
	got, err := ecdsaRecover(keccak256(authMessage(auth.Host, auth.Address, auth.IssuedAt, auth.Nonce)), sig)
	if err != nil {
		return "", fmt.Errorf("signature does not recover: %w", err)
	}
	// The address is not taken on trust from the body — it is whatever the
	// signature actually recovers to. Claiming one and signing with another
	// fails here.
	if !strings.EqualFold(got, auth.Address) {
		return "", fmt.Errorf("signature is by %s, not the claimed %s", got, auth.Address)
	}

	// Used once. Everything above is replayable on its own: the same header
	// sent twice inside the window would otherwise authenticate twice.
	if !seenNonces.use(strings.ToLower(auth.Address) + ":" + auth.Nonce) {
		return "", fmt.Errorf("authorization has already been used")
	}
	return strings.ToLower(got), nil
}

// nonceCache remembers which nonces have been spent, and forgets them once
// they are too old to be replayed anyway.
//
// In memory, which is the right size for the job: entries only need to outlive
// authWindow, and a restart makes every outstanding header expire early rather
// than accepting anything it should not.
type nonceCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

var seenNonces = &nonceCache{seen: map[string]time.Time{}}

// use records a nonce, returning false if it was already spent.
func (c *nonceCache) use(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	if len(c.seen) > 0 {
		for k, t := range c.seen {
			if now.Sub(t) > authWindow*2 {
				delete(c.seen, k)
			}
		}
	}
	if _, spent := c.seen[key]; spent {
		return false
	}
	c.seen[key] = now
	return true
}

// randRead is crypto/rand, named so the nonce path reads plainly.
func randRead(b []byte) (int, error) { return rand.Read(b) }

type walletSignerKeyType struct{}

// WalletSignerKey carries the address a request proved by signature.
//
// Verified once per request and stashed, rather than checked wherever it is
// needed: the nonce may only be spent once, so a second verification of the
// same header would correctly refuse it and the caller would look unauthorised
// halfway through their own request.
var WalletSignerKey = walletSignerKeyType{}

// SignerFrom returns the address this request proved, or "" if it proved none.
func SignerFrom(ctx context.Context) string {
	s, _ := ctx.Value(WalletSignerKey).(string)
	return s
}

// AuthenticateRequest verifies an Authorization: Wallet header once and returns
// a request carrying the proved address, plus the address itself.
//
// Returns the request unchanged when there is no wallet header or it does not
// verify — an unsigned or bad signature is simply not an identity, and saying
// so is the caller's business further along.
func AuthenticateRequest(r *http.Request, host string) (*http.Request, string) {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" || !strings.HasPrefix(h, AuthScheme) {
		return r, ""
	}
	addr, err := VerifyAuth(h, host)
	if err != nil || addr == "" {
		return r, ""
	}
	return r.WithContext(context.WithValue(r.Context(), WalletSignerKey, addr)), addr
}

// decodeKey parses a hex private key, with or without the 0x.
func decodeKey(s string) ([]byte, bool) {
	k, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(s), "0x"))
	if err != nil || len(k) != 32 {
		return nil, false
	}
	return k, true
}
