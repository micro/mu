package chat

// Addresses, and the small amount of escaping a hand-written XML server owes.

import (
	"encoding/xml"
	"regexp"
	"strings"

	"mu/internal/contacts"
)

// splitJID takes a bare JID apart: the local part and the domain.
func splitJID(jid string) (local, domain string) {
	jid = bareOf(jid)
	if i := strings.LastIndex(jid, "@"); i > 0 {
		return jid[:i], jid[i+1:]
	}
	return jid, ""
}

// bareOf drops the resource, which names a connection rather than a person.
func bareOf(jid string) string {
	jid = strings.TrimSpace(jid)
	if i := strings.Index(jid, "/"); i >= 0 {
		return jid[:i]
	}
	return jid
}

// agentAddressed reports whether a local part names an agent rather than a
// person.
//
// The same two shapes mail uses, deliberately: agent@ is the shared address and
// you+tag@ is one of your own. A JID localpart may contain "+" — RFC 7622
// forbids "&'/:<>@ and not that — so the address somebody learned for mail is
// the address that works here, which is the entire argument for having one.
func agentAddressed(local string) bool {
	local = strings.ToLower(strings.TrimSpace(local))
	if local == AgentMailbox {
		return true
	}
	return strings.Contains(local, "+")
}

// AgentMailbox is the shared agent address's local part.
//
// Declared here rather than imported: service/mail owns the mail half and
// services never import each other, so the two hold the same constant and a
// test asserts they agree — see TestTheAgentAddressIsTheSameOnBothProtocols.
const AgentMailbox = "agent"

// agentJID is the shared agent address as a JID.
func agentJID() string { return AgentMailbox + "@" + Domain() }

// AgentAddress is who the agent is on this instance, as an address.
//
// The same string mail hands out, because it is the same agent — see the
// package comment on xmpp.go for why one address serving two protocols is the
// point rather than a coincidence.
func AgentAddress() string { return agentJID() }

// xmppRoom is the conversation key for a one-to-one XMPP exchange.
//
// The pair, lowest first, so a message in either direction lands on the same
// conversation in the record. Prefixed, because this key shares a namespace
// with the websocket rooms' ids and "asim@micro.mu" is not a room id.
func xmppRoom(a, b string) string {
	a, b = strings.ToLower(bareOf(a)), strings.ToLower(bareOf(b))
	if b < a {
		a, b = b, a
	}
	return "xmpp_" + a + "_" + b
}

// resourceIn pulls the resource a client asked to bind, if it named one.
var resourceRe = regexp.MustCompile(`(?s)<resource[^>]*>(.*?)</resource>`)

func resourceIn(inner string) string {
	m := resourceRe.FindStringSubmatch(inner)
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// contact is one entry in a roster.
type contact struct {
	JID  string
	Name string
}

// rosterFor is who an account can talk to: its agent first, then its address
// book.
//
// The agent leads because it is the reason most people connect — an address
// that is always there and answers. The rest come from internal/contacts, which
// is where the mail client's people come from too, so the two clients show the
// same list rather than each keeping their own.
func rosterFor(accountID string) []contact {
	out := []contact{{JID: agentJID(), Name: "Micro"}}
	for _, c := range contacts.List(accountID) {
		addr := strings.TrimSpace(c.Email)
		if addr == "" || !strings.Contains(addr, "@") {
			continue
		}
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = addr
		}
		out = append(out, contact{JID: addr, Name: name})
	}
	return out
}

// xmlAttr escapes a value going into an attribute.
//
// Hand-written, because this server writes stanzas as strings rather than
// marshalling them, and a display name with an apostrophe in it would otherwise
// end the attribute — the same class of bug as the mail header injection this
// repository fixed, in a different syntax. See service/mail/header.go.
func xmlAttr(v string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(v))
	return b.String()
}

// xmlText escapes a value going into an element's body.
func xmlText(v string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(v))
	return b.String()
}
