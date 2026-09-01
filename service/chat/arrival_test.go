package chat

// A conversation between two people, with a third party in it.
//
// Reported from a real transcript. Somebody started a chat with one other
// person and got this:
//
//	micro   @asim joined
//	asim    hello?
//	asim    @micro ?
//	micro   @micro is an agent on this instance.
//	        Name: micro · Address: micro@micro.mu · Agent: yes
//	        Currently online: no
//
// Three separate faults, and every one of them says @micro is in the room:
//
//  1. The arrival line went out as an agent message, so the client drew it
//     with micro's byline — the room talking about itself, with a name on it.
//  2. The roster appended the agent to every room, so a two-person
//     conversation listed three.
//  3. Asked "@micro ?" the agent looked its own handle up in the directory and
//     reported its own profile, offline, in the room it was speaking in. It
//     had no framing at all — see agent/chat.about, which returned nothing for
//     a room with no subject, which is every conversation between two people.
//
// The first two are here. The third is in agent/chat.

import (
	"strings"
	"testing"
	"time"
)

// Somebody arriving is the room's line, not the agent's.
func TestAnArrivalIsNotSomethingTheAgentSaid(t *testing.T) {
	room := &Room{
		ID:        PairRoom("arra", "arrb"),
		Broadcast: make(chan RoomMessage, 4),
	}
	room.arrival("arra", "joined")

	select {
	case m := <-room.Broadcast:
		if !m.System {
			t.Error("the arrival is not marked as the room's own line, so the " +
				"client draws it with a byline")
		}
		if m.IsLLM {
			t.Error("the arrival is marked as the agent talking — which is what " +
				"put micro's name on the first line of a private conversation")
		}
		if m.UserID != "" {
			t.Errorf("the arrival is filed under @%s, and nobody said it", m.UserID)
		}
		if !strings.Contains(m.Content, "@arra joined") {
			t.Errorf("the line does not say what happened: %q", m.Content)
		}
	default:
		t.Fatal("nothing was said when somebody arrived")
	}

	// The agent's own comings and goings are not announced, and neither are
	// arrivals in a public room — those are the two cases arrival already
	// refused and they have to stay refused.
	room.arrival(agentName, "joined")
	lobby := &Room{ID: Lobby, Broadcast: make(chan RoomMessage, 4)}
	lobby.arrival("arra", "joined")
	select {
	case m := <-room.Broadcast:
		t.Errorf("the agent announced itself into a private room: %q", m.Content)
	default:
	}
	select {
	case m := <-lobby.Broadcast:
		t.Errorf("a public room announced an arrival: %q — that is the thing that "+
			"makes a busy room unreadable", m.Content)
	default:
	}
}

// And the window on Home does not quote it as something a person said.
//
// Latest skips the agent so the line under the count is a person talking. An
// arrival has no author at all now, so without this it would be the newest
// thing in the room and get drawn as one.
func TestAnArrivalIsNotTheLatestThingSaid(t *testing.T) {
	const room = "chat_arrival"
	now := time.Now()
	liveRoom(t, room, RoomMessage{UserID: "somebody", Content: "morning", Timestamp: now})

	roomsMutex.Lock()
	rooms[room].Messages = append(rooms[room].Messages, RoomMessage{
		Content: "@newcomer joined", System: true, Timestamp: now.Add(time.Second),
	})
	roomsMutex.Unlock()

	who, text, _ := Latest(room)
	if who == "" || text != "morning" {
		t.Errorf("Latest = (%q, %q) — an arrival is being quoted under the count "+
			"of who is online as though somebody said it", who, text)
	}
}
