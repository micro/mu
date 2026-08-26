package chat

import (
	"strings"
	"testing"
)

// A key verifies for the exchange it was made for, and for nothing else.
//
// Dialback is the whole identity check on the federated port: a key that
// verified for the wrong domain pair, or for any stream id, would let anybody
// who saw one message assert somebody else's domain forever.
func TestADialbackKeyIsGoodForOneExchange(t *testing.T) {
	const (
		us    = "here.test"
		them  = "there.test"
		other = "elsewhere.test"
		id    = "stream-1"
	)

	key := dialbackKey(us, them, id)
	if key == "" {
		t.Fatal("no key")
	}

	// The exchange it was made for.
	if !verifyKey(them, us, id, key) {
		t.Fatal("the key does not verify for the exchange it was made for")
	}

	// Every way of being a different exchange.
	for _, tt := range []struct {
		name              string
		from, to, id, key string
	}{
		{"a different stream", them, us, "stream-2", key},
		{"a different claimant", other, us, id, key},
		{"a different target", them, other, id, key},
		{"a key from somewhere else", them, us, id, dialbackKey(us, other, id)},
		{"no key at all", them, us, id, ""},
		{"a key of the right shape", them, us, id, strings.Repeat("a", len(key))},
	} {
		if verifyKey(tt.from, tt.to, tt.id, tt.key) {
			t.Errorf("%s verified, and must not", tt.name)
		}
	}
}

// The key is not the secret, and the secret does not appear in it.
func TestTheKeyDoesNotCarryTheSecret(t *testing.T) {
	key := dialbackKey("here.test", "there.test", "s")
	secret := s2sSecret()
	if len(secret) == 0 {
		t.Fatal("no secret")
	}
	if strings.Contains(key, string(secret)) {
		t.Error("the dialback key contains the secret it was derived from")
	}
	// Two domains, one secret, different keys — or the key says nothing about
	// who it is for.
	if dialbackKey("here.test", "there.test", "s") == dialbackKey("here.test", "elsewhere.test", "s") {
		t.Error("the same key is issued for two different domains")
	}
}

// Where a domain's server is, and the fallback when DNS says nothing.
//
// A domain with no SRV record still has to be reachable, because most small
// XMPP servers publish none and are found at their own name on 5269.
func TestADomainWithNoSRVIsStillReachable(t *testing.T) {
	addrs := resolveS2S("nonexistent-domain-for-a-test.invalid")
	if len(addrs) == 0 {
		t.Fatal("no address at all for a domain with no SRV")
	}
	last := addrs[len(addrs)-1]
	if last != "nonexistent-domain-for-a-test.invalid:5269" {
		t.Errorf("the fallback is %q, want the domain on 5269", last)
	}
}

// The certificate is offered, and it is for this domain.
//
// Self-signed on purpose — dialback proves the domain and every federated peer
// skips verification here — but it still has to be a usable certificate, or
// STARTTLS fails and the connection with it.
func TestThereIsACertificateToOffer(t *testing.T) {
	cert, err := s2sCertificate()
	if err != nil {
		t.Fatalf("no certificate: %v", err)
	}
	if len(cert.Certificate) == 0 || cert.PrivateKey == nil {
		t.Fatal("the certificate is empty")
	}
	// Generated once, not per connection: a new key per inbound stream would be
	// a signature operation on the hot path of a public port.
	again, _ := s2sCertificate()
	if len(again.Certificate) == 0 || &again.Certificate[0][0] != &cert.Certificate[0][0] {
		t.Error("a fresh certificate is generated per call")
	}
}
