package agent

// The surround is shared, and that is the point.
//
// Every client reached the same agent already. What each one wrote itself was
// the part around it — history, the run record, memory — and three of them kept
// history in a map in memory, keyed differently, lost on restart, invisible
// anywhere else. These pin the pieces that can be checked without a model.

import (
	"os"
	"strings"
	"testing"
	"time"
)

func turnVia(t *testing.T, account, client, thread, parent, prompt, answer string) string {
	t.Helper()
	id := Record(Recorded{
		Account: account, Source: client, Prompt: prompt, Answer: answer,
		Parent: parent, Via: Via{Client: client, Thread: thread},
		Started: time.Now(),
	})
	if id == "" {
		t.Fatal("Record returned no id")
	}
	return id
}

// A second message on the same thread continues the first.
func TestAThreadIsFoundByItsClientAndId(t *testing.T) {
	const acc = "ask-thread"
	first := turnVia(t, acc, "discord", "channel-1", "", "hello", "hi")

	if got := ThreadHead(acc, "discord", "channel-1"); got != first {
		t.Errorf("the conversation head is %q, want %q", got, first)
	}
	// A different channel is a different conversation, and so is the same id on
	// another client — a Discord channel and a phone number could collide.
	if got := ThreadHead(acc, "discord", "channel-2"); got != "" {
		t.Errorf("another channel resolved to %q", got)
	}
	if got := ThreadHead(acc, "whatsapp", "channel-1"); got != "" {
		t.Errorf("the same thread id on another client resolved to %q — clients do "+
			"not share an id space and must not share conversations", got)
	}
}

// A thread id is a string somebody else's service chose, so it is scoped.
func TestAThreadCannotBeReachedFromAnotherAccount(t *testing.T) {
	const mine, yours = "ask-mine", "ask-yours"
	turnVia(t, mine, "whatsapp", "+447700900000", "", "private", "confidential")

	if got := ThreadHead(yours, "whatsapp", "+447700900000"); got != "" {
		t.Fatalf("another account reached my conversation (%q) — a phone number is "+
			"not a secret and this would hand over its history", got)
	}
}

// The newest turn is the head, so a conversation grows at its end.
func TestTheHeadOfAThreadIsItsNewestTurn(t *testing.T) {
	const acc = "ask-head"
	first := turnVia(t, acc, "telegram", "chat-9", "", "one", "1")
	updateFlow(first, func(f *Flow) { f.CreatedAt = time.Now().Add(-time.Hour) })
	second := turnVia(t, acc, "telegram", "chat-9", first, "two", "2")

	if got := ThreadHead(acc, "telegram", "chat-9"); got != second {
		t.Errorf("head is %q, want the newest turn %q — otherwise each message hangs "+
			"off the middle and the conversation fans out", got, second)
	}
	if h := ThreadHistory(acc, second, 10); len(h) != 4 {
		t.Fatalf("history has %d messages, want both sides of both turns", len(h))
	}
}

// No client keeps a conversation of its own any more.
//
// This is the whole change, and it is the kind that half-happens: one client
// gets migrated and two keep their maps, which is worse than before because now
// there are two answers to where history lives.
func TestNoClientKeepsItsOwnHistory(t *testing.T) {
	for _, c := range []struct{ path, name string }{
		{"../client/discord/discord.go", "discord"},
		{"../client/telegram/telegram.go", "telegram"},
		{"../client/whatsapp/whatsapp.go", "whatsapp"},
	} {
		b, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, gone := range []string{"histories", "getHistory", "addHistory"} {
			if strings.Contains(src, gone) {
				t.Errorf("%s still has %s — a second copy of history that restarts "+
					"empty and nothing else can read", c.name, gone)
			}
		}
		if !strings.Contains(src, "agent.Ask(") {
			t.Errorf("%s does not go through agent.Ask, so it has its own surround", c.name)
		}
	}
}

// Every client teaches memory, not just the web.
func TestEveryClientTeachesMemory(t *testing.T) {
	b, err := os.ReadFile("ask.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "extractMemory") {
		t.Error("Ask does not extract memory, so an agent still only learns from " +
			"whichever client happens to call it — which was the web, and nowhere else")
	}
}

// A client that resolves its own parent still gets one conversation.
//
// Mail finds the turn a message answers from its headers, which is better than
// "the last thing on this thread" — a reply to something from a week ago is a
// reply to that, not to whatever came since. So Ask takes the parent when the
// client has one, and only looks it up when it does not.
func TestAResolvedParentWinsOverTheThreadHead(t *testing.T) {
	const acc = "ask-parent"
	old := turnVia(t, acc, "mail", "<root@x.com>", "", "first", "1")
	updateFlow(old, func(f *Flow) { f.CreatedAt = time.Now().Add(-time.Hour) })
	newer := turnVia(t, acc, "mail", "<root@x.com>", old, "second", "2")

	if head := ThreadHead(acc, "mail", "<root@x.com>"); head != newer {
		t.Fatalf("head is %q, want %q", head, newer)
	}
	// The thread of any turn in the chain is the thread the first one set.
	if got := ThreadOf(acc, old); got != "<root@x.com>" {
		t.Errorf("ThreadOf = %q, want the thread the conversation started under", got)
	}
	if got := ThreadOf(acc, ""); got != "" {
		t.Errorf("ThreadOf of nothing is %q", got)
	}
}

// A thread key identifies a conversation, not a room.
//
// Keying on the channel alone made everything ever said in a shared channel one
// conversation, and handed each person the others' history as context. That is
// somebody else's mail, arriving as if the agent already knew them.
func TestAThreadKeySeparatesPeopleInASharedRoom(t *testing.T) {
	for _, c := range []struct{ path, name string }{
		{"../client/discord/discord.go", "discord"},
		{"../client/telegram/telegram.go", "telegram"},
	} {
		b, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "func threadKey(") {
			t.Errorf("%s has no threadKey, so a shared channel is one conversation "+
				"and everyone in it shares a history", c.name)
		}
	}
}
