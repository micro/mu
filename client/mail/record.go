package mail

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
// Through the agent's own door rather than around it — agent.Said is what every
// other client uses to write down what a person wrote, and there is no version
// of this that is special enough to reach past it.

import (
	"strings"

	"mu/agent"
	"mu/internal/thread"
	"mu/service/mail"
)

// recordDelivery writes an arriving message into the system of record.
//
// Keyed the same way answerMail keys it, so a chain that gets answered and a
// chain that does not are the same conversation rather than two — see chainKey.
func recordDelivery(m mail.InboundMail) {
	text := asked(m)
	if m.Owner == "" || text == "" {
		return
	}
	th := thread.Open(m.Owner, Client, chainKey(m))
	if th == nil {
		return
	}
	agent.SaidTo(m.Owner, th.ID, text, m.MessageID, m.From, m.To)
	// The name behind the address, which a message cannot carry — it belongs to
	// whoever wrote in, not to each line they wrote.
	if name := strings.TrimSpace(m.FromName); name != "" && m.From != "" {
		thread.Join(m.Owner, th.ID, thread.Party{
			Kind: thread.RolePerson, Key: m.From, Name: name})
	}
}
