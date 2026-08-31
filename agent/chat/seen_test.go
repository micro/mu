package chat

// A message you watched arrive is not news.
//
// Reported as: talk in a room, get a reply, and an unread conversation turns up
// in the inbox for a message you had just seen appear on screen.
//
// The record is right — this happened, and it belongs in the account's threads.
// The inbox is a notification hub as well as a record, and those two jobs
// disagree here: what arrived is not news to somebody who was in the room.

import (
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/chat"
)

// inRoom opens the conversation a room is recorded as and leaves it unread,
// which is the state every reply puts it in.
func inRoom(t *testing.T, who, room string) *thread.Thread {
	t.Helper()
	auth.Create(&auth.Account{ID: who, Name: who}) //nolint:errcheck

	th := thread.Open(who, Client, room)
	if th == nil {
		t.Fatal("no thread")
	}
	thread.Add(thread.Message{Thread: th.ID, Account: who,
		Role: thread.RoleAgent, Text: "the answer you were watching for"})

	got := thread.Find(who, Client, room)
	if got == nil {
		t.Fatal("the room's conversation is not in the record")
	}
	if !thread.Unread(*got) {
		t.Fatal("a fresh reply is not unread, so this test proves nothing")
	}
	return got
}

// Nobody in the room: it stays unread, which is the whole point of an inbox.
//
// This is the half that must not break. Marking a message read for somebody who
// was not there loses it — they have no other way of being told — so the
// condition has to be a live connection rather than "have they used this server
// lately".
func TestAReplyNobodyWatchedStaysUnread(t *testing.T) {
	const who, room = "seenaway", "chat_seenaway"
	inRoom(t, who, room)

	// The account is present on the instance in the sense auth.OnlineUsers
	// means — reading their mail in another tab — and is not in this room.
	auth.UpdatePresence(who)
	if chat.Watching(room, who) {
		t.Fatal("nobody connected to this room and Watching says otherwise")
	}

	seen(room, who)

	got := thread.Find(who, Client, room)
	if got == nil || !thread.Unread(*got) {
		t.Error("a reply nobody was in the room for was marked read; being " +
			"online somewhere on the instance is not having seen it, and there " +
			"is nothing else that would tell them it arrived")
	}
}

// And the delivery paths mark it read when it did land in front of somebody.
//
// markSeen is what both call once they have established that, each in their own
// way: a websocket room asks Watching, and an XMPP delivery has been told by
// SayTo that a client was connected.
func TestAReplyThatLandedInFrontOfSomebodyIsRead(t *testing.T) {
	const who, room = "seenhere", "chat_seenhere"
	before := inRoom(t, who, room)

	// Stamped after the message, or "read" would be indistinguishable from the
	// zero time the conversation started with.
	time.Sleep(time.Millisecond)
	markSeen(who, room)

	got := thread.Find(who, Client, room)
	if got == nil {
		t.Fatal("the conversation went missing")
	}
	if thread.Unread(*got) {
		t.Errorf("a reply delivered to somebody watching is still unread "+
			"(updated %s, seen %s)", got.Updated, got.Seen)
	}
	if got.ID != before.ID {
		t.Error("a second conversation was made rather than the one marked")
	}
}

// It never touches somebody else's conversation.
//
// A room can hold several people and the others may well be away from it. Each
// has their own thread and their own unread state, and one person reading is
// not the others having read.
func TestReadingIsNotReadingForEverybody(t *testing.T) {
	const mine, other, room = "seenmine", "seenother", "chat_seenshared"
	inRoom(t, mine, room)
	inRoom(t, other, room)

	time.Sleep(time.Millisecond)
	markSeen(mine, room)

	theirs := thread.Find(other, Client, room)
	if theirs == nil || !thread.Unread(*theirs) {
		t.Error("one person reading in a room marked it read for somebody else")
	}
	// And an empty account or room does nothing rather than something arbitrary.
	markSeen("", room)
	markSeen(mine, "")
}
