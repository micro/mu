package agent

// A conversation that arrived by mail, rather than a pile of unrelated runs.
//
// A conversation on the page has been a chain since there were sessions: each
// turn carries the id of the one before it, and ListSessions walks the chain to
// group them. Mail never joined it. Every message that woke an agent started a
// run with no parent, so writing three times was three strangers answering in
// sequence — the second reply could not see the first, and the person on the
// other end was the only one holding the thread together.
//
// What links them is what email already threads on. A reply carries In-Reply-To
// and References naming the messages it answers; the ids of both the message
// that came in and the reply that went out are kept on the turn, so the next
// message finds the turn it continues and hangs itself off it. Nothing new is
// stored about the conversation — the chain is the same chain the page uses,
// entered through a different door.
//
// That is the whole point of doing it this way rather than building a thread
// store beside it: a conversation you started in the tab can be continued by
// email, and one that arrived by email is readable in the tab, because there is
// only ever one of them.

import (
	"strings"
)

// MailTurn is what mail knows about a turn: the message that started it, the
// reply that went back, and who was on the other end.
type MailTurn struct {
	// InboundID is the Message-ID of what arrived. ReplyID is the Message-ID of
	// what we sent back — a follow-up answers *that*, so it is the one most
	// lookups match on.
	InboundID string
	ReplyID   string
	// From is the address that wrote in. A turn somebody else set off is not
	// the same thing as one the owner typed, and the difference should be data
	// rather than a sentence in the trigger.
	From string
}

// ContinuesMail finds the turn a message answers, given its In-Reply-To and
// References headers.
//
// Both headers, because a mail client may send either: In-Reply-To names the
// immediate parent and References the whole chain, and a client that sends only
// References is common enough that matching just the first loses the thread.
// The newest match wins, so a long chain continues from its head.
func ContinuesMail(accountID, inReplyTo, references string) string {
	ids := mailIDs(inReplyTo + " " + references)
	if len(ids) == 0 {
		return ""
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	// ListFlows is newest first, so the first match is the latest turn in the
	// chain and the new turn hangs off the head rather than the middle.
	for _, f := range ListFlows(accountID) {
		if want[f.Mail.ReplyID] || want[f.Mail.InboundID] {
			return f.ID
		}
	}
	return ""
}

// Delivered records the Message-ID a turn's answer went out as, so the reply to
// that reply finds its way back here.
//
// Separate from Record because the id does not exist until the message is
// actually sent, and a turn is written down before that — a reply that fails to
// send still has to be visible to the owner.
func Delivered(flowID, messageID string) {
	if flowID == "" || strings.TrimSpace(messageID) == "" {
		return
	}
	updateFlow(flowID, func(f *Flow) { f.Mail.ReplyID = messageID })
}

// MailHistory is a conversation's prior turns, oldest first, as the model wants
// them.
//
// This is what makes a thread a conversation rather than a chain that merely
// looks like one: without it the agent is handed the newest message with no
// idea what was agreed two mails ago, and answers as a stranger every time.
//
// Bounded, because a long-running thread would otherwise grow the prompt until
// it costs more than it is worth, and the oldest turns are the least relevant.
func MailHistory(accountID, flowID string, max int) []QueryMessage {
	if flowID == "" || max <= 0 {
		return nil
	}
	chain := sessionChain(accountID, flowID)
	var out []QueryMessage
	for _, f := range chain {
		if f.Prompt != "" {
			out = append(out, QueryMessage{Role: "user", Text: f.Prompt})
		}
		if f.Answer != "" {
			out = append(out, QueryMessage{Role: "assistant", Text: f.Answer})
		}
	}
	// Trimmed from the front: the recent turns are the ones being answered.
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

// mailIDs pulls <...> bracketed Message-IDs out of a header value.
//
// Mail has its own copy of this for the inbound filter. It is four lines and
// duplicating it is cheaper than a shared package, or than agent importing a
// service to parse a string.
func mailIDs(s string) []string {
	var ids []string
	for {
		start := strings.Index(s, "<")
		if start < 0 {
			return ids
		}
		end := strings.Index(s[start:], ">")
		if end < 0 {
			return ids
		}
		ids = append(ids, s[start:start+end+1])
		s = s[start+end+1:]
	}
}
