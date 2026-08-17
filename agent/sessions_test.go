package agent

// The rail is the chat's own history, and a conversation can be got rid of.
//
// Every client records a run, so once mail could wake an agent the chat rail
// started listing emails: rows titled with whatever markup somebody's phone
// produced, opening a conversation that never happened on this page. And
// nothing could be deleted — the rail only ever grew.

import (
	"fmt"
	"testing"
	"time"

	"mu/internal/thread"
)

// A run that arrived some other way is not a chat session.
func TestTheRailListsWhatWasSaidHere(t *testing.T) {
	acc := fmt.Sprintf("rail-%d", time.Now().UnixNano())

	here := Record(Recorded{Account: acc, Prompt: "hello", Answer: "hi"})
	byMail := Record(Recorded{Account: acc, Source: "mail",
		Trigger: "email from someone@example.com",
		Prompt:  `<div dir="auto">What&#39;s happening </div>`, Answer: "Not much."})
	if here == "" || byMail == "" {
		t.Fatal("nothing recorded")
	}

	sessions := ListSessions(acc)
	if len(sessions) != 1 {
		t.Fatalf("the rail lists %d conversations, want only the one started here: %+v",
			len(sessions), sessions)
	}
	if sessions[0].RootID != here {
		t.Error("the rail lists a run that did not happen on this page — an email answered " +
			"at 6am became a row you cannot continue, titled with the sender's markup")
	}

	// Runs still shows everything: that is the difference between the two pages.
	if len(ListFlows(acc)) != 2 {
		t.Error("filtering the rail dropped a run from the run log")
	}
}

// Deleting a conversation deletes the conversation, not one turn of it.
func TestDeletingAConversationTakesTheWholeThing(t *testing.T) {
	acc := fmt.Sprintf("rail-del-%d", time.Now().UnixNano())

	first := Record(Recorded{Account: acc, Prompt: "book me a table", Answer: "which night?"})
	second := Record(Recorded{Account: acc, Parent: first, Prompt: "friday", Answer: "done"})
	if first == "" || second == "" {
		t.Fatal("nothing recorded")
	}
	if n := len(ListSessions(acc)); n != 1 {
		t.Fatalf("two turns made %d conversations, want 1", n)
	}

	// The record of what was said, keyed the way the web keys it: on the root.
	id := Opened(acc, WebClient, first, "", "")
	Said(acc, id, "book me a table", "", "")
	Answered(acc, id, "which night?", first)

	deleteSession(acc, first)

	if n := len(ListSessions(acc)); n != 0 {
		t.Errorf("%d conversations left after deleting the only one — deleting the id the "+
			"rail holds removed one turn and left the rest in the list", n)
	}
	if n := len(ListFlows(acc)); n != 0 {
		t.Errorf("%d turns survived, so a deleted conversation is still on Runs", n)
	}
	if got := thread.Messages(acc, id, 0); len(got) != 0 {
		t.Errorf("what was said survived deletion (%d messages) — the conversation is gone "+
			"from every page and the record still has it", len(got))
	}
}

// A conversation knows which agent it is with, so a surface that has one
// selected can show that agent's conversations rather than all of them or none.
func TestAConversationRemembersWhichAgentItIsWith(t *testing.T) {
	acc := fmt.Sprintf("rail-agent-%d", time.Now().UnixNano())

	mine := Opened(acc, WebClient, "root-a", "", "agent-42")
	other := Opened(acc, WebClient, "root-b", "", "")
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
	if again := Opened(acc, WebClient, "root-b", "", "agent-42"); again != other {
		t.Fatalf("the same key opened a second conversation: %q then %q", other, again)
	}
	if th := thread.Get(acc, other); th == nil || th.Agent != "agent-42" {
		t.Error("a conversation continued with a different agent still names the old one")
	}
}
