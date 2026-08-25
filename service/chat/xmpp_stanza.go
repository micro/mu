package chat

// Authenticating a client, and routing what it sends.
//
// Split from xmpp.go because that file is the connection and this is the
// protocol on top of it.

import (
	"encoding/base64"
	"encoding/xml"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
)

// authenticate runs SASL PLAIN and leaves the session holding an account.
//
// PLAIN and nothing else, because the credential is an access token rather than
// a password: there is no shared secret to do SCRAM over, and a token presented
// over a TLS-terminating proxy is exactly what IMAP and submission already
// accept here. A client that will not do PLAIN will not do this either, which
// is the same trade those two made.
func (s *session) authenticate() bool {
	_ = s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	start, ok := s.nextStart()
	if !ok || start.Name.Local != "auth" {
		return false
	}

	var payload string
	if err := s.dec.DecodeElement(&payload, &start); err != nil {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
	if err != nil {
		s.send(`<failure xmlns='%s'><incorrect-encoding/></failure>`, nsSASL) //nolint:errcheck
		return false
	}

	// authzid\x00authcid\x00password. The first is who they claim to act as
	// and is ignored: the token says who they are, and honouring an authzid
	// would be letting the client choose.
	parts := strings.Split(string(raw), "\x00")
	if len(parts) != 3 {
		s.send(`<failure xmlns='%s'><malformed-request/></failure>`, nsSASL) //nolint:errcheck
		return false
	}
	acc, err := auth.AccountForToken(parts[1], parts[2])
	if err != nil {
		app.Log("chat", "xmpp sign-in refused for %q from %s", parts[1], s.remote)
		s.send(`<failure xmlns='%s'><not-authorized/></failure>`, nsSASL) //nolint:errcheck
		return false
	}
	s.acc = acc
	app.Log("chat", "xmpp: %s signed in from %s", acc.ID, s.remote)
	return s.send(`<success xmlns='%s'/>`, nsSASL) == nil
}

// stanza is the shape every top-level element shares.
type stanza struct {
	XMLName xml.Name
	ID      string `xml:"id,attr"`
	Type    string `xml:"type,attr"`
	To      string `xml:"to,attr"`
	From    string `xml:"from,attr"`
	Body    string `xml:"body"`
	Inner   []byte `xml:",innerxml"`
}

// route reads stanzas until the client goes away.
func (s *session) route() {
	defer leave(s)
	for {
		// Generous, because a chat connection is idle most of the time and
		// closing it is a reconnect and a re-auth. XMPP's own answer to a dead
		// peer is a whitespace ping, which a client sends.
		_ = s.conn.SetReadDeadline(time.Now().Add(10 * time.Minute))
		start, ok := s.nextStart()
		if !ok {
			return
		}
		var st stanza
		if err := s.dec.DecodeElement(&st, &start); err != nil {
			return
		}
		switch start.Name.Local {
		case "iq":
			s.iq(st)
		case "message":
			s.message(st)
		case "presence":
			s.presence(st)
		}
	}
}

// iq answers the info/query stanzas a client needs to finish connecting.
func (s *session) iq(st stanza) {
	switch {
	case strings.Contains(string(st.Inner), nsBind):
		// The resource names this connection, so a phone and a laptop are two
		// places to deliver to rather than one that wins.
		s.resource = resourceIn(string(st.Inner))
		if s.resource == "" {
			s.resource = "mu"
		}
		join(s)
		s.send(`<iq type='result' id='%s'><bind xmlns='%s'><jid>%s</jid></bind></iq>`,
			st.ID, nsBind, s.jid()) //nolint:errcheck

	case strings.Contains(string(st.Inner), nsSess):
		// Legacy and deprecated, and some clients still will not proceed
		// without it. Answering costs a line.
		s.send(`<iq type='result' id='%s'/>`, st.ID) //nolint:errcheck

	case strings.Contains(string(st.Inner), nsRoster):
		s.sendRoster(st.ID)

	default:
		// Anything else is refused rather than ignored: a client waiting on an
		// iq it never gets hangs, and "not implemented" lets it move on.
		s.send(`<iq type='error' id='%s'><error type='cancel'>`+
			`<feature-not-implemented xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/>`+
			`</error></iq>`, st.ID) //nolint:errcheck
	}
}

// sendRoster is who this account can talk to.
//
// The agent first, because it is the reason most people will connect: an
// address that is always there and answers. Contacts come from the same place
// the mail client's do — see internal/contacts — so the two clients show the
// same people.
func (s *session) sendRoster(id string) {
	var b strings.Builder
	b.WriteString(`<iq type='result' id='` + xmlAttr(id) + `'><query xmlns='` + nsRoster + `'>`)
	for _, c := range rosterFor(s.acc.ID) {
		b.WriteString(`<item jid='` + xmlAttr(c.JID) + `' name='` + xmlAttr(c.Name) +
			`' subscription='both'/>`)
	}
	b.WriteString(`</query></iq>`)
	s.send("%s", b.String()) //nolint:errcheck
}

// presence answers a presence stanza and tells the client about the agent.
//
// Minimal on purpose: the agent is always available, which is true and is the
// thing worth saying. Presence between people follows once subscriptions do.
func (s *session) presence(st stanza) {
	if st.Type == "unavailable" {
		return
	}
	// The agent is here. Without this a client shows an empty roster of
	// offline contacts and no reason to type anything.
	s.send(`<presence from='%s' to='%s'/>`, agentJID(), xmlAttr(s.jid())) //nolint:errcheck
}

// message delivers what somebody said.
//
// Three destinations and the same rule as mail decides between them: an agent
// address wakes an agent, an account here is delivered to whatever it has
// connected, and anything else is refused rather than silently dropped.
func (s *session) message(st stanza) {
	text := strings.TrimSpace(st.Body)
	if text == "" || st.To == "" {
		return
	}
	to := strings.ToLower(bareOf(st.To))
	local, _ := splitJID(to)

	// An agent. Announced rather than answered here, because a service does
	// not run an agent — see event.ChatForAgent and agent/chat, which is the
	// same seam the websocket rooms use.
	if agentAddressed(local) {
		event.RequestChatReply(xmppRoom(s.bare(), to), "", "", "", s.acc.ID, text)
		return
	}

	// Somebody here.
	if deliverXMPP(s.bare(), to, text) {
		return
	}

	// Federation is not built, so say so rather than accepting a message that
	// goes nowhere. A stanza error is what a client renders as "not
	// delivered"; silence is what it renders as delivered.
	s.send(`<message type='error' to='%s' from='%s'><error type='cancel'>`+
		`<remote-server-not-found xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/>`+
		`</error></message>`, xmlAttr(s.jid()), xmlAttr(st.To)) //nolint:errcheck
}

// deliverXMPP hands a message to every resource a local account has open, and
// reports whether anybody was there.
func deliverXMPP(from, to, text string) bool {
	sessions := sessionsFor(to)
	if len(sessions) == 0 {
		return false
	}
	for _, other := range sessions {
		other.send(`<message type='chat' from='%s' to='%s'><body>%s</body></message>`,
			xmlAttr(from), xmlAttr(other.jid()), xmlText(text)) //nolint:errcheck
	}
	return true
}

// SayTo delivers a message to an account's connected clients, from an address.
//
// The door agent/chat answers through, the same shape Say is for the websocket
// rooms. Reports whether anybody was connected, so a caller can fall back to
// leaving it somewhere rather than assume it landed.
func SayTo(accountID, from, text string) bool {
	return deliverXMPP(from, strings.ToLower(accountID)+"@"+Domain(), text)
}
