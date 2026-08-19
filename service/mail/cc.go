package mail

// Being CC'd into somebody else's conversation.
//
// Every agent product is one person talking to one agent, in a place you have
// to go to. This is the other shape: you are already writing to your brother
// about when the train leaves, you put the agent in Cc, and it answers in the
// thread with both of you reading it. Nobody has to migrate, nobody installs
// anything, and the agent arrives where the conversation already is.
//
// SMTP does not distinguish To from Cc — both are RCPT TO, so a CC'd message
// already reached the server. What was missing is any notion of being a
// *participant* rather than the addressee: the reply went to the sender alone,
// so the agent answered one person in a room of three.
//
// # Who else is here
//
// Others is everybody on the message who is not this instance and not the
// sender. It is what the reply goes to, and it is what makes the difference
// between a conversation and a private exchange visible to the code that has to
// decide how to behave.
//
// # When it speaks
//
// Not on every message, which is the failure mode that would make this
// unusable. Three people having a conversation with an agent that answers every
// turn is worse than no agent: it is a model call each time, it is noise in
// somebody else's thread, and it is the reason people mute things.
//
// So: it answers when it is spoken to (in To), and when it has just been added
// (in Cc, having never spoken here). After that it is quiet unless somebody
// names it. See Addressed.

import (
	"net/mail"
	"strings"
)

// Recipients pulls the addresses out of a To or Cc header.
//
// Tolerant of what real clients send: a header that will not parse as a list
// still usually has one good address in it, and dropping the whole line because
// one entry is malformed loses everybody on it.
func Recipients(header string) []string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	if list, err := mail.ParseAddressList(header); err == nil {
		out := make([]string, 0, len(list))
		for _, a := range list {
			if addr := strings.TrimSpace(a.Address); addr != "" {
				out = append(out, addr)
			}
		}
		return out
	}
	var out []string
	for _, part := range strings.Split(header, ",") {
		if a, err := mail.ParseAddress(strings.TrimSpace(part)); err == nil && a.Address != "" {
			out = append(out, a.Address)
		}
	}
	return out
}

// Others is everybody else on a message: who the reply is copied to.
//
// Excluded: the sender, who is the To of the reply rather than the Cc, and the
// address this delivery is answering as, because copying yourself is silly.
//
// **Not** excluded: every other address on this instance. That was the first
// version and it was wrong twice over.
//
// It dropped human beings. A Mu user is a person with an address at the mail
// domain, so a thread between two accounts here had one of them silently
// removed from the reply — the loop guard eating a participant.
//
// And it defended against a loop that is already guarded, one layer down and in
// the better place: mayDispatch refuses any message whose *sender* is one of
// this instance's agent addresses. The guard belongs on who wrote, not on who
// was copied. With it there, two agents can sit on the same thread — CC
// agent+news@ and agent+markets@ and both answer — and neither wakes on the
// other's reply, because the human is always the one who triggers a run.
//
// Order is preserved and duplicates are dropped, because the same person is
// routinely in both To and Cc and being copied twice is how a reply-all looks
// broken.
func Others(to, cc []string, sender, self string) []string {
	seen := map[string]bool{
		strings.ToLower(strings.TrimSpace(sender)): true,
		strings.ToLower(strings.TrimSpace(self)):   true,
	}
	var out []string
	for _, addr := range append(append([]string{}, to...), cc...) {
		addr = strings.TrimSpace(addr)
		key := strings.ToLower(addr)
		if addr == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, addr)
	}
	return out
}

// Ours reports whether an address is delivered by this instance.
//
// Both the shared agent address and anything at the mail domain: an account's
// own you+research@ is as much a loop as agent@ is, and a reply-all that copies
// the mailbox it came from would wake the thing that just spoke.
func Ours(addr string) bool {
	if fromSharedAgent(addr) {
		return true
	}
	domain := ConfiguredDomain()
	if domain == "" || addr == "" {
		return false
	}
	_, at, found := strings.Cut(addr, "@")
	return found && strings.EqualFold(at, domain)
}

// Addressed reports whether a message is actually talking to the agent, as
// opposed to being a message in a thread the agent happens to be on.
//
// The distinction only exists once there are other people here. A message to
// agent@ with nobody else on it is always addressed to the agent — that is the
// whole of what it is.
//
// With other people on it:
//
//   - In To: somebody put it there deliberately. Answer.
//   - In Cc and it has not spoken here before: this is the message that added
//     it, which is exactly the moment it should introduce itself. Answer.
//   - In Cc and it has spoken: it is on the thread and is expected to listen.
//     Answer only if somebody names it — "@agent, what time is that train" —
//     because otherwise it interrupts every turn of a conversation between two
//     other people.
//
// spoken is whether the agent has already answered on this conversation, which
// the caller knows and this package does not: mail holds no conversations, the
// record does.
func Addressed(toAgent bool, others []string, spoken bool, body string) bool {
	if len(others) == 0 {
		return true
	}
	if toAgent || !spoken {
		return true
	}
	return Named(body)
}

// Named reports whether a message mentions the agent.
//
// Deliberately shallow: the local part of the shared address, with or without
// an @ in front of it. Asking a model whether it was being addressed would be a
// model call to decide whether to make a model call, on every message of every
// thread it sits on — which is the cost this whole rule exists to avoid.
//
// The cost of getting it wrong is asymmetric and the default is silence: a
// missed mention is somebody writing again with the name in it, and a false
// positive is an agent interrupting a private conversation.
func Named(body string) bool {
	lower := strings.ToLower(body)
	if strings.Contains(lower, "@"+AgentMailbox) {
		return true
	}
	if addr := strings.ToLower(SharedAgentAddress()); addr != "" && strings.Contains(lower, addr) {
		return true
	}
	// The bare word, as a word: "agent, can you check" but not "management".
	for _, f := range strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if f == AgentMailbox {
			return true
		}
	}
	return false
}

// inList reports whether an address is in a header's recipients.
//
// Case-insensitively, because a mail client will write Agent@Example.com if
// that is what somebody typed and the comparison decides whether the agent
// thinks it was spoken to.
func inList(list []string, addr string) bool {
	addr = strings.TrimSpace(addr)
	for _, a := range list {
		if strings.EqualFold(strings.TrimSpace(a), addr) {
			return true
		}
	}
	return false
}
