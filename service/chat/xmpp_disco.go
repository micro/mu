package chat

// What this server can do, when a client asks. XEP-0030.
//
// Every optional part of XMPP is discovered rather than assumed: a client sends
// one disco#info query on connecting and decides from the answer whether to ask
// for an archive, whether to expect carbons, whether group chat exists. Without
// this, everything below it is unreachable no matter how well it works —
// Conversations will not request an archive from a server that has not said it
// has one, so an archive with no disco entry is an archive nobody reads.
//
// It is answered for whatever the client addressed. A server's own features and
// an account's own features are different lists in the spec, and clients ask for
// both; the honest simplification while the two lists are identical is to
// answer the same thing to either, and to split them the day they differ.

import "strings"

const (
	nsDisco     = "http://jabber.org/protocol/disco#info"
	nsDiscoItem = "http://jabber.org/protocol/disco#items"
)

// features is everything this server implements that a client has to ask about.
//
// Add a line here when a XEP lands, not before: a feature advertised and not
// implemented is worse than one that is missing, because the client stops
// falling back and starts waiting.
func features() []string {
	return []string{
		nsDisco,
		nsDiscoItem,
		nsMAM,
	}
}

// disco answers "what can you do".
func (s *session) disco(st stanza) {
	var b strings.Builder
	b.WriteString(`<iq type='result' id='` + xmlAttr(st.ID) + `' from='` +
		xmlAttr(Domain()) + `' to='` + xmlAttr(s.jid()) + `'>`)
	b.WriteString(`<query xmlns='` + nsDisco + `'>`)
	// "im" rather than anything cleverer: this is an instant messaging server,
	// and a client uses the identity to decide what icon to draw.
	b.WriteString(`<identity category='server' type='im' name='Mu'/>`)
	for _, f := range features() {
		b.WriteString(`<feature var='` + f + `'/>`)
	}
	b.WriteString(`</query></iq>`)
	s.send("%s", b.String()) //nolint:errcheck
}

// discoItems answers "what else is there", which is nothing yet.
//
// An empty result rather than an error: a client asking this is looking for
// services hanging off the domain — a group chat component, a file store — and
// "none" is a true answer it can act on, where feature-not-implemented reads as
// a broken server.
func (s *session) discoItems(st stanza) {
	s.send(`<iq type='result' id='%s' from='%s' to='%s'><query xmlns='%s'/></iq>`,
		xmlAttr(st.ID), xmlAttr(Domain()), xmlAttr(s.jid()), nsDiscoItem) //nolint:errcheck
}
