// Package chat is what an agent does when somebody speaks in a room.
//
// Not the chat service — that is service/chat, which holds the rooms, decides
// who is in them, and is the only thing that knows whether an agent was named.
// This is the half that answers, and the two are separated for the reason
// AGENTS.md gives with a reason rather than a convention behind it: a service
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
//
// The record's own constant rather than a second copy of the word: service/chat
// writes person-to-person XMPP exchanges into the record too (xmpp_record.go),
// a service may not import an agent, and two spellings that drifted would file
// the two halves of one conversation under two clients.
const Client = thread.ChatClient

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
			return
		}
		// SayTo only succeeds when they had a client connected, which is the
		// same fact Watching establishes for a room: the answer went to a
		// screen somebody was looking at. See seen.
		markSeen(s.Account, s.Room)
		return
	}
	if !chat.Say(s.Room, chat.AgentName, reply) {
		app.Log("chat", "answered %s and the room had gone", s.Room)
		return
	}
	seen(s.Room, s.Account)
}

// seen marks the conversation read when the answer landed in front of somebody.
//
// Reported as: talk in a room, get a reply, and an unread conversation appears
// in the inbox for a message you just watched arrive. The record is right —
// this happened, and it belongs in the account's threads — but the inbox is a
// notification hub as well as a record, and those two jobs disagree here. What
// arrived is not news to somebody who was in the room.
//
// The condition is a live socket on this room, not auth.OnlineUsers. That is a
// three minute window over the whole instance and answers "have they used this
// server lately", which is true of somebody reading their mail in another tab —
// and marking a message read for them would lose it. In the room, now, is the
// fact that means they saw it.
//
// Only the account that spoke. A room can hold several people and the others
// may well be away from it; each of them has their own thread and their own
// unread state, and this says nothing about theirs.
func seen(room, account string) {
	if !chat.Watching(room, account) {
		return
	}
	markSeen(account, room)
}

// markSeen stamps the conversation this room is recorded as.
//
// Split from seen because the two deliveries establish "they saw it" in
// different ways and neither should reimplement the lookup: a websocket room
// asks Watching, and an XMPP delivery has already been told by SayTo that a
// client was connected.
func markSeen(account, room string) {
	if account == "" || room == "" {
		return
	}
	// The room id is the thread key — see the Thread field on the Ask above,
	// which is what makes a second message in a room a second turn.
	if t := thread.Find(account, Client, room); t != nil {
		thread.MarkSeen(account, t.ID)
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
// about is what the agent is told before it reads the message.
//
// # It always says who the agent is
//
// This returned "" for any room with no subject — which is every conversation
// between two people. So the agent was handed the bare text with no framing at
// all, and somebody who typed "@micro ?" in a private room got this back:
//
//	@micro is an agent on this instance.
//	Name: micro · Address: micro@micro.mu · Agent: yes
//	Currently online: no
//
// It had looked its own handle up in the directory. With nothing saying it was
// @micro, "@micro ?" is a string containing a username, and looking up a
// username is a reasonable thing to do with one — so it reported its own
// profile, in its own voice, in the room it was speaking in, and said it was
// offline while saying it.
//
// The framing is not decoration, then. It is the difference between being
// addressed and being mentioned, and only the room knows which.
//
// # And where it is
//
// A room is not the agent page. Several people can be in one and the agent is
// not one of them: it is not on the roster and it answers when it is named.
// Told that, it can answer as a participant rather than as an assistant with a
// single user in front of it.
func about(s spoken) string {
	var b strings.Builder

	b.WriteString("You are @" + chat.AgentName + ", the agent on this instance. " +
		"You are answering in a chat room, where other people may be present and " +
		"can read what you say. Somebody has just spoken to you.\n\n" +
		"When a message names you — \"@" + chat.AgentName + "\", or your name on " +
		"its own — that is somebody addressing you, not asking you to look up an " +
		"account. Never answer with your own profile, and never report your own " +
		"presence: you are speaking, so you are here. If you have been named with " +
		"no question attached, say hello and ask what they need, briefly.")

	if s.Title != "" {
		b.WriteString("\n\nThis conversation is about: " + s.Title)
	}
	if s.Summary != "" {
		if s.Title != "" {
			b.WriteString(". ")
		} else {
			b.WriteString("\n\n")
		}
		b.WriteString(s.Summary)
	}
	if s.URL != "" {
		b.WriteString(" (Source: " + s.URL + ")")
	}
	return b.String()
}
