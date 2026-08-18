// Package push is web push: telling somebody something happened when they are
// not looking at the page.
//
// This is the piece that makes an inbox on a phone an inbox. Mail arrives at
// four in the morning, the agent answers it, and until now the only way to find
// out was to open the site and look — which means the product's whole claim,
// that the agent works whether or not you have the page open, was invisible to
// the person it works for.
//
// # Substrate, not a service
//
// No page, no tools, no entry in the catalogue. It keeps the company of
// internal/x402 and internal/quota: a mechanism the product uses, not a
// capability a caller chooses. An agent has no business subscribing a browser
// to notifications.
//
// # No dependencies
//
// Web push is two specifications and both are small: RFC 8292 signs a request
// so the push service knows who is asking (a P-256 JWT), and RFC 8291 encrypts
// the payload so the push service cannot read what it is delivering (ECDH to
// the browser's own key, then AES-128-GCM). Everything either needs is in the
// standard library and in golang.org/x/crypto, which is already here. A library
// for this would be a dependency carrying a network client and a retry policy
// we would then have to disagree with.
//
// End-to-end encryption is not optional decoration here: the payload is the
// subject line of somebody's mail, and it passes through Google's or Apple's
// servers on the way to the handset. They forward bytes they cannot read.
package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"mu/internal/settings"
)

// b64 is base64url with no padding, which is what every part of web push uses.
var b64 = base64.RawURLEncoding

// vapidTTL is how long a signed request stays valid. The specification caps it
// at 24 hours; twelve leaves room for a clock that is wrong in either direction.
const vapidTTL = 12 * time.Hour

// keys returns this instance's VAPID keypair, minting one the first time.
//
// One pair per instance, kept in settings, and it must not change: a browser's
// subscription is bound to the public key it was created with, so a new pair
// silently invalidates every subscription anybody has made. That is the whole
// reason it is stored rather than generated at boot.
func keys() (*ecdsa.PrivateKey, error) {
	if stored := strings.TrimSpace(settings.Get("VAPID_PRIVATE_KEY")); stored != "" {
		raw, err := b64.DecodeString(stored)
		if err != nil || len(raw) == 0 {
			return nil, fmt.Errorf("VAPID_PRIVATE_KEY is not base64url")
		}
		d := new(big.Int).SetBytes(raw)
		key := &ecdsa.PrivateKey{D: d}
		key.PublicKey.Curve = elliptic.P256()
		key.PublicKey.X, key.PublicKey.Y = elliptic.P256().ScalarBaseMult(raw)
		return key, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	// Left-padded to the curve's byte length, because a D with a leading zero
	// byte would otherwise come back a different number.
	d := make([]byte, 32)
	key.D.FillBytes(d)
	settings.Set("VAPID_PRIVATE_KEY", b64.EncodeToString(d))
	return key, nil
}

// PublicKey is the application server key a browser subscribes with, as
// base64url — the one string the page needs.
//
// Empty when a key cannot be minted or stored, which is what turns the whole
// feature off in the UI rather than offering a button that cannot work.
func PublicKey() string {
	key, err := keys()
	if err != nil {
		return ""
	}
	return b64.EncodeToString(publicBytes(&key.PublicKey))
}

// Configured reports whether this instance can send push notifications.
func Configured() bool { return PublicKey() != "" }

// publicBytes is a P-256 point in the uncompressed form every part of web push
// uses: 0x04 then X then Y, 65 bytes.
func publicBytes(pub *ecdsa.PublicKey) []byte {
	out := make([]byte, 65)
	out[0] = 4
	pub.X.FillBytes(out[1:33])
	pub.Y.FillBytes(out[33:65])
	return out
}

// authorization builds the VAPID header for one push endpoint.
//
// The audience is the endpoint's origin and nothing else — scheme and host, no
// path. A push service rejects a token whose aud does not match, and the path
// is the subscription itself, which is a secret it already has.
func authorization(endpoint, contact string) (string, error) {
	key, err := keys()
	if err != nil {
		return "", err
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("not a push endpoint: %q", endpoint)
	}

	header, err := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"aud": u.Scheme + "://" + u.Host,
		"exp": time.Now().Add(vapidTTL).Unix(),
		"sub": contact,
	})
	if err != nil {
		return "", err
	}

	signing := b64.EncodeToString(header) + "." + b64.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	// Raw r||s, fixed width. ASN.1 is what ecdsa.SignASN1 gives and is not what
	// JWS ES256 is: a push service reading a DER blob here sees a bad signature.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])

	return "vapid t=" + signing + "." + b64.EncodeToString(sig) +
		", k=" + b64.EncodeToString(publicBytes(&key.PublicKey)), nil
}

// contact is who to complain to about this instance's push traffic.
//
// A push service uses it when something is wrong at scale — too many messages,
// endpoints that never respond — and an instance with nobody reachable is one
// that gets cut off with no warning. mailto: with the mail domain when there is
// one, and the origin otherwise, because a valid absolute URI is required and a
// missing one is a rejected request rather than a missing nicety.
var contact = func() string { return "" }

// Contact is filled in by the server, which knows the mail domain and this
// instance's own address. A function rather than an import because this package
// must not depend on the product.
func Contact(f func() string) {
	if f != nil {
		contact = f
	}
}
