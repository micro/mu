package thread

// The record has to be right, because everything downstream believes it.
//
// What an agent knows about somebody is what is in here. A conversation joined
// to the wrong one is a stranger's history handed to the model answering them,
// and a conversation lost is an agent that has forgotten a customer.

import (
	"strings"
	"testing"
	"time"
)

func reset(t *testing.T) {
	t.Helper()
	mu.Lock()
	threads = map[string]*Thread{}
	messages = map[string][]*Message{}
	owned = map[string]map[string]*Thread{}
	held = map[string]int{}
	mu.Unlock()
}

func TestAConversationIsFoundAgainByItsClientAndKey(t *testing.T) {
	reset(t)

	first := Open("asim", "discord", "channel-1")
	if first == nil {
		t.Fatal("Open returned nothing")
	}
	if again := Open("asim", "discord", "channel-1"); again.ID != first.ID {
		t.Error("the same conversation opened twice produced two threads, so every " +
			"message would start a new one and nothing would ever have history")
	}
	if other := Open("asim", "discord", "channel-2"); other.ID == first.ID {
		t.Error("two channels resolved to one conversation")
	}
	// A key is a string somebody else's service chose. The same one on another
	// client is not the same conversation.
	if other := Open("asim", "whatsapp", "channel-1"); other.ID == first.ID {
		t.Error("the same key on another client joined the same conversation — " +
			"clients do not share an id space")
	}
}

// A conversation belongs to one account and cannot be reached from another.
func TestAConversationIsScopedToItsAccount(t *testing.T) {
	reset(t)

	mine := Open("asim", "whatsapp", "+447700900000")
	Add(Message{Thread: mine.ID, Account: "asim", Text: "confidential"})

	// A phone number is not a secret, so the same key from another account must
	// start a conversation of its own rather than join this one.
	yours := Open("someone-else", "whatsapp", "+447700900000")
	if yours.ID == mine.ID {
		t.Fatal("another account joined my conversation by knowing my phone number")
	}
	if got := Messages("someone-else", mine.ID, 0); got != nil {
		t.Errorf("another account read %d of my messages", len(got))
	}
	if Get("someone-else", mine.ID) != nil {
		t.Error("another account fetched my conversation by id")
	}
	if Add(Message{Thread: mine.ID, Account: "someone-else", Text: "hello"}) != "" {
		t.Error("another account wrote into my conversation")
	}
}

func TestMessagesComeBackInOrderAndBounded(t *testing.T) {
	reset(t)

	th := Open("asim", "web", "session-1")
	base := time.Now().UTC().Add(-time.Hour)
	for i, text := range []string{"one", "two", "three", "four"} {
		Add(Message{Thread: th.ID, Account: "asim", Text: text,
			At: base.Add(time.Duration(i) * time.Minute)})
	}

	all := Messages("asim", th.ID, 0)
	if len(all) != 4 || all[0].Text != "one" || all[3].Text != "four" {
		t.Fatalf("messages came back as %v, want oldest first", texts(all))
	}
	// A limit keeps the most recent, still in order — the recent turns are the
	// ones being answered.
	last := Messages("asim", th.ID, 2)
	if len(last) != 2 || last[0].Text != "three" || last[1].Text != "four" {
		t.Errorf("the last two are %v, want [three four]", texts(last))
	}
}

// A reply finds its conversation by the id the client gave it.
//
// Mail's case: answering something from last week is answering *that*, not
// whatever has happened on the thread since.
func TestAReplyFindsItsConversationByReference(t *testing.T) {
	reset(t)

	th := Open("asim", "mail", "<root@example.com>")
	Add(Message{Thread: th.ID, Account: "asim", Text: "what are your rates?",
		Ref: "<in-1@example.com>"})
	Add(Message{Thread: th.ID, Account: "asim", Role: RoleAgent, Text: "£500 a day.",
		Ref: "<out-1@micro.mu>"})

	// Whichever id the client quotes, and however many.
	for _, refs := range [][]string{
		{"<out-1@micro.mu>"},
		{"<in-1@example.com>"},
		{"", "<other@elsewhere.com> <out-1@micro.mu>"},
	} {
		if got := ByRef("asim", refs...); got == nil || got.ID != th.ID {
			t.Errorf("ByRef(%v) did not find the conversation", refs)
		}
	}
	if got := ByRef("asim", "<never-seen@example.com>"); got != nil {
		t.Error("a reply to something we never sent found a conversation anyway")
	}
	if got := ByRef("asim"); got != nil {
		t.Error("no references at all found a conversation")
	}
	// And not from another account, however the id was come by.
	if got := ByRef("someone-else", "<out-1@micro.mu>"); got != nil {
		t.Fatal("quoting a message id let another account into this conversation, " +
			"and its whole history with it")
	}
}

func TestARecordedMessageKeepsWhatItWasToldAndFillsTheRest(t *testing.T) {
	reset(t)

	th := Open("asim", "mail", "<root@x.com>")
	id := Add(Message{Thread: th.ID, Account: "asim", Text: "  book me a table  ",
		Ref: "<m1@x.com>", From: "asim@aslam.me"})
	if id == "" {
		t.Fatal("Add recorded nothing")
	}

	got := Messages("asim", th.ID, 0)[0]
	if got.Role != RolePerson {
		t.Errorf("role defaulted to %q, want %q", got.Role, RolePerson)
	}
	if got.At.IsZero() {
		t.Error("no timestamp")
	}
	if got.Ref != "<m1@x.com>" || got.From != "asim@aslam.me" {
		t.Error("what the client supplied was not kept")
	}
	// The conversation takes its name from the first thing said, so a list of
	// them reads as something rather than as ids.
	if s := Get("asim", th.ID).Subject; !strings.Contains(s, "book me a table") {
		t.Errorf("subject is %q, want the first message", s)
	}

	// Nothing to record is not a record.
	if Add(Message{Thread: th.ID, Account: "asim", Text: "   "}) != "" {
		t.Error("an empty message was recorded")
	}
	if Add(Message{Thread: "no-such-thread", Account: "asim", Text: "hi"}) != "" {
		t.Error("a message was recorded against a conversation that does not exist")
	}
}

func TestConversationsListMostRecentlyActiveFirst(t *testing.T) {
	reset(t)

	old := Open("asim", "web", "old")
	Add(Message{Thread: old.ID, Account: "asim", Text: "ages ago",
		At: time.Now().UTC().Add(-24 * time.Hour)})
	recent := Open("asim", "mail", "<new@x.com>")
	Add(Message{Thread: recent.ID, Account: "asim", Text: "just now"})

	list := List("asim", 0)
	if len(list) != 2 {
		t.Fatalf("listed %d conversations, want 2", len(list))
	}
	if list[0].ID != recent.ID {
		t.Error("the least recently active conversation is listed first")
	}
	if got := List("someone-else", 0); len(got) != 0 {
		t.Errorf("another account listed %d of my conversations", len(got))
	}
}

func texts(m []Message) []string {
	var out []string
	for _, x := range m {
		out = append(out, x.Text)
	}
	return out
}
