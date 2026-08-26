package chat

// Server to server: the half that makes this a network rather than a building.
//
// Until now a message to somebody on another domain got remote-server-not-found,
// which was honest and final. This is RFC 6120 §4 with XEP-0220 dialback, in
// both directions: outbound so somebody here can write to a JID anywhere, and
// inbound so anywhere can write back.
//
// It is deliberately not a Mu-to-Mu protocol. The value is not two of these
// talking to each other — MCP over HTTP already does that, and x402 already
// pays across it. The value is that an account here can message somebody on a
// Prosody or ejabberd server that has never heard of this software, the same
// way mail already reaches any domain with an MX record. Federating with the
// world is worth the work; federating with ourselves would be a protocol we own,
// which is the thing this product is trying not to need.
//
// # Why dialback rather than certificates
//
// SASL EXTERNAL proves a domain with a certificate, which means a certificate
// per XMPP domain and a CA that both sides trust. Dialback proves it with DNS
// instead: we send the far side a key, it asks the domain we claim to be
// whether that key is ours, and the answer arrives over a connection it opened
// itself to the address DNS gave it. Weaker than a certificate and enormously
// easier to deploy, which is why the federated XMPP network runs on it.
//
// The consequence is that TLS here does not verify the peer certificate, and
// that is correct rather than a lapse: the encryption is worth having, the
// certificate is not what establishes identity, and requiring a valid one would
// refuse most of the servers this exists to reach. Dialback is the identity
// check; see verifyKey.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/data"
)

const (
	nsServer   = "jabber:server"
	nsDialback = "jabber:server:dialback"
)

// s2sPort is where federated servers meet, and it is not configurable per RFC.
//
// 5269 is what the SRV record points at by convention and what every other
// server dials. An operator can turn it off with XMPP_S2S_PORT=off, or move it
// for a test, but moving it in production means publishing a SRV record that
// says so and hoping the far side honours one.
const s2sPort = ":5269"

// dialTimeout bounds reaching a remote server.
//
// Ten seconds because this runs while somebody waits for a message to send, and
// a domain that is down should fail visibly rather than hang the client. A
// stanza error after ten seconds is a message marked undelivered, which is true.
const dialTimeout = 10 * time.Second

// ── The shared secret behind every dialback key ─────────────────

var (
	secretOnce sync.Once
	secretVal  []byte
)

// s2sSecret is this instance's dialback secret.
//
// Never leaves the process and is never sent: the keys derived from it are what
// go on the wire, and the far side cannot invert one. Persisted rather than
// generated per boot because a restart mid-handshake would otherwise fail a
// verification for a key we really did issue.
func s2sSecret() []byte {
	secretOnce.Do(func() {
		var stored struct {
			Secret string `json:"secret"`
		}
		if err := data.LoadJSON("xmpp_s2s.json", &stored); err == nil && stored.Secret != "" {
			if b, err := hex.DecodeString(stored.Secret); err == nil && len(b) >= 32 {
				secretVal = b
				return
			}
		}
		secretVal = make([]byte, 32)
		if _, err := rand.Read(secretVal); err != nil {
			// Cannot happen on any platform this runs on, and if it did, a
			// predictable dialback secret is worse than no federation.
			app.Log("chat", "s2s: no randomness for the dialback secret: %v", err)
			secretVal = nil
			return
		}
		stored.Secret = hex.EncodeToString(secretVal)
		data.SaveJSON("xmpp_s2s.json", stored)
	})
	return secretVal
}

// dialbackKey is the key XEP-0220 §2.1.1 asks for.
//
// HMAC-SHA256 over the two domains and the stream id, keyed by the SHA-256 of
// the secret. The stream id is in it so a key is good for one handshake and not
// for the domain pair forever.
func dialbackKey(from, to, streamID string) string {
	secret := s2sSecret()
	if len(secret) == 0 {
		return ""
	}
	sum := sha256.Sum256(secret)
	mac := hmac.New(sha256.New, []byte(hex.EncodeToString(sum[:])))
	fmt.Fprintf(mac, "%s %s %s", strings.ToLower(to), strings.ToLower(from), streamID)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyKey answers whether a key is one we issued.
//
// Constant time, because this is a signature check and the timing of a
// comparison is a way to learn a signature one byte at a time.
func verifyKey(from, to, streamID, key string) bool {
	want := dialbackKey(to, from, streamID)
	if want == "" || key == "" {
		return false
	}
	return hmac.Equal([]byte(want), []byte(key))
}

// ── Finding the far side ────────────────────────────────────────

// resolveS2S is where a domain's XMPP server lives.
//
// SRV first, per RFC 6120 §3.2: _xmpp-server._tcp names the host and port and is
// how a domain points chat somewhere other than itself — the same indirection MX
// gives mail. The domain itself on 5269 is the fallback, because plenty of small
// servers publish no SRV and are reached at their own name.
func resolveS2S(domain string) []string {
	var out []string
	if _, recs, err := net.LookupSRV("xmpp-server", "tcp", domain); err == nil {
		for _, r := range recs {
			host := strings.TrimSuffix(r.Target, ".")
			if host == "" || host == "." {
				continue // RFC 2782's "no service here"
			}
			out = append(out, fmt.Sprintf("%s:%d", host, r.Port))
		}
	}
	return append(out, domain+s2sPort)
}

// ── Outbound ────────────────────────────────────────────────────

// outLink is an authenticated stream to one remote domain.
type outLink struct {
	mu     sync.Mutex
	conn   net.Conn
	domain string
	opened time.Time
}

var (
	outMu    sync.Mutex
	outLinks = map[string]*outLink{}
)

// SendRemote delivers one message to a JID on another server.
//
// Returns an error the caller turns into a stanza error, because a message that
// silently fails is one a client renders as delivered.
func SendRemote(from, to, text string) error {
	_, domain := splitJID(strings.ToLower(to))
	if domain == "" || strings.EqualFold(domain, Domain()) {
		return fmt.Errorf("not a remote address: %s", to)
	}

	link, err := linkTo(domain)
	if err != nil {
		return err
	}

	stanza := fmt.Sprintf(
		`<message xmlns='%s' type='chat' from='%s' to='%s'><body>%s</body></message>`,
		nsServer, xmlAttr(from), xmlAttr(to), xmlText(text))

	link.mu.Lock()
	defer link.mu.Unlock()
	_ = link.conn.SetWriteDeadline(time.Now().Add(dialTimeout))
	if _, err := io.WriteString(link.conn, stanza); err != nil {
		// A link that fails mid-write is finished. Dropping it means the next
		// send redials rather than writing into a socket the far side closed
		// while nothing was being sent, which is the ordinary way an idle
		// federated link ends.
		dropLink(domain)
		return fmt.Errorf("sending to %s: %w", domain, err)
	}
	return nil
}

// CheckFederation completes a handshake with a real server and reports on it.
//
// The diagnostic that federation was missing. Everything else here is only
// reachable by sending a message, which needs an account, a client, and
// somebody at the other end to receive it — three things that have nothing to
// do with whether the handshake works, and each of which can fail on its own
// and look like federation failing.
//
// Dialback needs none of them. Authenticating a link to a domain exercises the
// whole mechanism — SRV, the outbound dial, and the far side's verification
// call arriving back here — and a domain that has never heard of this instance
// works as well as one that has. Which is the point: the check is real traffic
// with a real server, not a loopback.
//
// Deliberately not pooled. This is asked for by somebody looking at a page
// wanting to know the state now, so a cached link from an hour ago is the
// wrong answer even when it is a true one.
func CheckFederation(domain string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return "", errors.New("no domain to check")
	}
	if strings.EqualFold(domain, Domain()) {
		return "", fmt.Errorf("%s is this instance: federation is what happens between two of them", domain)
	}
	if _, on := app.ListenAddr("XMPP_S2S_PORT", s2sPort); !on {
		return "", errors.New("XMPP_S2S_PORT is off, so nothing is federating")
	}

	addrs := resolveS2S(domain)
	start := time.Now()
	var lastErr error
	for _, addr := range addrs {
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err != nil {
			lastErr = err
			continue
		}
		l, err := handshakeOut(conn, domain)
		if err != nil {
			conn.Close()
			lastErr = err
			continue
		}
		// Closed rather than kept. A link opened to answer a question is not
		// one anybody is going to send on, and leaving it in the pool means the
		// next real send inherits a socket the far side may already have timed
		// out.
		l.conn.Close()
		return fmt.Sprintf("dialback with %s completed via %s in %s",
			domain, addr, time.Since(start).Round(time.Millisecond)), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", domain)
	}
	return "", fmt.Errorf("tried %s: %w", strings.Join(addrs, ", "), lastErr)
}

// linkTo is an authenticated link to a domain, dialling one if there is none.
func linkTo(domain string) (*outLink, error) {
	outMu.Lock()
	if l, ok := outLinks[domain]; ok {
		outMu.Unlock()
		return l, nil
	}
	outMu.Unlock()

	l, err := dialS2S(domain)
	if err != nil {
		return nil, err
	}

	outMu.Lock()
	// Somebody else may have dialled while we were. Theirs wins and ours is
	// closed, so there is one link per domain rather than one per racing send.
	if existing, ok := outLinks[domain]; ok {
		outMu.Unlock()
		l.conn.Close()
		return existing, nil
	}
	outLinks[domain] = l
	outMu.Unlock()
	return l, nil
}

func dropLink(domain string) {
	outMu.Lock()
	l, ok := outLinks[domain]
	delete(outLinks, domain)
	outMu.Unlock()
	if ok && l.conn != nil {
		l.conn.Close()
	}
}

// dialS2S opens a stream to a remote domain and authenticates it by dialback.
func dialS2S(domain string) (*outLink, error) {
	var lastErr error
	for _, addr := range resolveS2S(domain) {
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err != nil {
			lastErr = err
			continue
		}
		l, err := handshakeOut(conn, domain)
		if err != nil {
			conn.Close()
			lastErr = err
			continue
		}
		app.Log("chat", "s2s: authenticated to %s via %s", domain, addr)
		return l, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", domain)
	}
	return nil, fmt.Errorf("connecting to %s: %w", domain, lastErr)
}

// handshakeOut opens the stream, upgrades it, and proves who we are.
func handshakeOut(conn net.Conn, domain string) (*outLink, error) {
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))

	open := func(c net.Conn) (*xml.Decoder, string, error) {
		hello := fmt.Sprintf(`<?xml version='1.0'?><stream:stream xmlns='%s' `+
			`xmlns:stream='%s' xmlns:db='%s' from='%s' to='%s' version='1.0'>`,
			nsServer, nsStream, nsDialback, xmlAttr(Domain()), xmlAttr(domain))
		if _, err := io.WriteString(c, hello); err != nil {
			return nil, "", err
		}
		dec := xml.NewDecoder(c)
		start, err := nextStartOf(dec)
		if err != nil {
			return nil, "", err
		}
		if start.Name.Local != "stream" {
			return nil, "", fmt.Errorf("expected a stream, got <%s>", start.Name.Local)
		}
		var id string
		for _, a := range start.Attr {
			if a.Name.Local == "id" {
				id = a.Value
			}
		}
		return dec, id, nil
	}

	dec, streamID, err := open(conn)
	if err != nil {
		return nil, err
	}

	// STARTTLS when it is offered. Opportunistic rather than required: the
	// federated network still carries servers that do not offer it, and
	// refusing them would be choosing not to reach the people on them.
	if offersStartTLS(dec) {
		if _, err := io.WriteString(conn,
			`<starttls xmlns='urn:ietf:params:xml:ns:xmpp-tls'/>`); err != nil {
			return nil, err
		}
		start, err := nextStartOf(dec)
		if err != nil {
			return nil, err
		}
		if start.Name.Local != "proceed" {
			return nil, fmt.Errorf("starttls refused: <%s>", start.Name.Local)
		}
		// InsecureSkipVerify is deliberate — see the file comment. Dialback,
		// not the certificate, is what proves the domain, and requiring a valid
		// chain here would refuse most of the servers this exists to reach.
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName:         domain,
			InsecureSkipVerify: true, //nolint:gosec // dialback proves the domain; see verifyKey
		})
		if err := tlsConn.Handshake(); err != nil {
			return nil, fmt.Errorf("tls to %s: %w", domain, err)
		}
		conn = tlsConn
		_ = conn.SetDeadline(time.Now().Add(dialTimeout))
		if dec, streamID, err = open(conn); err != nil {
			return nil, err
		}
	}

	// Dialback. We assert the domain and hand over a key; the far side asks us
	// — over a connection it opens itself — whether we issued it.
	key := dialbackKey(Domain(), domain, streamID)
	if key == "" {
		return nil, fmt.Errorf("no dialback secret")
	}
	if _, err := io.WriteString(conn, fmt.Sprintf(
		`<db:result xmlns:db='%s' from='%s' to='%s'>%s</db:result>`,
		nsDialback, xmlAttr(Domain()), xmlAttr(domain), key)); err != nil {
		return nil, err
	}

	// Read until the verdict. Stream features and whatever else the far side
	// sends first are skipped rather than parsed: nothing before the result
	// changes what we do next.
	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		start, err := nextStartOf(dec)
		if err != nil {
			return nil, err
		}
		if start.Name.Local != "result" {
			continue
		}
		var typ string
		for _, a := range start.Attr {
			if a.Name.Local == "type" {
				typ = a.Value
			}
		}
		if typ != "valid" {
			return nil, fmt.Errorf("%s refused our dialback: type=%q", domain, typ)
		}
		_ = conn.SetDeadline(time.Time{})
		return &outLink{conn: conn, domain: domain, opened: time.Now()}, nil
	}
	return nil, fmt.Errorf("%s never answered the dialback", domain)
}

// offersStartTLS reports whether the features we just read include it.
func offersStartTLS(dec *xml.Decoder) bool {
	start, err := nextStartOf(dec)
	if err != nil || start.Name.Local != "features" {
		return false
	}

	// Drained to </stream:features> whatever the answer, rather than returning
	// the moment the offer is found.
	//
	// This decoder is the one the rest of the handshake reads from, so leaving
	// it parked inside an element means the next read returns a child of it.
	// Prosody offers <starttls><required/></starttls>, so returning early left
	// <required/> in the buffer and the reply to our <starttls/> was read as
	// that — "starttls refused: <required>" against a server doing nothing
	// wrong. Every federated server that requires TLS advertises it that way,
	// which is most of them.
	found := false
	depth := 1
	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			return false
		}
		switch el := t.(type) {
		case xml.StartElement:
			if el.Name.Local == "starttls" {
				found = true
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return found
}

// nextStartOf reads to the next start element on a decoder.
func nextStartOf(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		t, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if start, ok := t.(xml.StartElement); ok {
			return start, nil
		}
	}
}

// ── The certificate we present ──────────────────────────────────

var (
	certOnce sync.Once
	certVal  tls.Certificate
	certErr  error
)

// s2sCertificate is what this server offers when a peer asks for STARTTLS.
//
// Self-signed, generated once and kept in memory. That looks wrong and is not:
// on this port the certificate is not what proves anything. Dialback is — the
// far side asks DNS for our domain and checks the key with whoever answers —
// and every federated server that speaks dialback skips certificate
// verification here for exactly that reason, as this one does outbound.
//
// So what a certificate buys on this port is encryption, and a self-signed one
// buys all of it. What it does not buy is the stronger identity SASL EXTERNAL
// would give, which needs a real certificate for the XMPP domain and a CA both
// sides trust. That is the upgrade path; this is the thing that works today
// without one, and without an operator having to obtain anything.
func s2sCertificate() (tls.Certificate, error) {
	certOnce.Do(func() {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			certErr = err
			return
		}
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if err != nil {
			certErr = err
			return
		}
		domain := Domain()
		tmpl := x509.Certificate{
			SerialNumber: serial,
			Subject:      pkix.Name{CommonName: domain},
			DNSNames:     []string{domain},
			NotBefore:    time.Now().Add(-time.Hour),
			// Ten years. A federated link that starts failing because a
			// certificate nobody validates expired would be a mystery outage
			// with no cause anybody could find.
			NotAfter:              time.Now().AddDate(10, 0, 0),
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			BasicConstraintsValid: true,
		}
		der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
		if err != nil {
			certErr = err
			return
		}
		certVal = tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	})
	return certVal, certErr
}
