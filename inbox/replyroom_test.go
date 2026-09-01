package inbox

// A room conversation is answered in the room, and that is Reply.
//
// Reported: "so many of these chat threads are assign to agent, why not
// Reply". Assign was the only control on the row, which made it read as the
// thing you do with a chat thread. It was not — it was the second verb on a
// row whose first one was missing, because Reply was drawn only where there
// was an address to compose to and a room has none.
//
// The way back existed the whole time, as a link inside the explanatory
// sentence underneath. A link in a paragraph is not an action.

import (
	"strings"
	"testing"

	"mu/internal/thread"
)

func TestAChatThreadIsRepliedToInTheRoom(t *testing.T) {
	const who = "replyroom"
	const roomID = "dm_replyroom_other"

	th := thread.Open(who, thread.ChatClient, roomID)
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{
		Thread: th.ID, Account: who, Role: thread.RolePerson, Text: "@micro ?",
	})
	live := thread.Get(who, th.ID)
	msgs := thread.Messages(who, th.ID, MessagesShown)

	got := conversationPane(who, live, msgs, false, false, "")

	if !strings.Contains(got, ">Reply<") {
		t.Errorf("a chat thread offers no Reply, so the only thing on the row is "+
			"what you do with it occasionally:\n%s", got)
	}
	if !strings.Contains(got, "/chat?id="+roomID) {
		t.Errorf("Reply does not go to the room the conversation happened in:\n%s", got)
	}

	// And the sentence explaining that a reply carries on elsewhere is gone,
	// because the button above it now says so and carries the same link.
	if strings.Contains(got, "so a reply carries on there") {
		t.Errorf("the note repeats the button directly above it:\n%s", got)
	}
}

// Everything that genuinely cannot be answered here still says so.
//
// The note is not dead: a conversation that arrived on a channel with no way
// back needs the sentence, and removing it for chat must not remove it for the
// rest.
func TestAThreadWithNowhereToReplyStillSaysSo(t *testing.T) {
	const who = "replynone"

	th := thread.Open(who, "web", "web_replynone")
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{
		Thread: th.ID, Account: who, Role: thread.RolePerson, Text: "hello",
	})
	live := thread.Get(who, th.ID)
	msgs := thread.Messages(who, th.ID, MessagesShown)

	got := conversationPane(who, live, msgs, false, false, "")
	if !strings.Contains(got, "so a reply carries on there") {
		t.Errorf("a conversation with no way back says nothing about it:\n%s", got)
	}
	if strings.Contains(got, ">Reply<") {
		t.Errorf("Reply is offered where there is nowhere for it to go:\n%s", got)
	}
}
