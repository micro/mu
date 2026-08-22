package mail

// One way for mail to leave this instance.
//
// It is one function on purpose, and for the reason service/sms/send.go gives
// about texts: every rule this service has about mail going out is a rule you
// can only enforce where the sending happens, and a second path that skipped
// one would not look like a bug until the damage was done. There were three —
// the tool, the JSON API and the compose form — each with its own copy of the
// quota check, each able to send without the others' rules.
//
// ── The gate ────────────────────────────────────────────────────
//
// Mail leaving here goes out as you@<mail domain>, which on a public instance
// is the domain that also carries password resets, receipts and sign-in links.
// Anyone can sign up. So what an unaccountable account sends is charged to the
// deliverability of the mail that has to arrive, and no balance makes that
// whole — it is the same argument that gave service/email a domain of its own,
// aimed at the half that was already happening here.
//
// Two ways through, and each is the answer to a different question.
//
// **Trusted** is auth's word, not a new one: an admin, an approved account, a
// verified email address, or money on the balance. It is what already decides
// who may post publicly, and posting and sending both spend something shared,
// so a second notion of accountability would only be this one with different
// bugs. Note what it is not — it is not a plan. Somebody who has topped up a
// pound has a card behind them, which is the whole point.
//
// **A reply** is never gated, and that is not a concession. An agent with an
// address that cannot answer the mail it receives is the feature not working;
// answering a stranger who wrote to you is the most ordinary thing that address
// does. It is also the right line on risk rather than a hole in it — complaints
// come from mail nobody asked for, and a reply to somebody who wrote first is
// the best reputation signal a domain has. An instance whose outbound is
// transactional mail plus answers to people who made contact is not merely
// protected, it is well behaved.
//
// A self-hosted instance is not caught by this. The first account is the admin,
// so it is trusted from the moment it exists, and an operator can approve
// anyone else. The gate closes on the case it was written for: a free signup on
// a public instance, cold-mailing strangers under everybody's domain.

import (
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
)

// MaySendOut reports whether this account may send mail out of this instance to
// this address, and says why not when it may not.
//
// Separated from the send so a page can ask before drawing a form, and so the
// reason is written once. The reason names both ways out, because a refusal
// that does not is indistinguishable from the feature being broken.
func MaySendOut(owner, to string) (bool, string) {
	if owner == "" {
		return false, "sign in to send mail"
	}
	if auth.Trusted(owner) {
		return true, ""
	}
	if WroteToUs(owner, to) {
		return true, ""
	}
	// Places, not paths. A route written into a sentence reads as a link and is
	// not one — see internal/app, which linkifies these words for a page and
	// leaves the sentence readable everywhere else.
	return false, fmt.Sprintf("this account cannot send mail out as itself yet — %s has not "+
		"written to you, and mail leaving here goes out under this instance's own domain. "+
		"Verify your email address in your Account, or add credit to your Balance, and it will", to)
}

// WroteToUs reports whether this address has written to this account before.
//
// The test for a reply, and deliberately looser than a threading check. An agent
// answering a message from last week, or writing back about something raised in
// a different thread, is still answering somebody who made contact — and a rule
// that only recognised a strict In-Reply-To chain would refuse exactly the mail
// a person would call a reply.
//
// Spam does not count. A message filed as spam is not a relationship, and
// treating it as one would let anybody open the gate by sending one message
// nobody wanted.
func WroteToUs(owner, addr string) bool {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if owner == "" || addr == "" {
		return false
	}
	mutex.RLock()
	defer mutex.RUnlock()
	for _, msg := range messages {
		if msg == nil || msg.Spam || msg.ToID != owner {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(msg.FromID), addr) {
			return true
		}
	}
	return false
}

// SendOut delivers one message off this instance and files a copy.
//
// The order is the order everything else here charges in: everything that can
// refuse does so before the provider is called, because after that the mail has
// gone whatever we decide, and the charge lands before rather than after —
// mail that has left cannot be recalled when a charge fails.
func SendOut(owner, displayName, to, subject, bodyPlain, bodyHTML, replyTo string) (string, error) {
	return ReplyOut(owner, displayName, to, subject, bodyPlain, bodyHTML, replyTo, "")
}

// ReplyOut is SendOut for a message that continues a thread.
//
// Same gate, same charge, same provider — the difference is two headers.
// In-Reply-To names the message being answered and References carries the whole
// chain, and a receiving client needs both: Gmail threads on References, and one
// that sees only In-Reply-To files a long conversation as a run of unrelated
// messages.
//
// It exists because /inbox's reply had nowhere to put them. It called SendOut
// with an empty replyTo — SendOut had no references parameter at all — so every
// answer sent from the inbox arrived at the other end as a brand new
// conversation, next to the thread it was answering. The reply was correct in
// this instance's own record and wrong in everybody else's.
func ReplyOut(owner, displayName, to, subject, bodyPlain, bodyHTML, inReplyTo, references string) (string, error) {
	to = strings.TrimSpace(to)
	if !IsExternalEmail(to) {
		return "", fmt.Errorf("%s is on this instance — that is not mail leaving it", to)
	}
	if !Reachable() {
		return "", fmt.Errorf("%s is outside this instance, and there is no mail domain here to "+
			"send it from — set MAIL_DOMAIN and this instance can send as itself", to)
	}
	if ok, why := MaySendOut(owner, to); !ok {
		return "", fmt.Errorf("%s", why)
	}
	if err := charge(owner, quota.OpMailSend); err != nil {
		return "", err
	}

	if bodyHTML == "" {
		bodyHTML = convertPlainTextToHTML(bodyPlain)
	}
	from := EmailForUser(owner, ConfiguredDomain())
	messageID, err := SendExternalReply(displayName, from, to, subject, bodyPlain, bodyHTML,
		inReplyTo, references)
	if err != nil {
		return "", err
	}
	return messageID, nil
}

// DeliverHere files a message for somebody else on this instance, charged and
// capped the same as one leaving it.
//
// # Why this exists
//
// Local mail was free on every door and gated on none. mail_send cost 0 and
// carried no daily limit, so charge() returned early twice over and the calls
// that did make it read like a gate while doing nothing. The paths that did not
// call it at all were worse: submission's deliverLocally said so in as many
// words — "nothing to charge: this is the same act as sending a message from
// one page of this instance to another" — which meant a signed-in account with
// a mail client could write to every username here, at any rate, for nothing.
// The agent's own reply path had no gate either.
//
// Spam is not a fact about which network the recipient is on. An inbox full of
// junk from the account next door is the same failure as one full of junk from
// outside, and the instance not having paid to carry it is beside the point.
//
// # What is not a send
//
// Writing to yourself, or to your own agent, is not reaching anybody — it is
// the product's main loop, and charging it would put a price on the thing
// people came for. Same for the agent answering you. Those are free and
// uncapped, and the test for it is that the message stays inside one account.
//
// Everything else — you to somebody else here, an agent to a third party on a
// copied-in thread — is a send.
// Local is a message for somebody on this instance.
//
// A struct because there are ten fields and they are all strings: the shape
// where adding one silently shifts the rest, which internal/app's renderShell
// has its own comment about. Everything that files a local message fills this
// in, so a new field is a compile error at every call site rather than a
// misaligned argument at one.
type Local struct {
	// FromID is the account sending. Empty only for the instance's own
	// notices, which have nobody to charge.
	FromID    string
	Display   string // the name the recipient sees
	From      string // the address it comes from
	To        string // an account id, a username, or a local address
	Tag       string // the part after the plus, where there was one
	Subject   string
	Body      string
	ReplyTo   string
	MessageID string
	// References is the chain this message continues, as the mail header
	// spells it. Carried so the stored message threads the same way an
	// inbound one does — a reply delivered locally used to arrive with
	// nothing to key on but its own id, so it started a conversation
	// instead of joining one.
	References string
	SenderIP   string
}

func DeliverHere(m Local) error {
	acc, err := auth.GetAccount(LocalRecipient(m.To))
	if err != nil || acc == nil {
		return fmt.Errorf("no account here called %q", m.To)
	}

	// Your own mailbox, your own agent, or your agent answering you. Not a
	// send: it is the product's main loop, and putting a price on it would
	// charge people for the thing they came for.
	own := m.FromID != "" && strings.EqualFold(m.FromID, acc.ID)

	switch {
	case own:
	case m.FromID == "":
		// Nobody to charge and nobody to hold responsible. The instance's own
		// notices come through here and are the only legitimate case, so
		// anything else arriving without a sender is worth seeing in the log.
		app.Log("mail", "delivering to %s with no sender to charge", acc.ID)
	default:
		if err := charge(m.FromID, quota.OpMailSend); err != nil {
			return err
		}
	}

	return SendMessageTo(Delivery{
		From: m.Display, FromID: m.From,
		To: acc.Name, ToID: acc.ID, Tag: m.Tag,
		Subject: m.Subject, Body: m.Body,
		ReplyTo: m.ReplyTo, MessageID: m.MessageID, References: m.References,
		SenderIP: m.SenderIP,
	})
}
