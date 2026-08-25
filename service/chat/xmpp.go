package chat

// XMPP: the same address, in real time.
//
// # One address, two protocols
//
// asim@micro.mu is a mailbox and a chat address. Not two accounts that happen
// to share a spelling — one account, one local part, one domain, reachable two
// ways. Mail leaves it in a mailbox to be read later; XMPP hands it to whatever
// that account has connected right now, and leaves it in the same record either
// way.
//
// That falls straight out of what the address already is. service/mail does not
// own asim@micro.mu; internal/auth owns the account and mail is one way to
// reach it. This is another, and it needs no new namespace to be another.
//
// The agent addresses come along unchanged. A JID localpart may contain "+"
// (RFC 7622 forbids "&'/:<>@ and not this), so asim+research@micro.mu is a
// valid JID as well as a valid mail address, and it is the same agent at the
// end of it. agent@micro.mu likewise. Somebody who learned one address has
// learned the other, which is the whole argument for an address being the
// smallest interface there is.
//
// # Why serve it rather than integrate
//
// The same reason this instance runs its own SMTP rather than calling a mail
// provider. A protocol is a thing anybody's existing client already speaks, so
// serving one is how somebody uses this without being asked to adopt anything.
// Conversations, Dino, Gajim and Monal are clients for this the day it works.
//
// And it replaces something hand-rolled rather than adding a channel. The rooms
// in this package are a bespoke websocket protocol with a bespoke roster; MUC
// is the standard version of exactly that, and presence is a thing this has
// never had and never has to write.
//
// # What the agent is here
//
// A participant, not a gateway. The agent is reachable at a JID like anything
// else, and a message to it is announced on event.ChatForAgent for agent/chat
// to answer — the same seam the websocket rooms now use, so this door needed no
// second one. A room with no agent in it is just a room, and everything here
// works with the model switched off, which is the test for whether a protocol
// belongs in this product at all.
//
// # TLS
//
// None here, for the reason imap.go and submission.go both give: nothing in
// this repository terminates TLS. The operator puts the same proxy in front on
// 5223 — direct TLS, the same shape as 993 and 465 — and binds this listener to
// loopback so the plaintext port is not reachable from outside. There is no
// STARTTLS on 5222 and none is advertised: a client told to use it there would
// send its token in the clear believing otherwise. docs/INSTALL.md has the
// nginx stream block and the SRV record a client needs to find any of it.
//
// # Storage
//
// internal/thread, and no store of its own — see xmpp_record.go.

import (
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/settings"
)

// stream namespaces, spelled once.
const (
	nsStream = "http://etherx.jabber.org/streams"
	nsClient = "jabber:client"
	nsSASL   = "urn:ietf:params:xml:ns:xmpp-sasl"
	nsBind   = "urn:ietf:params:xml:ns:xmpp-bind"
	nsSess   = "urn:ietf:params:xml:ns:xmpp-session"
	nsRoster = "jabber:iq:roster"
	// nsFraming is the WebSocket subprotocol's own namespace: <open/> and
	// <close/> stand in for the stream tags a socket uses. RFC 7395.
	nsFraming = "urn:ietf:params:xml:ns:xmpp-framing"
)

// StartXMPPServer serves XMPP client connections on addr until it fails.
func StartXMPPServer(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	app.Log("chat", "Starting XMPP server on %s", addr)
	app.Log("chat", "  - Log in with your username and an access token as the password")
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					app.Log("chat", "xmpp session panicked: %v", rec)
				}
				conn.Close()
			}()
			serveXMPP(conn)
		}()
	}
}

// StartXMPPServerIfEnabled starts XMPP unless it is turned off.
func StartXMPPServerIfEnabled() {
	addr, on := app.ListenAddr("XMPP_PORT", app.XMPPPort)
	if !on {
		return
	}
	go func() {
		if err := StartXMPPServer(addr); err != nil {
			app.Log("chat", "xmpp server error: %v", err)
		}
	}()
}

// carrier is how stanzas reach a session.
//
// Two of them: a TCP socket, and a WebSocket for the browser. They differ in
// exactly two ways — how a stanza is framed on the wire, and how a stream is
// opened — and in nothing else. Everything above this interface is the protocol
// and does not know which one it is on, which is the point: the browser stops
// being a second protocol and becomes a second way in to the first.
type carrier interface {
	// Read gives the decoder its bytes. A WebSocket delivers one stanza per
	// message and a socket delivers a stream; both read the same from here.
	Read(p []byte) (int, error)
	// writeStanza sends one stanza. Not io.Writer, because a WebSocket has to
	// know where a stanza ends and a socket does not care.
	writeStanza(string) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	Close() error
	// framed reports RFC 7395 framing: <open/> rather than <stream:stream>.
	framed() bool
}

// socket is the TCP carrier.
type socket struct{ net.Conn }

func (c socket) writeStanza(s string) error {
	_, err := io.WriteString(c.Conn, s)
	return err
}

func (socket) framed() bool { return false }

// session is one connected client.
type session struct {
	conn     carrier
	dec      *xml.Decoder
	acc      *auth.Account
	resource string
	// remote is who this is, for the log and for the error a stranger sees.
	remote string
}

// jid is this session's full address, once it has one.
func (s *session) jid() string {
	if s.acc == nil {
		return ""
	}
	bare := s.acc.ID + "@" + Domain()
	if s.resource == "" {
		return bare
	}
	return bare + "/" + s.resource
}

func (s *session) bare() string {
	if s.acc == nil {
		return ""
	}
	return s.acc.ID + "@" + Domain()
}

// send writes one stanza.
//
// Formatted here rather than written straight out, because a WebSocket carrier
// needs the whole stanza in one call: a stanza split across two writes is two
// messages, and RFC 7395 says one message is one stanza.
func (s *session) send(format string, args ...interface{}) error {
	_ = s.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return s.conn.writeStanza(fmt.Sprintf(format, args...))
}

// serveXMPP negotiates a stream and then routes stanzas until the client goes.
//
// Written against RFC 6120 rather than a library, for the same reason the SMTP
// and IMAP servers here are: the part of the protocol a real client needs is
// small, and a dependency that owns the connection owns the routing decisions
// too.
func serveXMPP(conn net.Conn) {
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	serve(socket{conn}, ip)
}

// serve is the protocol, on whichever carrier.
func serve(conn carrier, remote string) {
	s := &session{conn: conn, dec: xml.NewDecoder(conn), remote: remote}

	// Two passes over the stream header. The first offers SASL and nothing
	// else, because an unauthenticated client may not be told what an
	// authenticated one can do; the second offers bind.
	if !s.openStream(false) {
		return
	}
	if !s.authenticate() {
		return
	}
	// A new stream after SASL, which the RFC requires: authentication changes
	// what the stream is, so the client restarts it.
	s.dec = xml.NewDecoder(s.conn)
	if !s.openStream(true) {
		return
	}
	s.route()
}

// openStream answers the client's stream header and advertises what it may do
// next.
func (s *session) openStream(authed bool) bool {
	_ = s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	start, ok := s.nextStart()
	if !ok || start.Name.Local != s.openTag() {
		return false
	}

	// The one place the two carriers differ. A socket opens a stream that stays
	// open and holds every stanza as a child; a WebSocket sends a self-closing
	// <open/> and stanzas are siblings after it, because the message boundary
	// already says where each one ends. See RFC 7395 section 3.4.
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	hello := `<?xml version='1.0'?><stream:stream from='%s' id='%s' ` +
		`version='1.0' xmlns='%s' xmlns:stream='%s'>`
	if s.conn.framed() {
		hello = `<open xmlns='` + nsFraming + `' from='%s' id='%s' ` +
			`version='1.0' xmlns:stream='%s' xml:lang='en'/>`
	}
	if s.conn.framed() {
		if err := s.send(hello, Domain(), id, nsStream); err != nil {
			return false
		}
	} else if err := s.send(hello, Domain(), id, nsClient, nsStream); err != nil {
		return false
	}

	if !authed {
		// PLAIN only, and the password is an access token — the same
		// credential IMAP and submission take, so somebody who set up a mail
		// client has already set up this one. See accountForToken in
		// service/mail, which is where that rule lives.
		return s.send(`<stream:features><mechanisms xmlns='%s'>`+
			`<mechanism>PLAIN</mechanism></mechanisms></stream:features>`, nsSASL) == nil
	}
	return s.send(`<stream:features><bind xmlns='%s'/><session xmlns='%s'/>`+
		`</stream:features>`, nsBind, nsSess) == nil
}

// openTag is what this carrier's client sends to open a stream.
func (s *session) openTag() string {
	if s.conn.framed() {
		return "open"
	}
	return "stream"
}

// nextStart reads to the next start element.
func (s *session) nextStart() (xml.StartElement, bool) {
	for {
		t, err := s.dec.Token()
		if err != nil {
			return xml.StartElement{}, false
		}
		if start, ok := t.(xml.StartElement); ok {
			return start, true
		}
	}
}

// Domain is the domain this serves.
//
// The same one the mailbox is on, because that is the whole point: asim@here is
// an address, and which protocol reaches it is the caller's choice rather than
// a different account. MU_DOMAIN is the instance's own name and MAIL_DOMAIN is
// what mail was configured with first; either answers, and an instance that set
// only the second keeps working.
//
// Read rather than borrowed from service/mail, which would be a sideways import
// between two services — see TestServicesDoNotImportEachOther.
func Domain() string {
	if d := strings.TrimSpace(settings.Get("MU_DOMAIN")); d != "" {
		return d
	}
	if d := strings.TrimSpace(settings.Get("MAIL_DOMAIN")); d != "" {
		return d
	}
	return "localhost"
}

var (
	// connected is who is online, by bare JID, so a message can be delivered
	// to whatever they have open. Several resources per account: a phone and a
	// laptop are two, and both should ring.
	connected   = map[string][]*session{}
	connectedMu sync.RWMutex
)

func join(s *session) {
	connectedMu.Lock()
	connected[s.bare()] = append(connected[s.bare()], s)
	connectedMu.Unlock()
}

func leave(s *session) {
	connectedMu.Lock()
	defer connectedMu.Unlock()
	list := connected[s.bare()]
	for i, other := range list {
		if other == s {
			connected[s.bare()] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(connected[s.bare()]) == 0 {
		delete(connected, s.bare())
	}
}

// sessionsFor is everything a bare JID currently has open.
func sessionsFor(bare string) []*session {
	connectedMu.RLock()
	defer connectedMu.RUnlock()
	out := make([]*session, len(connected[bare]))
	copy(out, connected[bare])
	return out
}

// Online reports whether an account has an XMPP client connected.
//
// Presence is the thing the websocket rooms never had and never have to write:
// the protocol carries it.
func Online(accountID string) bool {
	connectedMu.RLock()
	defer connectedMu.RUnlock()
	return len(connected[strings.ToLower(accountID)+"@"+Domain()]) > 0
}

// io.EOF is the ordinary end of a session and not worth a line in the log.
func quiet(err error) bool { return err == nil || err == io.EOF }
