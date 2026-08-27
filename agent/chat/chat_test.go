package chat

import (
	"os"
	"strings"
	"testing"
)

// The chat service does not compose replies any more.
//
// It did: a hundred and ninety lines inside a websocket goroutine, with its own
// RAG over the index, its own decision about whether to search the web, its own
// history rebuilt from the room's last twenty messages, and a model call. A
// hand-rolled agent inside a service.
//
// The cost was not tidiness. That agent could reach two sources; the agent
// everywhere else reaches every tool the instance has, keeps the conversation
// in the record, is metered, and remembers across clients. A room was the one
// place you talked to something worse and nothing said so.
func TestTheChatServiceNoLongerComposesReplies(t *testing.T) {
	src := read(t, "../../service/chat/chat.go")

	// The tell is a model call on the message path. askLLM survives for the
	// topic summaries and the tagging work this package does not own yet —
	// tracked in #1469 — so the check is on what disappeared with the reply.
	for _, gone := range []string{
		"ai.Prompt{\n\t\t\t\t\t\t\tTopic:", // the room reply prompt
		"RAG: found %d results",
		"[WebSearch] RAG thin",
		"Follow-up: searching for",
	} {
		if strings.Contains(src, gone) {
			t.Errorf("service/chat still composes replies: found %q", gone)
		}
	}

	// And it says what happened instead.
	if !strings.Contains(src, "event.RequestChatReply(") {
		t.Error("service/chat does not announce that somebody spoke, so nothing " +
			"can answer")
	}
}

// The gate stays in the service.
//
// Who is in a room and whether the agent was named are facts about the room.
// Publishing every message and letting the subscriber decide would be the
// mistake event.MailForAgent's comment describes: a guarantee turned into a
// convention, failing open on the path where somebody else's message drives an
// agent.
func TestTheServiceStillDecidesWhoAnswers(t *testing.T) {
	src := read(t, "../../service/chat/chat.go")
	for _, want := range []string{"mentionedMicro", "inActiveConvo", "isAlone", "isItemRoom"} {
		if !strings.Contains(src, want) {
			t.Errorf("the gate lost %s — the service is the only thing that knows "+
				"who is in the room", want)
		}
	}
	// And the publish is inside it, not before it.
	gate := strings.Index(src, "if mentionedMicro || inActiveConvo")
	pub := strings.Index(src, "event.RequestChatReply(")
	if gate < 0 || pub < 0 || pub < gate {
		t.Error("the announcement is not inside the gate, so a message that failed " +
			"it would still wake an agent")
	}
}

// A deterministic lookup does not become a model call.
//
// Answering "what is the btc price" from a table is a service answering a
// question about state, which is this package's job and cheaper. Routing it
// through an agent would be the opposite mistake to the one being fixed.
func TestAPatternMatchStillSkipsTheAgent(t *testing.T) {
	src := read(t, "../../service/chat/chat.go")
	match := strings.Index(src, "handlePatternMatch(content, room)")
	pub := strings.Index(src, "event.RequestChatReply(")
	if match < 0 {
		t.Fatal("pattern matching is gone, so every price question is now a model call")
	}
	if pub < match {
		t.Error("the announcement comes before the pattern match, so a lookup that " +
			"needs no model wakes an agent anyway")
	}
}

// This package hands the question to the real agent rather than rebuilding one.
func TestTheAgentIsTheRealOne(t *testing.T) {
	src := read(t, "chat.go")
	if !strings.Contains(src, "agent.Ask(agent.AskRequest{") {
		t.Error("agent/chat does not call agent.Ask, so it is a second agent rather " +
			"than the one every other client uses")
	}
	// A room is not a DM: several people can be in one, so the owner's mail,
	// wallet and notes are out of scope.
	if !strings.Contains(src, "Public: true") {
		t.Error("a room reply is not marked public, so private context reaches a " +
			"conversation other people can read")
	}
	// The room is the conversation, so it is the thread key — which is what
	// makes a second message a second turn.
	if !strings.Contains(src, "Thread: s.Room") {
		t.Error("the room is not the thread, so every message starts a new " +
			"conversation in the record")
	}
	// None of the old machinery comes back here.
	for _, gone := range []string{"data.Search", "ai.WebSearch", "ai.ShouldWebSearch", "ai.Ask"} {
		if strings.Contains(src, gone) {
			t.Errorf("agent/chat reimplements %s — agent.Ask already has it as a tool", gone)
		}
	}
}

// A room that has gone is not an error worth failing on, and never a room that
// gets created to receive an answer nobody is waiting for.
func TestAnAnswerToAnEmptyRoomIsDropped(t *testing.T) {
	src := read(t, "../../service/chat/chat.go")
	say := strings.Index(src, "func Say(")
	if say < 0 {
		t.Fatal("no Say")
	}
	body := src[say:]
	if end := strings.Index(body, "\nfunc "); end > 0 {
		body = body[:end]
	}
	if strings.Contains(body, "getOrCreateRoom") {
		t.Error("Say creates the room it is answering into, so a reply to a " +
			"conversation nobody is in conjures the conversation")
	}
	if !strings.Contains(body, "default:") {
		t.Error("Say blocks when a room has stopped reading, which stalls the " +
			"whole subscriber rather than one message")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
