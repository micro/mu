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

// The rail is what you started; the inbox is what arrived.
//
// These were one list twice: the rail showed every conversation on the account
// and so does /inbox, so the two pages differed only in furniture and neither
// could be described in a sentence. The line is the one every mail product
// draws — see thread.Arrived.
func TestTheRailIsWhatYouStartedNotWhatArrived(t *testing.T) {
	acc := fmt.Sprintf("rail_%d", time.Now().UnixNano()%100000000)

	here := Opened(acc, thread.WebClient, "root", "", "")
	Said(acc, here, "hello", "", "")
	Answered(acc, here, "hi", "")

	byMail := Opened(acc, "mail", "<one@phone>", "", "")
	Said(acc, byMail, "What's happening", "", "someone@example.com")
	Answered(acc, byMail, "Not much.", "")

	rail := chatThreads(acc, "", false)
	if len(rail) != 1 {
		t.Fatalf("the rail lists %d conversations, want only the one started here: %+v", len(rail), rail)
	}
	if rail[0].Client != thread.WebClient {
		t.Errorf("the rail is showing a conversation from %q", rail[0].Client)
	}

	// And the one that arrived still reads back, on the page that owns it.
	// Read-only there, because replying happens where it arrived.
	mail := thread.Get(acc, byMail)
	if mail == nil {
		t.Fatal("the mail conversation is not in the record")
	}
	if got := inbox.ConversationView(acc, mail); !strings.Contains(got, "Mail") ||
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
	acc := fmt.Sprintf("rail_del_%d", time.Now().UnixNano()%100000000)

	id := Opened(acc, thread.WebClient, "root_flow", "", "")
	Said(acc, id, "book me a table", "", "")
	Answered(acc, id, "which night?", "flow_1")
	Said(acc, id, "friday", "", "")
	Answered(acc, id, "done", "flow_2")

	if n := len(chatThreads(acc, "", false)); n != 1 {
		t.Fatalf("two turns made %d conversations, want 1", n)
	}

	thread.Delete(acc, id)

	if n := len(chatThreads(acc, "", false)); n != 0 {
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
	acc := fmt.Sprintf("rail_adopt_%d", time.Now().UnixNano()%100000000)

	first := Record(Recorded{Account: acc, Prompt: "book me a table", Answer: "which night?"})
	second := Record(Recorded{Account: acc, Parent: first, Prompt: "friday", Answer: "done"})
	if first == "" || second == "" {
		t.Fatal("nothing recorded")
	}
	if n := len(chatThreads(acc, "", false)); n != 0 {
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
	if n := len(chatThreads(acc, "", false)); n != 1 {
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
	acc := fmt.Sprintf("rail_hist_%d", time.Now().UnixNano()%100000000)

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
	acc := fmt.Sprintf("rail_agent_%d", time.Now().UnixNano()%100000000)

	mine := Opened(acc, thread.WebClient, "root_a", "", "agent_42")
	other := Opened(acc, thread.WebClient, "root_b", "", "")
	Said(acc, mine, "what are the markets doing", "", "")
	Said(acc, other, "hello", "", "")

	var withAgent, without int
	for _, th := range thread.List(acc, 0) {
		if th.Agent == "agent_42" {
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
	if again := Opened(acc, thread.WebClient, "root_b", "", "agent_42"); again != other {
		t.Fatalf("the same key opened a second conversation: %q then %q", other, again)
	}
	if th := thread.Get(acc, other); th == nil || th.Agent != "agent_42" {
		t.Error("a conversation continued with a different agent still names the old one")
	}
}

// A page about one agent shows that agent's conversations — including Micro's.
//
// The filter was `agentID != ""`, and the default agent's id *is* the empty
// string. So /agent/micro, which is one agent's page, filtered nothing and
// listed every conversation on the account: opening a chat with Micro showed
// somebody else's history first, and clicking it switched the agent, because a
// reopened conversation carries its own.
//
// Same shape as the scope fail-open in filterServices. Empty meant both "no
// restriction" and "the default", and those are different statements.
func TestOneAgentsPageShowsOneAgentsConversations(t *testing.T) {
	const acc = "rail_scoped"

	mine := Opened(acc, thread.WebClient, "rail_micro", "", "")
	theirs := Opened(acc, thread.WebClient, "rail_foobar", "", "foobar")
	if mine == "" || theirs == "" {
		t.Fatal("no conversations")
	}

	// Micro's page: Micro's conversations, and not Foobar's.
	onMicro := chatThreads(acc, "", true)
	for _, got := range onMicro {
		if got.Agent != "" {
			t.Errorf("Micro's page lists a conversation with %q", got.Agent)
		}
	}
	if len(onMicro) == 0 {
		t.Error("Micro's page lists none of its own conversations")
	}

	// Foobar's page: Foobar's.
	onFoobar := chatThreads(acc, "foobar", true)
	for _, got := range onFoobar {
		if got.Agent != "foobar" {
			t.Errorf("Foobar's page lists a conversation with %q", got.Agent)
		}
	}
	if len(onFoobar) == 0 {
		t.Error("Foobar's page lists none of its own conversations")
	}

	// And a bare /agent, which is about no agent in particular, lists them all.
	// "No restriction" is still a thing somebody can mean.
	if all := chatThreads(acc, "", false); len(all) < 2 {
		t.Errorf("a page about no agent lists %d conversations, want all of them", len(all))
	}
}
