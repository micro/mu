package thread

// A conversation has parties, not two sides.
//
// It had an owner and an agent, which is true of a chat on a page and of
// nothing else. Mail already broke it: a stranger writes to agent@, the agent
// answers, and the owner is a third party who can read the exchange and was
// never in it. A group is four people and an agent. An agent writing to another
// agent is two agents and nobody.

import (
	"testing"
	"time"
)

// Writing puts you on a conversation, so no client has to know this exists.
func TestWhoeverSpeaksIsOnTheConversation(t *testing.T) {
	reset(t)

	th := Open("asim", "mail", "<root@example.com>")
	SetAgent("asim", th.ID, "agent-42")
	Add(Message{Thread: th.ID, Account: "asim", Text: "what are your rates?",
		From: "client@example.com"})
	Add(Message{Thread: th.ID, Account: "asim", Role: RoleAgent, Text: "£500 a day."})

	parties := Parties("asim", th.ID)
	if len(parties) != 2 {
		t.Fatalf("a stranger wrote in and the agent answered, and the conversation "+
			"has %d parties: %+v", len(parties), parties)
	}

	var person, agent *Party
	for i, p := range parties {
		switch p.Kind {
		case RolePerson:
			person = &parties[i]
		case RoleAgent:
			agent = &parties[i]
		}
	}
	if person == nil || person.Key != "client@example.com" {
		t.Errorf("the person who wrote in is not on the conversation: %+v", parties)
	}
	if agent == nil {
		t.Fatalf("the agent that answered is not on the conversation: %+v", parties)
	}
	if agent.Agent != "agent-42" {
		t.Errorf("the agent party does not say which agent (%q) — a conversation with "+
			"one specialist and a conversation with another are different conversations",
			agent.Agent)
	}

	// Saying more does not add you twice.
	Add(Message{Thread: th.ID, Account: "asim", Text: "and for a week?",
		From: "client@example.com"})
	if n := len(Parties("asim", th.ID)); n != 2 {
		t.Errorf("a second message made %d parties", n)
	}
}

// The owner and the agent are two parties, not one.
//
// Both have an empty key — the owner because that is how every message written
// before parties existed identifies its author — so matching on the key alone
// would collapse a conversation to a single party talking to itself.
func TestTheOwnerAndTheAgentAreNotTheSameParty(t *testing.T) {
	reset(t)

	th := Open("asim", "web", "session-1")
	Add(Message{Thread: th.ID, Account: "asim", Text: "hello"})
	Add(Message{Thread: th.ID, Account: "asim", Role: RoleAgent, Text: "hi"})

	parties := Parties("asim", th.ID)
	if len(parties) != 2 {
		t.Fatalf("you and your agent are %d parties: %+v", len(parties), parties)
	}
	if parties[0].Kind == parties[1].Kind {
		t.Errorf("both parties are %q", parties[0].Kind)
	}
}

// Somebody added who has not spoken is still on it.
//
// This is the case that cannot be derived from messages, and it is the whole of
// what a shared inbox needs: the colleague who was added and has said nothing is
// still supposed to see it.
func TestSomebodyCanBeAddedBeforeTheySpeak(t *testing.T) {
	reset(t)

	th := Open("asim", "mail", "<root@example.com>")
	Add(Message{Thread: th.ID, Account: "asim", Text: "kicking this off"})

	Join("asim", th.ID, Party{Kind: RolePerson, Key: "sam@example.com", Name: "Sam"})
	parties := Parties("asim", th.ID)
	if len(parties) != 2 {
		t.Fatalf("adding somebody made %d parties: %+v", len(parties), parties)
	}

	// Idempotent, and a name learned late is kept.
	Join("asim", th.ID, Party{Kind: RolePerson, Key: "sam@example.com"})
	if n := len(Parties("asim", th.ID)); n != 2 {
		t.Errorf("adding the same person twice made %d parties", n)
	}
	Join("asim", th.ID, Party{Kind: RolePerson, Key: "", Name: "Asim"})
	for _, p := range Parties("asim", th.ID) {
		if p.Kind == RolePerson && p.Key == "" && p.Name != "Asim" {
			t.Errorf("a name learned after the first message was dropped: %+v", p)
		}
	}
}

// Being a party to somebody else's conversation grants nothing.
//
// This step records who is on a conversation. It deliberately does not decide
// who may read one — every read is still scoped to the account the thread
// belongs to — because widening that is a separate decision and making it by
// accident is how a private record stops being one.
func TestAPartyCannotReadSomebodyElsesConversation(t *testing.T) {
	reset(t)

	th := Open("asim", "mail", "<root@example.com>")
	Add(Message{Thread: th.ID, Account: "asim", Text: "confidential"})
	Join("asim", th.ID, Party{Kind: RolePerson, Key: "sam@example.com", Name: "Sam"})

	// Sam has an account here and is on the thread. That is still not access.
	if Get("sam", th.ID) != nil {
		t.Error("a party read another account's conversation")
	}
	if got := Messages("sam", th.ID, 0); got != nil {
		t.Errorf("a party read %d of another account's messages", len(got))
	}
	if got := Parties("sam", th.ID); got != nil {
		t.Errorf("a party read another account's participant list")
	}
	if got := Search("sam", "confidential", "", 10); len(got) != 0 {
		t.Errorf("a party found %d of another account's messages by searching", len(got))
	}
	// And cannot add themselves to it.
	Join("sam", th.ID, Party{Kind: RolePerson, Key: "someone@else.com"})
	if n := len(Parties("asim", th.ID)); n != 2 {
		t.Errorf("another account added a party to my conversation (%d now)", n)
	}
}

// Parties survive being written and read back.
func TestPartiesArePersisted(t *testing.T) {
	reset(t)

	th := Open("asim", "mail", "<root@example.com>")
	Add(Message{Thread: th.ID, Account: "asim", Text: "hello", From: "client@example.com",
		At: time.Now().UTC()})

	for _, p := range List("asim", 0) {
		if p.ID != th.ID {
			continue
		}
		if len(p.Parties) == 0 {
			t.Error("a conversation came back from the list with no parties, so they " +
				"are not on the thread itself and will not be written to disk")
		}
	}
}
