package chat

import (
	"encoding/xml"
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

// The check refuses the two domains that would make it lie.
//
// Both would report success without proving anything. An empty domain never
// dials, and this instance's own domain would exercise a loopback — the far
// side of a real handshake is the point, and a server agreeing with itself is
// exactly the test that federation did not have.
//
// The error has to name the reason, not merely be an error. Dialling our own
// domain fails on its own in a test environment, so an assertion that something
// went wrong passes just as well with the guard deleted — which is the shape of
// test that lets a guard rot out from under it.
func TestTheFederationCheckRefusesADomainThatProvesNothing(t *testing.T) {
	if _, err := CheckFederation("  "); err == nil {
		t.Error("an empty domain was accepted")
	} else if !strings.Contains(err.Error(), "no domain") {
		t.Errorf("an empty domain failed for the wrong reason: %v", err)
	}

	// Whatever this instance calls itself, in either case.
	for _, self := range []string{Domain(), strings.ToUpper(Domain())} {
		_, err := CheckFederation(self)
		if err == nil {
			t.Errorf("%q was accepted: the check dialled itself", self)
			continue
		}
		if !strings.Contains(err.Error(), "this instance") {
			t.Errorf("%q was refused for the wrong reason, so the guard is not "+
				"what refused it: %v", self, err)
		}
	}
}

// A domain that does not resolve fails, and says what it tried.
//
// The error is the whole product of a failed check — it is what an operator
// reads on /admin/diagnostics — so an error that only says "failed" sends them
// to the server log to find out which address was involved.
func TestAFailedCheckNamesWhatItTried(t *testing.T) {
	const bad = "nonexistent-domain-for-a-test.invalid"
	if _, err := CheckFederation(bad); err == nil {
		t.Fatal("dialback completed with a domain that does not exist")
	} else if !strings.Contains(err.Error(), bad+":5269") {
		t.Errorf("the error does not name the address it tried: %v", err)
	}
}

// Reading the offer leaves the decoder after </stream:features>.
//
// The position is the whole point, not the answer. offersStartTLS used to
// return the moment it saw the offer, which left the decoder inside the
// element — and since the handshake keeps reading from that same decoder, the
// reply to our <starttls/> was read as whatever child came next. Prosody
// advertises <starttls><required/></starttls>, so a real server got
// "starttls refused: <required>" while doing nothing wrong.
func TestReadingTheStartTLSOfferConsumesTheWholeFeatures(t *testing.T) {
	const stream = `<stream:stream xmlns:stream='http://etherx.jabber.org/streams'>` +
		`<stream:features>` +
		`<starttls xmlns='urn:ietf:params:xml:ns:xmpp-tls'><required/></starttls>` +
		`<dialback xmlns='urn:xmpp:features:dialback'/>` +
		`</stream:features>` +
		`<proceed xmlns='urn:ietf:params:xml:ns:xmpp-tls'/>`

	dec := xml.NewDecoder(strings.NewReader(stream))
	if _, err := nextStartOf(dec); err != nil { // the stream header
		t.Fatalf("no stream header: %v", err)
	}
	if !offersStartTLS(dec) {
		t.Fatal("the offer was not seen")
	}

	// What a real server sends next, and the only thing that should be read
	// next. Anything else means features was left half-read.
	next, err := nextStartOf(dec)
	if err != nil {
		t.Fatalf("reading past the features: %v", err)
	}
	if next.Name.Local != "proceed" {
		t.Errorf("after the offer the next element is <%s>, want <proceed> — the "+
			"decoder is still inside stream:features", next.Name.Local)
	}
}

// A server offering no TLS is still left in a readable place.
//
// The other half of the same property: opportunistic means the handshake
// carries on unencrypted, and it carries on reading from this decoder.
func TestFeaturesWithNoOfferAreAlsoConsumed(t *testing.T) {
	const stream = `<stream:stream xmlns:stream='http://etherx.jabber.org/streams'>` +
		`<stream:features><dialback xmlns='urn:xmpp:features:dialback'/></stream:features>` +
		`<db:result xmlns:db='jabber:server:dialback' type='valid'/>`

	dec := xml.NewDecoder(strings.NewReader(stream))
	if _, err := nextStartOf(dec); err != nil {
		t.Fatalf("no stream header: %v", err)
	}
	if offersStartTLS(dec) {
		t.Fatal("an offer was found where there is none")
	}
	next, err := nextStartOf(dec)
	if err != nil {
		t.Fatalf("reading past the features: %v", err)
	}
	if next.Name.Local != "result" {
		t.Errorf("after the features the next element is <%s>, want <result>", next.Name.Local)
	}
}
