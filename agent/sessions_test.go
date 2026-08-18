package agent

// One list of conversations, and it can be got rid of.
//
// The agent surface had four names for one thing — Chat, Threads, Runs,
// Connect — two of them listing the same conversations under different
// headings. There is one list now: the rail, everything on it, with the ones
// that happened elsewhere marked and read-only. And nothing could be deleted;
// the list only ever grew.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mu/inbox"
	"mu/internal/thread"
)

// One list of conversations, wherever they happened.
//
// There were two: the rail listed the ones from this page and a second page
// listed all of them under another heading, differing by a filter nobody asked
// for. A conversation from another client opens read-only, which is the honest
// difference and does not need a page of its own to say.
func TestTheRailListsEveryConversation(t *testing.T) {
	acc := fmt.Sprintf("rail-%d", time.Now().UnixNano())

	here := Opened(acc, thread.WebClient, "root", "", "")
	Said(acc, here, "hello", "", "")
	Answered(acc, here, "hi", "")

	byMail := Opened(acc, "mail", "<one@phone>", "", "")
	Said(acc, byMail, "What's happening", "", "someone@example.com")
	Answered(acc, byMail, "Not much.", "")

	rail := chatThreads(acc, "")
	if len(rail) != 2 {
		t.Fatalf("the rail lists %d conversations, want both: %+v", len(rail), rail)
	}

	// The one that did not happen here is marked as such, because it cannot be
	// continued from this page — replying happens where it arrived.
	var mail *thread.Thread
	for i, tt := range rail {
		if tt.Client != thread.WebClient {
			mail = &rail[i]
		}
	}
	if mail == nil {
		t.Fatal("the mail conversation is not in the list")
	}
	if got := inbox.ConversationView(acc, mail); !strings.Contains(got, "Email") ||
		!strings.Contains(got, "What&#39;s happening") {
		t.Errorf("a conversation from another client does not read back:\n%s", got)
	}
}

// Deleting a conversation takes the whole conversation.
//
// The rail lists conversations, so deleting the id it holds has to mean what the
// rail says it means — not one turn of a five-turn chat, leaving the rest in the
// list.
func TestDeletingAConversationTakesTheWholeThing(t *testing.T) {
	acc := fmt.Sprintf("rail-del-%d", time.Now().UnixNano())

	id := Opened(acc, thread.WebClient, "root-flow", "", "")
	Said(acc, id, "book me a table", "", "")
	Answered(acc, id, "which night?", "flow-1")
	Said(acc, id, "friday", "", "")
	Answered(acc, id, "done", "flow-2")

	if n := len(chatThreads(acc, "")); n != 1 {
		t.Fatalf("two turns made %d conversations, want 1", n)
	}

	thread.Delete(acc, id)

	if n := len(chatThreads(acc, "")); n != 0 {
		t.Errorf("%d conversations left in the rail after deleting the only one", n)
	}
	if got := thread.Messages(acc, id, 0); len(got) != 0 {
		t.Errorf("what was said survived deletion (%d messages)", len(got))
	}
}

// A conversation the chat kept before there was a record is adopted into it,
// rather than disappearing from the rail the day the rail changed where it
// reads from.
func TestOldConversationsAreAdoptedIntoTheRecord(t *testing.T) {
	acc := fmt.Sprintf("rail-adopt-%d", time.Now().UnixNano())

	first := Record(Recorded{Account: acc, Prompt: "book me a table", Answer: "which night?"})
	second := Record(Recorded{Account: acc, Parent: first, Prompt: "friday", Answer: "done"})
	if first == "" || second == "" {
		t.Fatal("nothing recorded")
	}
	if n := len(chatThreads(acc, "")); n != 0 {
		t.Fatalf("a workflow chain is already in the record (%d) — this test proves nothing", n)
	}

	// Opening it by any id in the chain, which is what old links hold.
	id := openThread(acc, second)
	if id == "" {
		t.Fatal("an old conversation could not be opened, so its history is gone")
	}

	msgs := thread.Messages(acc, id, 0)
	if len(msgs) != 4 {
		t.Fatalf("adopted %d messages, want both sides of both turns: %+v", len(msgs), msgs)
	}
	if msgs[0].Text != "book me a table" || msgs[3].Text != "done" {
		t.Errorf("the conversation was adopted out of order: %+v", msgs)
	}
	if n := len(chatThreads(acc, "")); n != 1 {
		t.Errorf("the rail lists %d conversations after adoption, want 1", n)
	}
	// Twice is once: opening it again must not duplicate it.
	if again := openThread(acc, first); again != id {
		t.Errorf("opening the same conversation twice adopted it twice: %q then %q", id, again)
	}
	if n := len(thread.Messages(acc, id, 0)); n != 4 {
		t.Errorf("re-opening doubled the conversation to %d messages", n)
	}
}

// History comes from what was said, not from how it was produced.
func TestHistoryIsReadFromTheRecord(t *testing.T) {
	acc := fmt.Sprintf("rail-hist-%d", time.Now().UnixNano())

	id := Opened(acc, thread.WebClient, "root", "", "")
	Said(acc, id, "one", "", "")
	Answered(acc, id, "first", "")
	Said(acc, id, "two", "", "")
	Answered(acc, id, "second", "")
	// The message being answered right now, written down before the run starts.
	Said(acc, id, "three", "", "")

	turns := pastTurns(acc, id, 6)
	if len(turns) != 2 {
		t.Fatalf("history has %d turns, want the two that were answered: %+v", len(turns), turns)
	}
	if turns[0].Prompt != "one" || turns[0].Answer != "first" {
		t.Errorf("first turn is %+v", turns[0])
	}
	if turns[1].Prompt != "two" || turns[1].Answer != "second" {
		t.Errorf("second turn is %+v", turns[1])
	}
}

// A conversation knows which agent it is with, so a surface that has one
// selected can show that agent's conversations rather than all of them or none.
func TestAConversationRemembersWhichAgentItIsWith(t *testing.T) {
	acc := fmt.Sprintf("rail-agent-%d", time.Now().UnixNano())

	mine := Opened(acc, thread.WebClient, "root-a", "", "agent-42")
	other := Opened(acc, thread.WebClient, "root-b", "", "")
	Said(acc, mine, "what are the markets doing", "", "")
	Said(acc, other, "hello", "", "")

	var withAgent, without int
	for _, th := range thread.List(acc, 0) {
		if th.Agent == "agent-42" {
			withAgent++
		} else {
			without++
		}
	}
	if withAgent != 1 || without != 1 {
		t.Fatalf("attributed %d conversations to the agent and %d to the default, want 1 each",
			withAgent, without)
	}

	// And the last agent to answer is the one it is with: writing to a
	// specialist after a week of writing to the default moves the conversation.
	if again := Opened(acc, thread.WebClient, "root-b", "", "agent-42"); again != other {
		t.Fatalf("the same key opened a second conversation: %q then %q", other, again)
	}
	if th := thread.Get(acc, other); th == nil || th.Agent != "agent-42" {
		t.Error("a conversation continued with a different agent still names the old one")
	}
}
