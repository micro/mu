// Package chat is what an agent does when somebody speaks in a room.
//
// Not the chat service — that is service/chat, which holds the rooms, decides
// who is in them, and is the only thing that knows whether an agent was named.
// This is the half that answers, and the two are separated for the reason
// CLAUDE.md gives with a reason rather than a convention behind it: a service
// answers a question about state, an agent decides which question to ask, and
// a service calling a model is asking what its own answer should be.
//
// # What this replaces
//
// service/chat composed the replies itself: a hundred and ninety lines inside
// a websocket goroutine doing its own RAG over the index, its own decision
// about whether to search the web, its own history assembled from the room's
// last twenty messages, its own entity extraction for follow-up pronouns, and
// a model call.
//
// That is a hand-rolled agent, and the cost of it was not tidiness. It could
// reach two sources. The agent everywhere else in this product reaches every
// tool the instance has, keeps the conversation in the record, is metered, and
// remembers across clients. A room was the one place you talked to something
// worse, and nothing on the page said so.
//
// So this does not reimplement any of it. It hands the question to agent.Ask,
// which already has web search, the archive, the news, and the rest as tools —
// and already writes the turn down.
//
// # Why it subscribes
//
// The same inversion service/mail made. service/chat publishes on
// event.ChatForAgent only once its own gate has passed, and knows nothing
// about who listens; this subscribes. A hook would have been the other
// direction — a service reaching up into the agent through a function variable
// filled in at boot — which is how that rule gets avoided rather than kept.
package chat

import (
	"strings"

	"mu/agent"
	"mu/internal/app"
	"mu/internal/event"
	"mu/internal/thread"
	"mu/service/chat"
)

// Client is what the record calls a turn that happened in a room.
const Client = "chat"

// Load subscribes to the rooms.
func Load() {
	sub := event.Subscribe(event.ChatForAgent)
	go func() {
		for e := range sub.Chan {
			said, ok := spokenIn(e.Data)
			if !ok {
				app.Log("chat", "%s carried no message", event.ChatForAgent)
				continue
			}
			// One goroutine per message: answering is a model call and several
			// tool calls, and the subscription channel is small and drops when
			// it fills. Recovered, because a panic answering one message must
			// not stop the instance answering the next.
			go func(s spoken) {
				defer func() {
					if rec := recover(); rec != nil {
						app.Log("chat", "answering in %s panicked: %v", s.Room, rec)
					}
				}()
				answer(s)
			}(said)
		}
	}()
}

// spoken is one message that is expecting an answer.
type spoken struct {
	Room    string
	Title   string
	Summary string
	URL     string
	Account string
	Text    string
}

func spokenIn(data map[string]interface{}) (spoken, bool) {
	str := func(k string) string {
		v, _ := data[k].(string)
		return v
	}
	s := spoken{
		Room: str("room"), Title: str("title"), Summary: str("summary"),
		URL: str("url"), Account: str("account"), Text: str("text"),
	}
	if s.Room == "" || s.Text == "" {
		return spoken{}, false
	}
	return s, true
}

// answer asks the agent and puts what it says back in the room.
func answer(s spoken) {
	res, err := agent.Ask(agent.AskRequest{
		Account: s.Account,
		Client:  Client,
		// The room is the conversation, so the room id is the thread key. That
		// is what makes a second message in the same room a second turn rather
		// than a fresh question — which the old path did not have at all: it
		// rebuilt a history from the room's last twenty messages every time,
		// and none of it reached the record.
		Thread: s.Room,
		Text:   s.Text,
		// A room is not a DM. Several people can be in one, so the agent is
		// told not to reach for the owner's mail, wallet or notes — see
		// QueryOpts.Public.
		Public: true,
		// What this room is about, as framing for the medium — the same slot
		// mail uses to say whether the agent is alone with the sender or
		// copied into somebody else's conversation.
		System: about(s),
	})
	if err != nil {
		app.Log("chat", "could not answer in %s: %v", s.Room, err)
		return
	}
	reply := strings.TrimSpace(res.Text)
	if reply == "" {
		return
	}
	// Back the way it came. A websocket room and an XMPP client are two
	// different deliveries, and the conversation key says which — see
	// xmppRoom, which prefixes so the two namespaces cannot collide.
	if strings.HasPrefix(s.Room, "xmpp_") {
		if !chat.SayTo(s.Account, chat.AgentAddress(), reply) {
			app.Log("chat", "answered %s and nobody was connected", s.Account)
		}
		return
	}
	if !chat.Say(s.Room, chat.AgentName, reply) {
		app.Log("chat", "answered %s and the room had gone", s.Room)
	}
}

// about is what the room is discussing.
//
// Carried on the event rather than looked up, because looking it up would mean
// importing the service's state back into the thing that was decoupled from
// it.
//
// It says what the conversation is, not what the agent is — a room attached to
// a news article is a different medium from a bare room, in the same way mail
// with three people copied in is different from mail with one.
func about(s spoken) string {
	var b strings.Builder
	if s.Title != "" {
		b.WriteString("This conversation is about: " + s.Title)
	}
	if s.Summary != "" {
		if b.Len() > 0 {
			b.WriteString(". ")
		}
		b.WriteString(s.Summary)
	}
	if s.URL != "" {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("(Source: " + s.URL + ")")
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

// unused keeps the record's client vocabulary honest: the constant above has
// to be one the record knows, and thread is where that list lives.
var _ = thread.WebClient
