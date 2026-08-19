package mail

// Joining a conversation rather than answering a message.
//
// Somebody is writing to their brother about when the train leaves, and puts the
// agent in Cc. Nobody migrated, nobody installed anything, and the agent is now
// in a thread that already had people in it. That is a different situation from
// somebody writing to their agent, and it needs three things the one-to-one case
// does not:
//
//   - The reply goes to everybody, not just the sender. An agent that answers
//     one person in a room of three is talking to itself in public.
//   - It says what it is, once. The brother did not sign up for anything and an
//     unexplained third party answering his mail is alarming rather than useful.
//   - It shuts up. This is the one that decides whether the feature is usable:
//     an agent that replies to every message of a conversation between two other
//     people is noise, costs a model call a turn, and gets muted. It speaks when
//     it is spoken to, and when it has just arrived.
//
// The rules live in service/mail — Others, Addressed, Named — because they are
// facts about a message. What is here is what to do about them, which is this
// client's job: translate what arrived, and render what goes back.

import (
	"strings"

	"mu/internal/thread"
	"mu/service/mail"
)

// spoken reports whether the agent has already answered on this conversation.
//
// The question Addressed cannot answer for itself: service/mail holds messages,
// not conversations, and whether the agent has spoken here is a fact about the
// record. Looked up by the ids the incoming message carries, which is the same
// way the run finds the conversation to continue.
func spoken(owner string, m mail.InboundMail) bool {
	refs := append(thread.Refs(m.InReplyTo), thread.Refs(m.References)...)
	if len(refs) == 0 {
		return false
	}
	t := thread.ByRef(owner, refs...)
	if t == nil {
		return false
	}
	for _, msg := range thread.Messages(owner, t.ID, 50) {
		if msg.Role == thread.RoleAgent {
			return true
		}
	}
	return false
}

// wanted reports whether this message is for the agent to answer.
//
// Always, for an ordinary one-to-one message — that is what writing to an agent
// is. In a thread with other people on it, the rule is in mail.Addressed and the
// default is silence.
func wanted(owner string, m mail.InboundMail) bool {
	return mail.Addressed(m.ToAgent, m.Others, spoken(owner, m), m.Text)
}

// introduction is the line the agent leads with the first time it appears in
// somebody else's thread.
//
// Once, and only where there is somebody who did not ask for it. Writing to your
// own agent needs no introduction, and repeating one on every message would be a
// signature.
//
// It says three things because all three are somebody else's reasonable
// question: what this is, who put it here, and how to make it stop. The last one
// matters most — a third party in your mail thread that you cannot remove is not
// a feature, and saying so up front is the difference between being useful and
// being something that happened to you.
func introduction(owner string, m mail.InboundMail, addr string) string {
	if len(m.Others) == 0 || spoken(owner, m) {
		return ""
	}
	who := strings.TrimSpace(m.FromName)
	if who == "" {
		who = m.From
	}
	return "_" + who + " copied me in — I'm an assistant that can look things up. " +
		"Drop me from the recipients any time and the thread carries on without me._\n\n"
}

// replyTo is who the answer goes to: the sender, with everybody else copied.
//
// Reply-all is the whole point. The reply landing only with the person who
// happened to send the last message would leave the rest of the thread watching
// half a conversation, and the next reply-all from anybody would carry the
// agent's answer to them anyway, out of order.
func replyTo(m mail.InboundMail) (to string, cc []string) {
	return m.From, m.Others
}
