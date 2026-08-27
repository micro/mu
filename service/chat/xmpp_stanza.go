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

	// What this server can do. Asked once on connecting, and everything
	// optional is unreachable until it is answered — see xmpp_disco.go.
	case strings.Contains(string(st.Inner), nsDisco):
		s.disco(st)

	case strings.Contains(string(st.Inner), nsDiscoItem):
		s.discoItems(st)

	// The archive. This is what puts yesterday's conversation on the screen
	// when a client opens it — see xmpp_mam.go.
	case strings.Contains(string(st.Inner), nsMAM):
		s.archive(st)

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
// Four destinations and the same rule as mail decides between them: an agent
// address wakes an agent, an account here is written down and handed to whatever
// it has connected, an address here that is nobody is refused, and another
// domain is refused as unreachable.
//
// The last two used to be one answer, and it was the wrong one. Whether the
// recipient happened to have a client running decided the error, so writing to
// a colleague who was asleep came back as "that server does not exist" — a
// permanent failure a client will not retry, for a temporary condition that is
// not a failure at all. Being offline is the ordinary case for chat, which is
// exactly why the record has to be underneath it.
func (s *session) message(st stanza) {
	text := strings.TrimSpace(st.Body)
	if text == "" || st.To == "" {
		return
	}
	to := strings.ToLower(bareOf(st.To))
	local, domain := splitJID(to)

	// An agent. Announced rather than answered here, because a service does
	// not run an agent — see event.ChatForAgent and agent/chat, which is the
	// same seam the websocket rooms use.
	if agentAddressed(local) {
		event.RequestChatReply(xmppRoom(s.bare(), to), "", "", "", s.acc.ID, text)
		return
	}

	// Somebody on another server. Sent over a federated link, and the record is
	// kept here either way — a conversation you had is yours whichever server
	// the other half was on.
	//
	// The error, when it fails, is remote-server-not-found rather than silence,
	// for the same reason it was that before federation existed: a stanza error
	// is what a client renders as "not delivered", and silence is what it
	// renders as delivered.
	if !strings.EqualFold(domain, Domain()) {
		if err := SendRemote(s.bare(), to, text); err != nil {
			app.Log("chat", "s2s: %v", err)
			s.stanzaError(st.To, "cancel", "remote-server-not-found")
			return
		}
		record(s.bare(), to, text)
		return
	}

	// Somebody here — or nobody. An address on this domain that names no
	// account is a typo, and telling the sender that is the whole reason to
	// separate this case from the one above.
	if accountFor(to) == nil {
		s.stanzaError(st.To, "cancel", "item-not-found")
		return
	}

	// Written down first, so it survives whether or not anybody is connected —
	// see xmpp_record.go for why there is no chat store of its own.
	record(s.bare(), to, text)
	deliverXMPP(s.bare(), to, text)
}

// accountFor is whose address this is, or nil for nobody here.
//
// Two lookups in the order service/mail uses them, because it is the same
// question about the same addresses: a custom address somebody has added to
// their account is theirs, and otherwise the local part is the account id. An
// instance where chat resolved only one of those would have addresses that take
// mail and refuse chat, which is the opposite of the point.
func accountFor(jid string) *auth.Account {
	jid = strings.ToLower(bareOf(jid))
	if acc := auth.AccountForAddress(jid); acc != nil {
		return acc
	}
	local, _ := splitJID(jid)
	if local == "" {
		return nil
	}
	acc, err := auth.GetAccount(local)
	if err != nil {
		return nil
	}
	return acc
}

// stanzaError tells the sender their message did not arrive.
func (s *session) stanzaError(to, kind, condition string) {
	s.send(`<message type='error' to='%s' from='%s'><error type='%s'>`+
		`<%s xmlns='urn:ietf:params:xml:ns:xmpp-stanzas'/>`+
		`</error></message>`, xmlAttr(s.jid()), xmlAttr(to), kind, condition) //nolint:errcheck
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
