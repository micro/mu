package chat

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// A key verifies for the exchange it was made for, and for nothing else.
//
// Dialback is the whole identity check on the federated port: a key that
// verified for the wrong domain pair, or for any stream id, would let anybody
// who saw one message assert somebody else's domain forever.
//
// In the roles XEP-0220 uses, not the direction of a stanza. `from` and `to`
// reverse between issuing a key and being asked about it, and naming these
// after the stanza is how the two ends came to disagree while this test passed.
func TestADialbackKeyIsGoodForOneExchange(t *testing.T) {
	const (
		receiving   = "there.test"
		originating = "here.test"
		other       = "elsewhere.test"
		id          = "stream-1"
	)

	key := dialbackKey(receiving, originating, id)
	if key == "" {
		t.Fatal("no key")
	}

	// The exchange it was made for.
	if !verifyKey(receiving, originating, id, key) {
		t.Fatal("the key does not verify for the exchange it was made for")
	}

	// Every way of being a different exchange.
	for _, tt := range []struct {
		name                       string
		receiving, originating, id string
		key                        string
	}{
		{"a different stream", receiving, originating, "stream-2", key},
		{"a different receiver", other, originating, id, key},
		{"a different originator", receiving, other, id, key},
		{"the roles the other way round", originating, receiving, id, key},
		{"a key from somewhere else", receiving, originating, id, dialbackKey(other, originating, id)},
		{"no key at all", receiving, originating, id, ""},
		{"a key of the right shape", receiving, originating, id, strings.Repeat("a", len(key))},
	} {
		if verifyKey(tt.receiving, tt.originating, tt.id, tt.key) {
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

// A key handshakeOut issues is a key dialbackVerify accepts.
//
// The test that was missing, and the reason federation shipped broken twice.
// The tests above check dialbackKey against verifyKey, which is a pair agreeing
// with itself — they passed while every key this server issued was refused by
// this same server, because neither used the argument order either real call
// site used.
//
// So this drives the real handshake against a fake peer, and the peer answers
// the verification the way an authoritative server does: by handing the key
// back to our own dialbackVerify. Nothing here recomputes a key, which is the
// point — a test that builds the key itself is a test that passes when the
// server stops building it the same way.
func TestAKeyWeIssueIsAKeyWeAccept(t *testing.T) {
	const them = "far.test"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("no listener: %v", err)
	}
	defer ln.Close()

	// The fake peer: a server that speaks just enough to receive a dialback.
	verified := make(chan bool, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		dec := xml.NewDecoder(conn)
		if _, err := nextStartOf(dec); err != nil { // their stream header
			return
		}
		// No STARTTLS offered: encryption is not what is under test, and a real
		// TLS upgrade here would test the handshake around this one.
		_, _ = io.WriteString(conn, fmt.Sprintf(
			`<?xml version='1.0'?><stream:stream xmlns='%s' xmlns:stream='%s' `+
				`xmlns:db='%s' from='%s' id='%s' version='1.0'><stream:features/>`,
			nsServer, nsStream, nsDialback, them, "peer-stream-id"))

		start, err := nextStartOf(dec)
		if err != nil || start.Name.Local != "result" {
			verified <- false
			return
		}
		key, err := textOf(dec, start)
		if err != nil {
			verified <- false
			return
		}

		// What jabber.org does with the key: asks the domain that claims to
		// have issued it. Which is us, so it goes to our own verifier.
		reply := askOurselves(t, fmt.Sprintf(
			`<db:verify xmlns:db='%s' from='%s' to='%s' id='%s'>%s</db:verify>`,
			nsDialback, them, Domain(), "peer-stream-id", key))
		ok := strings.Contains(reply, `type='valid'`)
		verified <- ok

		typ := "invalid"
		if ok {
			typ = "valid"
		}
		_, _ = io.WriteString(conn, fmt.Sprintf(
			`<db:result xmlns:db='%s' from='%s' to='%s' type='%s'/>`,
			nsDialback, them, Domain(), typ))
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dialling the fake peer: %v", err)
	}
	defer conn.Close()

	link, err := handshakeOut(conn, them)
	select {
	case ok := <-verified:
		if !ok {
			t.Error("we refused a key we had just issued ourselves, so no server " +
				"can ever federate with this one")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the peer never got as far as verifying")
	}
	if err != nil {
		t.Fatalf("the handshake failed: %v", err)
	}
	if link == nil {
		t.Fatal("the handshake reported no error and produced no link")
	}
}

// And a key we did not issue is refused, or the check above proves nothing.
func TestAKeyWeDidNotIssueIsRefused(t *testing.T) {
	const them = "jabber.org"

	for _, tt := range []struct {
		name string
		key  string
	}{
		{"a key for another stream", dialbackKey(them, Domain(), "a-different-stream")},
		{"a key for another domain", dialbackKey("elsewhere.test", Domain(), "s1")},
		{"nothing at all", ""},
		{"the right shape", strings.Repeat("a", 64)},
	} {
		verdict := askOurselves(t, fmt.Sprintf(
			`<db:verify xmlns:db='%s' from='%s' to='%s' id='s1'>%s</db:verify>`,
			nsDialback, them, Domain(), tt.key))
		if strings.Contains(verdict, `type='valid'`) {
			t.Errorf("%s was accepted: anybody who guesses a stream id federates "+
				"as anybody", tt.name)
		}
	}
}

// askOurselves runs one db:verify through dialbackVerify and returns the reply.
func askOurselves(t *testing.T, stanza string) string {
	t.Helper()

	ours, theirs := net.Pipe()
	defer ours.Close()
	defer theirs.Close()

	replies := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		_ = theirs.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, _ := theirs.Read(buf)
		replies <- string(buf[:n])
	}()

	dec := xml.NewDecoder(strings.NewReader(stanza))
	start, err := nextStartOf(dec)
	if err != nil {
		t.Fatalf("the test stanza does not parse: %v", err)
	}
	s := &inStream{conn: ours, dec: dec, id: "our-stream-id"}
	s.dialbackVerify(start)

	select {
	case reply := <-replies:
		return reply
	case <-time.After(5 * time.Second):
		t.Fatal("dialbackVerify never answered")
		return ""
	}
}
