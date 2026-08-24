package mail

// Sending one message, wherever its recipient turns out to be.
//
// The recipient decides the route and the sender should never have had to.
// Five doors send mail from an account here — the compose form at /inbox, the
// mail_send tool, the JSON and the form handlers behind /mail, and a mail
// client over submission — and each of them had made its own arrangement with
// that fact:
//
//   - submission got it right, and its own comment names the bug it was fixing
//     as one already fixed in smtp.go "a fortnight later".
//   - the tool and both /mail handlers filed local mail and never woke
//     anything, because all three dropped the +tag on the way to DeliverHere.
//     Writing to asim+research@ from a Mu page reached the inbox and not the
//     agent.
//   - /inbox did not route at all. It called ReplyOut, which is the function
//     for mail leaving this instance, and ReplyOut refuses an address that is
//     not leaving — so writing to somebody on your own instance, or to your own
//     agent, came back "agent@micro.mu is on this instance — that is not mail
//     leaving it". Which is true and is not an answer to what was asked.
//
// So this is the one door, and the branch inside it is the only copy.

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mu/internal/auth"
)

// Outgoing is one message an account here is sending.
//
// A struct for the same reason Local is one: nine fields, all strings, and the
// shape where adding a tenth silently shifts the rest at every call site.
type Outgoing struct {
	// FromID is the account sending. Every caller has signed in, so unlike
	// Local this is never empty — the instance's own notices go through
	// DeliverHere directly.
	FromID  string
	Display string // the name the recipient sees
	// To is where it is going: an external address, a local address, a local
	// address with a +tag, or a bare username here.
	To         string
	Subject    string
	Body       string // plain text
	HTML       string // the same message as markup, where the sender wrote it
	InReplyTo  string // the Message-ID this answers
	References string // the whole chain, oldest first
	SenderIP   string
}

// One limit on an Outgoing, checked here where every door meets, so size
// is a fact about a message rather than about how it arrived. Matches the
// 10MB the SMTP paths already accept: a stored body is re-marshalled,
// re-encrypted and re-written on every later delivery to anybody, so one
// oversized message makes every account slower forever.
const maxOutgoingBytes = 10 << 20

// Deliver sends one message to wherever its recipient is: off this instance
// over SMTP, or into an inbox here.
//
// It returns the Message-ID, so a caller can record what it sent and thread the
// next one onto it. It does not file the sender's own copy — that is the
// contract ReplyOut already had, and every caller does it differently enough
// (the mail store for three of them, internal/thread for /inbox) that doing it
// here would double one of them up.
func Deliver(m Outgoing) (string, error) {
	to := strings.TrimSpace(m.To)
	if to == "" {
		return "", errors.New("no recipient")
	}
	if n := len(m.Body) + len(m.HTML); n > maxOutgoingBytes {
		return "", fmt.Errorf("message is %d bytes; the limit is %d", n, maxOutgoingBytes)
	}
	if IsExternalEmail(to) {
		return ReplyOut(m.FromID, m.Display, to, m.Subject, m.Body, m.HTML, m.InReplyTo, m.References)
	}
	return deliverHere(m, to)
}

// deliverHere files a message for somebody on this instance and wakes whatever
// was listening for it.
//
// Both halves matter equally, and filing was the easy one — which is why three
// of the four doors did that and stopped. A message in an inbox that woke
// nothing is the feature looking broken rather than half-built.
func deliverHere(m Outgoing, to string) (string, error) {
	// A local recipient may be a bare username or a full local address, with or
	// without a +tag: asim, asim@micro.mu and asim+research@micro.mu all reach
	// the same inbox, and only the last of them wakes an agent.
	local := to
	if at := strings.LastIndex(to, "@"); at > 0 {
		local = to[:at]
	}
	account, tag := SplitAlias(local)

	// Writing to the shared address files the message in your *own* inbox,
	// because agent@ resolves to whoever wrote to it. Same rule as inbound —
	// see smtp.go, where sharedAgentMail does this for mail off the network —
	// and the reason agent@ is not an account and must not be looked up as one.
	shared := strings.EqualFold(account, AgentMailbox)
	owner := m.FromID
	if !shared {
		acc, err := auth.AccountByUsername(account)
		if err != nil || acc == nil {
			return "", fmt.Errorf("no account here called %q", account)
		}
		owner = acc.ID
	}

	domain := ConfiguredDomain()
	from := EmailForUser(m.FromID, domain)
	// A full address for the record, whatever the sender typed. A bare
	// username is a convenience at the door and not something to store: it is
	// what the reply is addressed to.
	arrivedAt := to
	if !strings.Contains(arrivedAt, "@") {
		arrivedAt = EmailForUser(local, domain)
	}
	messageID := fmt.Sprintf("<%d.local@%s>", time.Now().UnixNano(), domain)

	if err := DeliverHere(Local{
		FromID: m.FromID, Display: m.Display, From: from, To: owner, Tag: tag,
		// The header goes in the field for headers and nowhere else. ReplyTo is
		// this instance's own id for the parent, which a sender does not hold
		// and must not be handed a header in place of — see parentOf.
		Subject: m.Subject, Body: m.Body, MessageID: messageID,
		InReplyTo: m.InReplyTo, References: m.References,
		SenderIP: m.SenderIP,
	}); err != nil {
		return "", err
	}

	// Authenticated without asking SPF or DKIM. Those decide whether a sender
	// off the network is who they say they are; this one signed in before
	// being allowed to say anything at all, which answers the same question
	// more strongly rather than going round it.
	//
	// Owned is a narrower claim and has to be checked rather than asserted: it
	// means the sender signed in as *the account the mail is for*, and
	// mayDispatch takes it as licence to skip asking whether that account has
	// ever heard of them. True when writing to your own agent or to agent@,
	// which resolves to you. Not true when writing to somebody else's — that
	// wakes their agent and spends their credits, so it goes through
	// senderKnownTo like mail from outside.
	//
	// It was passed as a constant, which was correct in submission where the
	// only reachable case was your own, and became a hole the moment the rule
	// was shared with the doors where it is not.
	owned := strings.EqualFold(m.FromID, owner)
	deliverInbound(InboundMail{
		Owner:      owner,
		Tag:        tag,
		Shared:     shared,
		From:       from,
		To:         arrivedAt,
		FromName:   m.Display,
		Subject:    m.Subject,
		Body:       m.Body,
		Text:       stripHTMLTags(m.Body),
		MessageID:  messageID,
		InReplyTo:  m.InReplyTo,
		References: m.References,
	}, wakeRequest{
		Owner:         owner,
		Tag:           tag,
		Shared:        shared,
		From:          from,
		To:            arrivedAt,
		Authenticated: true,
		Owned:         owned,
	})
	return messageID, nil
}
