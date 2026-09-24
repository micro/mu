// Inbox owns the projection of incoming correspondence. Services keep the source records.
package inbox

// Every delivery goes in the record.
//
// The inbox reads internal/thread, and until now the only mail that ever
// reached it was mail addressed to an agent — because the only handler mail had
// was the one that wakes one. So an account with a full mailbox saw an empty
// /inbox, and the page whose entire claim is that things turn up in it showed
// nothing turning up. What was missing was not a page: it was the fact.
//
// This is deliberately not the agent's business. answerMail decides whether
// something should be *answered*, which is a question about trust and about
// cost — a stranger must not be able to drive your agent, and a run costs
// credits. Recording is a question about fact: it arrived, it is yours, it is in
// your inbox on /mail already, and leaving it out of the record only means the
// agent cannot see the mailbox it is supposed to be working on.
//
// The two run independently and either may go first, which is why they cannot
// be one function and must not double-record. thread.Add settles that on the
// Message-ID: the same arrival described twice is one message.
//
import (
	"net/url"
	"strings"

	"mu/internal/thread"
	"mu/service/mail"
)

// recordMail writes an arriving message into the system of record.
//
// Keyed the same way answerMail keys it, so a chain that gets answered and a
// chain that does not are the same conversation rather than two — see chainKey.
func recordMail(m mail.InboundMail) {
	// The body, not the subject and the body.
	//
	// asked() joins them because that is what the *agent* is handed: to a model
	// the subject is part of the question. The record is a different job — the
	// subject belongs to the conversation, and prepending it to every message
	// meant a reader saw it as the heading and again at the top of every message
	// in the thread. thread.Name is how a client says what a conversation is
	// about without writing it into what somebody said.
	text := incomingMailBody(m)
	if m.Owner == "" {
		return
	}
	// A message with no text is still a message that arrived.
	//
	// This returned on an empty body, so mail carrying only an attachment was
	// delivered, stored, and never written to the record — which is what
	// /inbox reads. DMARC reports are the ones you notice, because they arrive
	// daily and several senders ship the report with no covering text at all,
	// but calendar invites and scanned documents land the same way. The mail
	// was in the mailbox and the page whose whole claim is that things turn up
	// in it did not show them.
	//
	// What is recorded says what arrived rather than pretending to be prose.
	// Not the subject: thread.Name already carries that, and putting it in the
	// message body too is the duplication removed above.
	if text == "" {
		if m.Attachment == "" {
			return // genuinely nothing: no words, nothing attached
		}
		text = "(no message — attached: " + m.Attachment + ")"
	}
	th := thread.ByRef(m.Owner, append(thread.Refs(m.InReplyTo), thread.Refs(m.References)...)...)
	if th == nil {
		th = thread.Open(m.Owner, "mail", mailKey(m))
	}
	if th == nil {
		return
	}
	thread.Name(m.Owner, th.ID, mailSubject(m.Subject))
	thread.Add(thread.Message{Account: m.Owner, Thread: th.ID, Role: thread.RolePerson, Text: text, Ref: m.MessageID, From: m.From, To: m.To})
	// The name behind the address, which a message cannot carry — it belongs to
	// whoever wrote in, not to each line they wrote.
	if name := strings.TrimSpace(m.FromName); name != "" && m.From != "" {
		thread.Join(m.Owner, th.ID, thread.Party{
			Kind: thread.RolePerson, Key: m.From, Name: name})
	}
}

// MailURL records a delivery before returning its owner-scoped conversation link.
// Recording is idempotent by Message-ID, including when the subscriber runs first.
func MailURL(m mail.InboundMail) string {
	recordMail(m)
	if th := thread.ByRef(m.Owner, m.MessageID); th != nil {
		return "/inbox?id=" + url.QueryEscape(th.ID)
	}
	return "/inbox"
}

func mailKey(m mail.InboundMail) string {
	if refs := thread.Refs(m.References); len(refs) > 0 {
		return refs[0]
	}
	if refs := thread.Refs(m.InReplyTo); len(refs) > 0 {
		return refs[0]
	}
	if m.MessageID != "" {
		return m.MessageID
	}
	// Nothing to key on at all. The address is a poor thread — everyone who
	// writes to it lands in one — so it is scoped by sender, which at least
	// keeps two strangers apart.
	return m.To + " " + m.From
}

func mailSubject(s string) string {
	s = strings.TrimSpace(s)
	for {
		l := strings.ToLower(s)
		switch {
		case strings.HasPrefix(l, "re:"):
			s = strings.TrimSpace(s[3:])
		case strings.HasPrefix(l, "fwd:"):
			s = strings.TrimSpace(s[4:])
		case strings.HasPrefix(l, "fw:"):
			s = strings.TrimSpace(s[3:])
		default:
			return s
		}
	}
}

func incomingMailBody(m mail.InboundMail) string {
	if b := strings.TrimSpace(m.Text); b != "" {
		return b
	}
	return strings.TrimSpace(m.Body)
}
