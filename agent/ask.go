package agent

// One way in, for every client.
//
// mail and the web page all ended up at
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
	"errors"
	"strings"

	"mu/internal/notes"
	"mu/internal/safety"
	"mu/internal/thread"
)

// errNoConversation is a caller naming a conversation that is not theirs, or is
// not there. Not "forbidden": scoped by account, somebody else's id is not a
// thing that exists here.
var errNoConversation = errors.New("no conversation with that id")

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
	// On names a conversation in the record that the caller already has, by id.
	// Set it and Thread and Ref are not consulted — there is nothing to find.
	//
	// For a surface that is looking at a conversation rather than receiving a
	// message on one: the inbox, where somebody reads an email and tells the
	// agent to do something about it. Without this such a caller has to
	// reconstruct the client's own key for a thread it is holding a pointer to,
	// and get it right, which is how a second way of writing the record starts.
	On   string
	Text string
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
	//
	// Read only. It is a chain of ids, not one, and it names other messages —
	// which is why it is not what gets stored against this one. See MessageRef.
	Ref string
	// MessageRef is this message's own identifier on the client, where it has
	// one: mail's Message-ID. Stored against the message, so a later reply
	// naming it finds this conversation, and so that two callers who both saw
	// the same arrival record it once — see thread.Add.
	//
	// It used to be Ref that was written down, which is a different fact said in
	// the same field: the ids a message *answers*, stored as the id a message
	// *is*. Nothing ever matched it — ByRef compares one id at a time and a
	// References header is several — so mail threaded on the agent's replies
	// alone, and two answers to one message shared a "ref" that identified
	// neither.
	MessageRef string
	// From is who wrote in, where that is not simply the account: a message
	// somebody else sent to an address this account owns.
	From string
	// As is the address this agent answers from, where it has one.
	//
	// Recorded on the answer so "has this agent spoken here" can be asked of
	// one agent rather than of the conversation. Empty everywhere but mail,
	// which is the only client where two agents can be in the same
	// conversation — see AnsweredAs.
	As string

	// FromName is what to call them, where the client knew — a mail display
	// name. It belongs to the party rather than to each message, so it is
	// recorded once against the conversation and not on every line.
	FromName string
	// Via carries anything else the client needs to recognise this
	// conversation later — mail's message ids. Client and Thread are filled
	// in from the fields above, so a caller cannot set one and mean another.
	Via Via
	// Stream reports the answer as it is produced, for a client that can show
	// it arriving — the web streams to the page. An option for the caller and not a fork in
	// the implementation: with nobody listening the same run happens and the
	// same answer comes back, so a streaming client and a quiet one cannot
	// drift apart.
	//
	// Meaningless on mail, which is the one client with nothing to update.
	Stream StreamHooks
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
	switch {
	case r.On != "":
		// The caller is holding it. Scoped to the account by Get, so an id from
		// somewhere else is not a conversation.
		th = thread.Get(r.Account, r.On)
		if th == nil {
			return Answer{}, errNoConversation
		}
	default:
		if r.Ref != "" {
			th = thread.ByRef(r.Account, r.Ref)
		}
		if th == nil {
			th = thread.Open(r.Account, r.Client, r.Thread)
		}
	}
	// Who the conversation is with, so a surface that has an agent selected can
	// show that agent's conversations rather than all of them or none.
	thread.SetAgent(r.Account, threadID(th), r.Agent)
	// And who is on it. Writing puts you on a conversation by itself — see
	// thread.Add — so this is only for what a message cannot carry: the name
	// behind an address.
	if r.From != "" && strings.TrimSpace(r.FromName) != "" {
		thread.Join(r.Account, threadID(th), thread.Party{
			Kind: thread.RolePerson, Key: r.From, Name: strings.TrimSpace(r.FromName)})
	}

	opts := QueryOpts{
		Public:  r.Public,
		History: History(r.Account, threadID(th), historyTurns),
		Stream:  r.Stream,
	}
	if plat := Platform(r.Agent); r.Agent != "" && plat != nil {
		// Both halves. It took System only, so every one of this instance's
		// agents arrived at the chat and at agent+weather@ holding every tool
		// on the box — the allow-list that makes a specialist a specialist was
		// read on the Execute path and dropped on this one. See PlatformOpts,
		// which returns the pair for exactly this reason.
		o := PlatformOpts(plat)
		opts.System, opts.Tools = o.System, o.Tools
		// And what this agent knows about you, which is the thing that made
		// eleven agents worth having rather than one prompt eleven ways. The
		// scope was declared in the registry and read in one place — the
		// planner inside micro.Execute — so a conversation held here, or by
		// mail, never saw it. See notes.ForScopedContext.
		if plat.MemoryScope != "" && !r.Public {
			if mem := notes.ForScopedContext(r.Account, plat.MemoryScope); mem != "" {
				opts.System += "\n\nWhat you know about them:\n" + mem
			}
		}
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

	Said(r.Account, threadID(th), r.Text, r.MessageRef, r.From)

	// The one category that is refused wherever it appears, before the model is
	// asked and before anything is charged.
	//
	// Only that category here. The full generation policy belongs where
	// somebody asks for something to be made — see service/images — because an
	// agent is handed text it did not choose: an arriving email, a fetched
	// page, a message from somebody else. Refusing to answer because that text
	// mentions something is how an inbox stops working.
	if reason, refused := safety.NeverAllowed(r.Text); refused {
		Answered(r.Account, threadID(th), reason, "")
		return Answer{Text: reason, Thread: threadID(th)}, nil
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

	AnsweredAs(r.Account, threadID(th), answer, id, r.As)

	// Notice anything worth remembering, from every client rather than one.
	// Off the response path: it is a background model call and the answer is
	// already written.
	if err == nil {
		go extractMemory(r.Account, r.Text, scopeOf(r.Agent))
	}

	return Answer{Text: answer, Flow: id, Thread: threadID(th)}, err
}

// Opened is the conversation a message belongs to, started if it is new.
//
// Exported because a client that drives its own run still has to write to the
// record — the web streams, so it cannot hand the whole turn to Ask and wait.
// What it must not do is write its own version of the record, which is why
// these three are the only way in and Ask uses them too.
func Opened(account, client, key, ref, agentID string) string {
	id := ""
	if ref != "" {
		if th := thread.ByRef(account, ref); th != nil {
			id = th.ID
		}
	}
	if id == "" {
		id = threadID(thread.Open(account, client, key))
	}
	thread.SetAgent(account, id, agentID)
	return id
}

// Said records what a person wrote. It happens before the run, so a message
// that nothing answers is still in the record.
func Said(account, threadID, text, ref, from string) {
	SaidTo(account, threadID, text, ref, from, "")
}

// SaidTo is Said where the client also knows which of the account's addresses
// the message arrived at.
//
// A separate function rather than a sixth argument on Said, because Said has
// five callers across the clients and only mail has a "to" worth recording —
// a message in a shared room arrives at the room, not at one of your addresses.
func SaidTo(account, threadID, text, ref, from, to string) {
	if threadID == "" {
		return
	}
	thread.Add(thread.Message{
		Thread: threadID, Account: account, Role: thread.RolePerson,
		Text: text, Ref: ref, From: from, To: to,
	})
}

// Answered records what the agent replied, and which workflow produced it.
func Answered(account, threadID, text, workflow string) {
	AnsweredAs(account, threadID, text, workflow, "")
}

// AnsweredAs is Answered, recording which address answered.
//
// Empty for every client but mail, where it is the address the reply goes out
// from — agent@, agent+news@, you+research@. It is the answer to "has *this*
// agent spoken on this conversation", which is not the same question as "has an
// agent spoken", and the difference only shows once two of them are on one
// thread: copy agent+news@ and agent+markets@ into the same mail and whichever
// ran first would otherwise silence the other, because the rule that keeps an
// agent from interrupting a conversation it has already joined would read the
// other agent's answer as its own.
func AnsweredAs(account, threadID, text, workflow, from string) {
	if threadID == "" || strings.TrimSpace(text) == "" {
		return
	}
	thread.Add(thread.Message{
		Thread: threadID, Account: account, Role: thread.RoleAgent,
		Text: text, Workflow: workflow, From: from,
	})
}

// History is a conversation's prior messages, as the model wants them.
func History(account, threadID string, max int) []QueryMessage {
	if threadID == "" || max <= 0 {
		return nil
	}
	msgs := thread.Messages(account, threadID, max)
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
