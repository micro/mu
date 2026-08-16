package agent

// One way in, for every client.
//
// Discord, Telegram, WhatsApp, mail and the web page all ended up at
// QueryWithOpts, so the agent itself was never the problem. Everything around
// it was written five times and diverged: three clients kept conversation
// history in a map in memory, keyed by whatever id that service happened to
// use, lost on restart and invisible everywhere else. None of them wrote down
// that the run had happened, so a WhatsApp conversation left no trace on
// /agents. And memory — the durable facts an agent learns about somebody — was
// only ever extracted on the web, so an agent remembered what you typed into a
// browser and forgot everything you said anywhere else.
//
// Ask is the surround, in one place: find the conversation this message
// continues, give the agent what was already said, run it, write the turn down,
// and notice anything worth remembering. A client is left doing the only thing
// it can do — speaking its own protocol, and naming an agent when the address
// or the command already chose one.
//
// Three words that are not the same thing, and are kept apart deliberately:
//
//   History  the turns of one thread. Per thread, per client, persisted.
//   Memory   durable facts about an account, across every client. notes.
//   Context  what is assembled for one run out of both, plus live tool data.
//            Assembled fresh each time, never stored.

import (
	"strings"

	"mu/internal/thread"
)

// AskRequest is one message arriving from a client.
type AskRequest struct {
	Account string
	// Client is which one: discord, telegram, whatsapp, mail, web, cli. Named
	// for the directory rather than for an abstraction, because that is the
	// word anybody would use for it.
	Client string
	// Thread is the client's own identifier for the conversation — a channel
	// id, a phone number, the root message id of a mail chain. Opaque here:
	// only the client knows what a conversation is on its own service.
	//
	// Not "session", which already means the chain of turns itself, an SMTP
	// connection, and a login.
	Thread string
	Text   string
	// Public skips private context — a group chat is not a DM.
	Public bool
	// Agent names one when the client already decided: a slash command, or
	// agent+news@. Empty means the account's default, which is Micro.
	Agent string
	// System is extra framing for the medium, prepended to the agent's own
	// prompt. Setting it also means the caller has chosen, so the keyword
	// router does not get a second vote — see QueryWithOpts.
	System string
	// Trigger says who set this off, in words, for the run record.
	Trigger string
	// Ref is what the client says this message answers — mail's In-Reply-To
	// and References. Where it matches something recorded, that conversation
	// is continued in preference to whatever is newest on the thread.
	Ref string
	// From is who wrote in, where that is not simply the account: a message
	// somebody else sent to an address this account owns.
	From string
	// Via carries anything else the client needs to recognise this
	// conversation later — mail's message ids. Client and Thread are filled
	// in from the fields above, so a caller cannot set one and mean another.
	Via Via
}

// Answer is what came back, and where it was written down.
type Answer struct {
	Text string
	// Flow is the workflow record's id, so a client that later learns delivery
	// failed can mark it against the right run.
	Flow string
	// Thread is the conversation it was recorded on, so a client can note the
	// id its own protocol gave the reply — see Sent.
	Thread string
}

// historyTurns is how much of a conversation an agent is reminded of.
//
// Six messages is three exchanges. Enough to hold a thread together, and
// bounded so a conversation somebody has been adding to for a month does not
// cost more in prompt than the answer is worth.
const historyTurns = 6

// Ask runs one turn of a conversation and remembers it happened.
func Ask(r AskRequest) (Answer, error) {
	if strings.TrimSpace(r.Account) == "" || strings.TrimSpace(r.Text) == "" {
		return Answer{}, nil
	}

	// The conversation this belongs to, in the system of record. A client that
	// knows better says so — mail resolves a reply from its headers, which
	// beats "the last thing on this thread" when somebody answers a message
	// from last week.
	var th *thread.Thread
	if r.Ref != "" {
		th = thread.ByRef(r.Account, r.Ref)
	}
	if th == nil {
		th = thread.Open(r.Account, r.Client, r.Thread)
	}

	opts := QueryOpts{
		Public:  r.Public,
		History: history(r.Account, th, historyTurns),
	}
	if plat := Platform(r.Agent); r.Agent != "" && plat != nil {
		opts.System = PlatformOpts(plat).System
	} else if r.Agent != "" {
		// One of the account's own. Unknown names fall through to the default
		// rather than failing: a client naming an agent that no longer exists
		// should still get an answer.
		if o, err := AskAs(r.Account, r.Agent); err == nil {
			opts.System = o.System
			opts.Tools = o.Tools
		}
	}
	if strings.TrimSpace(r.System) != "" {
		opts.System = strings.TrimSpace(r.System) + "\n\n" + opts.System
		opts.System = strings.TrimSpace(opts.System)
	}

	// What was said goes in the record whatever the run did. An agent that
	// failed was still written to, and the next message continues the same
	// conversation.
	if th != nil {
		thread.Add(thread.Message{
			Thread: th.ID, Account: r.Account, Role: thread.RolePerson,
			Text: r.Text, Ref: r.Ref, From: r.From,
		})
	}

	answer, err := QueryWithOpts(r.Account, r.Text, opts)

	via := r.Via
	via.Client, via.Thread = r.Client, r.Thread

	// The workflow record: how the answer was produced, which is a different
	// question with a different lifetime from what was said.
	id := Record(Recorded{
		Account: r.Account, Agent: r.Agent,
		Source: r.Client, Trigger: r.Trigger,
		Prompt: r.Text, Answer: answer, Err: err,
		Via: via,
	})

	if th != nil && strings.TrimSpace(answer) != "" {
		thread.Add(thread.Message{
			Thread: th.ID, Account: r.Account, Role: thread.RoleAgent,
			Text: answer, Workflow: id,
		})
	}

	// Notice anything worth remembering, from every client rather than one.
	// Off the response path: it is a background model call and the answer is
	// already written.
	if err == nil {
		go extractMemory(r.Account, r.Text)
	}

	return Answer{Text: answer, Flow: id, Thread: threadID(th)}, err
}

// history is a conversation's prior messages, as the model wants them.
//
// Read from the record rather than from workflow records, which is the whole
// point of there being a record: a workflow is how one answer was produced and
// expires as debugging does, while what was said is memory and does not.
func history(accountID string, th *thread.Thread, max int) []QueryMessage {
	if th == nil || max <= 0 {
		return nil
	}
	msgs := thread.Messages(accountID, th.ID, max)
	out := make([]QueryMessage, 0, len(msgs))
	for _, m := range msgs {
		role := "user"
		if m.Role == thread.RoleAgent {
			role = "assistant"
		}
		out = append(out, QueryMessage{Role: role, Text: m.Text})
	}
	return out
}

// Sent records the id a client's own protocol gave an answer, so a reply to it
// finds this conversation. Mail's Message-ID; nothing for a client without one.
func Sent(accountID, threadID, ref string) {
	if threadID == "" || strings.TrimSpace(ref) == "" {
		return
	}
	for _, m := range thread.Messages(accountID, threadID, 1) {
		if m.Role == thread.RoleAgent {
			thread.SetRef(accountID, m.ID, ref)
		}
	}
}

func threadID(th *thread.Thread) string {
	if th == nil {
		return ""
	}
	return th.ID
}
