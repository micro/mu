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
}

// Answer is what came back, and where it was written down.
type Answer struct {
	Text string
	// Flow is the run record's id, so a client that later learns delivery
	// failed can mark it against the right turn.
	Flow string
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

	parent := ThreadHead(r.Account, r.Client, r.Thread)

	opts := QueryOpts{
		Public:  r.Public,
		History: ThreadHistory(r.Account, parent, historyTurns),
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

	answer, err := QueryWithOpts(r.Account, r.Text, opts)

	id := Record(Recorded{
		Account: r.Account, Agent: r.Agent,
		Source: r.Client, Trigger: r.Trigger,
		Prompt: r.Text, Answer: answer, Err: err,
		Parent: parent,
		Via:    Via{Client: r.Client, Thread: r.Thread},
	})

	// Notice anything worth remembering, from every client rather than one.
	// Off the response path: it is a background model call and the answer is
	// already written.
	if err == nil {
		go extractMemory(r.Account, r.Text)
	}

	return Answer{Text: answer, Flow: id}, err
}

// ThreadHead is the latest turn of a conversation, or "" if it is a new one.
//
// Scoped to the account as well as the client, because a thread id is a string
// somebody else's service chose and two accounts may well be handed the same
// one.
func ThreadHead(accountID, client, thread string) string {
	if accountID == "" || client == "" || thread == "" {
		return ""
	}
	// ListFlows is newest first, so the first match is the head and a new turn
	// hangs off the end of the conversation rather than its middle.
	for _, f := range ListFlows(accountID) {
		if f.Via.Client == client && f.Via.Thread == thread {
			return f.ID
		}
	}
	return ""
}

// ThreadHistory is a conversation's prior turns, oldest first, as the model
// wants them.
//
// Without it a chain is cosmetic: the turns group on a page and the agent still
// meets every message as a stranger, so "as we discussed" means nothing.
func ThreadHistory(accountID, flowID string, max int) []QueryMessage {
	if flowID == "" || max <= 0 {
		return nil
	}
	var out []QueryMessage
	for _, f := range sessionChain(accountID, flowID) {
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
