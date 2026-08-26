package chat

// The inbound half of federation: another server connecting here.
//
// Two different conversations arrive on this port and they are easy to confuse,
// because both are dialback and both mention two domains.
//
//	<db:result>  "I am example.org, here is a key, go and check it."
//	<db:verify>  "Somebody claiming to be you sent me this key. Is it yours?"
//
// The first is a server asserting itself to us; we answer it by opening our own
// connection to the domain it claims and asking. The second is that question
// arriving, about a key *we* issued, and we answer from the secret rather than
// from any state — which is why a restart in the middle of somebody else's
// handshake does not fail it.
//
// A server is authenticated per domain, not per connection: once example.org
// has proved itself on a stream, stanzas from example.org on that stream are
// delivered. Anything from a domain that has not proved itself is dropped, and
// dropped quietly — a federated port answers to the whole internet, and a
// detailed refusal is a detailed map for whoever is scanning.

import (
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"mu/internal/app"
)

// StartS2SIfEnabled serves federated connections unless turned off.
func StartS2SIfEnabled() {
	addr, on := app.ListenAddr("XMPP_S2S_PORT", s2sPort)
	if !on {
		return
	}
	go func() {
		if err := StartS2SServer(addr); err != nil {
			app.Log("chat", "xmpp s2s server error: %v", err)
		}
	}()
}

// StartS2SServer accepts federated connections on addr until it fails.
func StartS2SServer(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	app.Log("chat", "Starting XMPP federation on %s (dialback)", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					app.Log("chat", "s2s session panicked: %v", rec)
				}
				conn.Close()
			}()
			serveS2S(conn)
		}()
	}
}

// inStream is one connection from another server.
type inStream struct {
	conn net.Conn
	dec  *xml.Decoder
	id   string
	// from is the domain this stream has proved, empty until it has.
	from string
}

// serveS2S handles one inbound federated connection.
func serveS2S(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))

	s := &inStream{conn: conn, id: fmt.Sprintf("%d", time.Now().UnixNano())}
	if !s.open() {
		return
	}

	// STARTTLS if they ask. Offered rather than required, matching the outbound
	// side: this network still carries servers that do not speak it.
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		start, err := nextStartOf(s.dec)
		if err != nil {
			return
		}
		switch start.Name.Local {
		case "starttls":
			if !s.startTLS() {
				return
			}
		case "result":
			s.dialbackResult(start)
		case "verify":
			s.dialbackVerify(start)
		case "message":
			s.inboundMessage(start)
		case "presence", "iq":
			// Read past it. Presence between servers is real and this does not
			// carry it yet; an iq we do not answer is better than a malformed
			// one, and both are skipped the same way.
			_ = s.dec.Skip()
		default:
			_ = s.dec.Skip()
		}
	}
}

// open reads their stream header and sends ours.
func (s *inStream) open() bool {
	dec := xml.NewDecoder(s.conn)
	start, err := nextStartOf(dec)
	if err != nil || start.Name.Local != "stream" {
		return false
	}
	s.dec = dec

	var to string
	for _, a := range start.Attr {
		if a.Name.Local == "to" {
			to = a.Value
		}
	}
	// A stream addressed to a domain we do not serve is not ours to answer.
	if to != "" && !strings.EqualFold(to, Domain()) {
		return false
	}

	hello := fmt.Sprintf(`<?xml version='1.0'?><stream:stream xmlns='%s' `+
		`xmlns:stream='%s' xmlns:db='%s' from='%s' id='%s' version='1.0'>`+
		`<stream:features><starttls xmlns='urn:ietf:params:xml:ns:xmpp-tls'/>`+
		`</stream:features>`,
		nsServer, nsStream, nsDialback, xmlAttr(Domain()), s.id)
	_, err = io.WriteString(s.conn, hello)
	return err == nil
}

// startTLS upgrades the connection and restarts the stream.
func (s *inStream) startTLS() bool {
	cert, err := s2sCertificate()
	if err != nil {
		_, _ = io.WriteString(s.conn,
			`<failure xmlns='urn:ietf:params:xml:ns:xmpp-tls'/>`)
		app.Log("chat", "s2s: no certificate to offer: %v", err)
		return false
	}
	if _, err := io.WriteString(s.conn,
		`<proceed xmlns='urn:ietf:params:xml:ns:xmpp-tls'/>`); err != nil {
		return false
	}
	tlsConn := tls.Server(s.conn, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err := tlsConn.Handshake(); err != nil {
		return false
	}
	s.conn = tlsConn
	_ = s.conn.SetDeadline(time.Now().Add(2 * time.Minute))
	// The stream restarts after TLS, per RFC 6120 §5.4.3.3.
	return s.open()
}

// dialbackResult answers a server asserting who it is.
//
// We cannot check the key ourselves — it was made from *their* secret. So we
// open our own connection to the domain they claim and ask, which is the whole
// mechanism: the answer arrives over a link to the address DNS gave us for that
// domain, so a server that cannot receive at the domain it claims cannot pass.
func (s *inStream) dialbackResult(start xml.StartElement) {
	from, to := attrOf(start, "from"), attrOf(start, "to")
	key, err := textOf(s.dec, start)
	if err != nil {
		return
	}
	if from == "" || !strings.EqualFold(to, Domain()) {
		return
	}

	ok := askDialback(from, s.id, key)
	typ := "invalid"
	if ok {
		typ = "valid"
		s.from = strings.ToLower(from)
		app.Log("chat", "s2s: %s authenticated inbound", s.from)
	}
	_, _ = io.WriteString(s.conn, fmt.Sprintf(
		`<db:result xmlns:db='%s' from='%s' to='%s' type='%s'/>`,
		nsDialback, xmlAttr(Domain()), xmlAttr(from), typ))
}

// dialbackVerify answers "is this key one you issued".
//
// Recomputed from the secret rather than looked up, so there is no table of
// outstanding keys to keep, expire, or lose in a restart.
func (s *inStream) dialbackVerify(start xml.StartElement) {
	from, to, id := attrOf(start, "from"), attrOf(start, "to"), attrOf(start, "id")
	key, err := textOf(s.dec, start)
	if err != nil {
		return
	}
	typ := "invalid"
	if strings.EqualFold(to, Domain()) && verifyKey(Domain(), from, id, key) {
		typ = "valid"
	}
	_, _ = io.WriteString(s.conn, fmt.Sprintf(
		`<db:verify xmlns:db='%s' from='%s' to='%s' id='%s' type='%s'/>`,
		nsDialback, xmlAttr(Domain()), xmlAttr(from), xmlAttr(id), typ))
}

// inboundMessage delivers a message from a server that has proved itself.
func (s *inStream) inboundMessage(start xml.StartElement) {
	from, to := attrOf(start, "from"), attrOf(start, "to")
	var body struct {
		Body string `xml:"body"`
	}
	if err := s.dec.DecodeElement(&body, &start); err != nil {
		return
	}
	text := strings.TrimSpace(body.Body)
	if text == "" {
		return
	}

	// Unauthenticated, or claiming to be a domain other than the one it proved.
	// Dropped without a word: this port answers to the whole internet, and a
	// precise refusal tells somebody scanning exactly which lie to tell next.
	_, fromDomain := splitJID(strings.ToLower(from))
	if s.from == "" || !strings.EqualFold(fromDomain, s.from) {
		return
	}

	local := strings.ToLower(bareOf(to))
	if _, d := splitJID(local); !strings.EqualFold(d, Domain()) {
		return
	}
	if accountFor(local) == nil {
		return
	}

	// The record first, then the delivery — the same order a local message
	// takes, and for the same reason: being offline is the ordinary case.
	record(strings.ToLower(bareOf(from)), local, text)
	deliverXMPP(strings.ToLower(bareOf(from)), local, text)
}

// askDialback opens a connection to a domain and asks whether a key is theirs.
func askDialback(domain, streamID, key string) bool {
	for _, addr := range resolveS2S(domain) {
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err != nil {
			continue
		}
		ok := verifyOver(conn, domain, streamID, key)
		conn.Close()
		if ok {
			return true
		}
	}
	return false
}

// verifyOver runs one verification exchange on an open connection.
func verifyOver(conn net.Conn, domain, streamID, key string) bool {
	_ = conn.SetDeadline(time.Now().Add(dialTimeout))

	hello := fmt.Sprintf(`<?xml version='1.0'?><stream:stream xmlns='%s' `+
		`xmlns:stream='%s' xmlns:db='%s' from='%s' to='%s' version='1.0'>`,
		nsServer, nsStream, nsDialback, xmlAttr(Domain()), xmlAttr(domain))
	if _, err := io.WriteString(conn, hello); err != nil {
		return false
	}
	dec := xml.NewDecoder(conn)
	if start, err := nextStartOf(dec); err != nil || start.Name.Local != "stream" {
		return false
	}

	// Plain rather than TLS. This carries no secret — a key and a verdict — and
	// negotiating TLS here would double the handshake on the hot path of every
	// inbound connection. The key itself is what carries the proof.
	if _, err := io.WriteString(conn, fmt.Sprintf(
		`<db:verify xmlns:db='%s' from='%s' to='%s' id='%s'>%s</db:verify>`,
		nsDialback, xmlAttr(Domain()), xmlAttr(domain), xmlAttr(streamID), key)); err != nil {
		return false
	}

	deadline := time.Now().Add(dialTimeout)
	for time.Now().Before(deadline) {
		start, err := nextStartOf(dec)
		if err != nil {
			return false
		}
		if start.Name.Local != "verify" {
			continue
		}
		return attrOf(start, "type") == "valid"
	}
	return false
}

// attrOf is one attribute of a start element, by local name.
func attrOf(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// textOf is the character data inside an element.
func textOf(dec *xml.Decoder, start xml.StartElement) (string, error) {
	var v struct {
		Text string `xml:",chardata"`
	}
	if err := dec.DecodeElement(&v, &start); err != nil {
		return "", err
	}
	return strings.TrimSpace(v.Text), nil
}
