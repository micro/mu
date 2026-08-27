package inbox

// The record, as a mail client sees it.
//
// IMAP is not mail's protocol the way SMTP is. SMTP delivers; IMAP delivers
// nothing at all — it is a reader over a message store, and it has no opinion
// about how anything got there. So the question "should IMAP serve texts" is
// not the same question as "should a chat client serve email", which is the
// objection it looks like: XMPP is a transport and IMAP is a reader.
//
// The store worth reading is the record every channel writes to. A mail client
// is the one client almost everybody already has on every device they own, and
// pointing it at the mail alone is what made a text invisible in it.
//
// # Where this lives, and why
//
// service/mail may not read internal/thread: a delivery mechanism keeps its own
// record and knows nothing about anybody else's. So it exposes mail.Bridged and
// this fills it in. Nothing in the mail service knows what a text is; it is
// handed messages and serves them.
//
// This package is where that knowledge already lives. It reads the record, it
// reads the mail service, and it is the unified inbox — which is the thing IMAP
// is now a client of.
//
// # Mail is not bridged
//
// The record holds mail too, written by agent/mail above the delivery. Handing
// it back here would put every message in twice, once as an envelope out of
// mail's own store and once as prose out of the record. Mail comes from mail.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"mu/internal/thread"
	"mu/service/mail"
	"mu/service/sms"
)

// bridgeDepth is how far back the bridge reaches, and how many messages of each
// conversation it renders.
//
// A mailbox is not an archive. A client that syncs everything on first connect
// would pull the whole record over a phone connection, and what somebody wants
// in a mail client is what has been happening — /recall and the inbox search are
// for going further back on purpose.
const (
	bridgeThreads  = 200
	bridgeMessages = 50
)

// Bridge is every conversation this account has had somewhere other than mail,
// rendered as messages a mail client can read.
//
// Filled into mail.Bridged at boot. See internal/server/hooks.go.
func Bridge(accountID string) []*mail.Message {
	if accountID == "" {
		return nil
	}
	domain := mail.ConfiguredDomain()

	var out []*mail.Message
	for _, t := range thread.List(accountID, bridgeThreads) {
		// Mail comes from mail, or every message arrives twice.
		if t.Client == mailClient {
			continue
		}
		// What arrived, not what you started here — the same rule the inbox
		// list follows, so the two agree about what an inbox is.
		if !thread.Arrived(t) {
			continue
		}
		// And nothing held. Held means somebody nobody here has heard of has
		// not been let in; pushing it to a phone through Gmail is precisely
		// what being held is meant to prevent.
		if thread.IsHeld(t) {
			continue
		}
		out = append(out, asMessages(accountID, t, domain)...)
	}
	return out
}

// asMessages renders one thread as a run of messages.
//
// One message per message rather than one per conversation. A mail client
// threads on In-Reply-To and shows the exchange; collapsing it into a single
// message would hand somebody a transcript instead of a conversation, and there
// would be nothing to reply to.
func asMessages(accountID string, t thread.Thread, domain string) []*mail.Message {
	msgs := thread.Messages(accountID, t.ID, bridgeMessages)
	if len(msgs) == 0 {
		return nil
	}

	subject := strings.TrimSpace(t.Subject)
	if subject == "" {
		subject = strings.TrimSpace(t.Key)
	}
	if subject == "" {
		subject = "Conversation"
	}

	out := make([]*mail.Message, 0, len(msgs))
	prev := ""
	for _, m := range msgs {
		id := bridgeID(m.ID)
		messageID := "<" + id + "@" + domain + ">"

		one := &mail.Message{
			ID:        id,
			Subject:   subject,
			Body:      m.Text,
			CreatedAt: m.At,
			MessageID: messageID,
			ReplyTo:   prev,
			ThreadID:  bridgeID(t.ID),
			// Read is the thread's own answer. The record marks a conversation
			// seen rather than each message, so anything said before you last
			// looked has been seen and anything after it has not — which is
			// exactly what the flag means to a client.
			Read: !m.At.After(t.Seen),
		}

		if said := strings.TrimSpace(m.From); said != "" && m.Role != thread.RoleAgent &&
			!strings.EqualFold(said, accountID) {
			// Somebody else wrote it, so it is a message that arrived.
			one.ToID = accountID
			one.From = said
			one.FromID = bridgeAddress(said, t.Client, domain)
		} else {
			// You or your agent wrote it. FromID is the account, which is what
			// files it under Sent rather than in the inbox — see sentBy. A
			// client that showed your own replies as new mail would ring for
			// every answer the agent gave.
			one.FromID = accountID
			one.From = accountID
			one.ToID = accountID
			one.To = strings.TrimSpace(t.Key)
		}

		out = append(out, one)
		prev = messageID
	}
	return out
}

// bridgeAddress is an address for somebody who does not have one.
//
// A phone number is not an address and a mail client needs one to show a
// sender at all. The number becomes the local part with the channel after it —
// 447700900123.sms@example.com — which is unambiguous, routable back to one
// conversation, and reads as what it is. A chat address is already an address
// and is left alone.
func bridgeAddress(who, client, domain string) string {
	who = strings.TrimSpace(who)
	if strings.Contains(who, "@") {
		return who
	}
	// Digits only. A local part may not begin with a plus — that is the tag
	// separator this instance uses for agent boxes — and punctuation somebody
	// wrote a number with is not part of the number.
	var local strings.Builder
	for _, r := range who {
		if r >= '0' && r <= '9' {
			local.WriteRune(r)
		}
	}
	if local.Len() == 0 || domain == "" {
		return ""
	}
	kind := strings.ToLower(strings.TrimSpace(client))
	if kind == "" {
		kind = "msg"
	}
	return local.String() + "." + kind + "@" + domain
}

// bridgeID is a stable id for a bridged message.
//
// Stable because IMAP's contract is that a UID, once given, means that message
// in that folder forever — see service/mail/imapbox.go. The record's own ids
// are stable, and this only marks them as coming from the record so they cannot
// collide with a mail id.
func bridgeID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "r" + hex.EncodeToString(sum[:8])
}

// Reply answers a conversation that did not arrive by mail, addressed the way
// the bridge names it, and reports whether the address was one of those at all.
//
// # It is a reply, and only ever a reply
//
// The address is composed — 447700900123.sms@example.com — so anybody who has
// seen one can write another. If the address alone were enough to send, knowing
// the pattern would be permission to text any number in the world from this
// instance's number, which is a shared number whose reputation belongs to
// everybody on the instance.
//
// So the address does not grant anything. It names a conversation, and a
// conversation either exists for this account or it does not: thread.Find
// answers that, scoped to the account the submission session authenticated as.
// An address naming no conversation sends nothing and says so.
//
// That is the same rule the inbound side already keeps — service/sms wakes an
// agent only for a sender the account knows — arrived at from the other
// direction, which is the sign it is the right rule rather than a convenience.
func Reply(accountID, to, text string) (bool, error) {
	client, key, ok := bridgeParse(to, mail.ConfiguredDomain())
	if !ok {
		return false, nil
	}
	text = strings.TrimSpace(quotedOff(text))
	if text == "" {
		return true, fmt.Errorf("there is nothing to send")
	}

	// The conversation, or nothing. Scoped to the account that authenticated.
	t := thread.Find(accountID, client, key)
	if t == nil {
		return true, fmt.Errorf("no conversation here with %s — this address answers "+
			"one that already exists and cannot start one", key)
	}
	if thread.IsHeld(*t) {
		return true, fmt.Errorf("that conversation is waiting to be let in — let it " +
			"in from the inbox first")
	}

	channel := sms.ChannelSMS
	switch client {
	case thread.SMSClient:
	case thread.WhatsAppClient:
		channel = sms.ChannelWhatsApp
	default:
		// A conversation this cannot answer. Chat goes out over XMPP and there
		// is no path from here to it yet; saying so is better than a message
		// that looks sent and never arrives.
		return true, fmt.Errorf("replying to a %s conversation from a mail client is not wired yet",
			client)
	}

	if _, err := sms.SendOn(channel, accountID, key, text); err != nil {
		return true, err
	}
	// On the conversation, so the thread reads as one exchange wherever it is
	// read — the same record the inbox writes to when you reply there.
	thread.Add(thread.Message{Thread: t.ID, Account: accountID, Text: text})
	return true, nil
}

// bridgeParse reads a bridge address back into the conversation it names.
//
// The inverse of bridgeAddress. Only this instance's own domain, because an
// address at somebody else's domain is somebody else's mail however it is
// spelled.
func bridgeParse(addr, domain string) (client, key string, ok bool) {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		addr = strings.TrimSuffix(addr[i+1:], ">")
	}
	at := strings.LastIndex(addr, "@")
	if at <= 0 || domain == "" || !strings.EqualFold(addr[at+1:], domain) {
		return "", "", false
	}
	local := addr[:at]

	dot := strings.LastIndex(local, ".")
	if dot <= 0 {
		return "", "", false
	}
	client = strings.ToLower(local[dot+1:])
	digits := local[:dot]
	for _, r := range digits {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	switch client {
	case thread.SMSClient, thread.WhatsAppClient:
	default:
		return "", "", false
	}
	// The plus goes back on: the record keys a phone conversation by the number
	// in international form, and the local part could not carry a leading plus
	// because that is this instance's own tag separator.
	return client, "+" + digits, true
}

// quotedOff takes the quoted history off a reply.
//
// A mail client quotes what it is answering, and a text is not a mail: sending
// the quote back would charge for it and put the whole exchange on somebody's
// lock screen. The inbox already knows how to find the top of a reply.
func quotedOff(text string) string {
	top, _ := unquoted(text)
	return top
}
