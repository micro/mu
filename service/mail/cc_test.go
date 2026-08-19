package mail

// Being copied into somebody else's conversation.

import (
	"strings"
	"testing"
)

func TestRecipientsReadsAHeader(t *testing.T) {
	got := Recipients(`Asim <asim@example.com>, brother@example.net, "Someone, Jr." <s@example.org>`)
	want := []string{"asim@example.com", "brother@example.net", "s@example.org"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
	// A header with one bad entry still yields the good ones. Dropping the whole
	// line because a client wrote something odd loses everybody on it.
	if got := Recipients(`good@example.com, <<<broken`); len(got) == 0 {
		t.Error("one malformed address lost the whole recipient list")
	}
	if got := Recipients("  "); got != nil {
		t.Errorf("an empty header produced %v", got)
	}
}

// Others is everybody who will read the reply: not the sender, not us.
func TestOthersLeavesOutTheSenderAndUs(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")

	to := []string{"brother@example.net", "agent@mu.example"}
	cc := []string{"asim@example.com", "BROTHER@example.net"}

	got := Others(to, cc, "asim@example.com", "agent@mu.example")
	want := []string{"brother@example.net"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v — the sender is the To of the reply, the address "+
			"we answer as is ourselves, and a duplicate is a person copied twice",
			got, want)
	}
}

// A second agent on the thread stays on the reply.
//
// The first version of Others stripped every address at the mail domain, which
// killed two things at once. Copy agent+news@ and agent+markets@ into one mail
// and neither could see the other, so a thread with two specialists on it was
// two private conversations. And a Mu user is a person with an address at the
// mail domain, so a thread between two accounts here had one of them silently
// dropped from the reply — the loop guard eating a participant.
//
// It is safe to leave them because the guard that matters is on the sender, not
// the recipient: mayDispatch refuses anything written by one of this instance's
// agent addresses, so an agent never wakes on another agent's answer. The human
// is always what triggers a run.
func TestASecondAgentStaysOnTheThread(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")

	got := Others(
		[]string{"brother@example.net"},
		[]string{"agent+news@mu.example", "agent+markets@mu.example", "someone@mu.example"},
		"asim@example.com", "agent+news@mu.example")

	want := []string{"brother@example.net", "agent+markets@mu.example", "someone@mu.example"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v — the other agent and the Mu user were dropped "+
			"from the reply, so nobody on this thread can see each other", got, want)
	}
}

// And an agent's own answer never wakes another agent.
//
// This is the guard that makes the above safe, and the failure it prevents is
// the worst one in this design: two agents replying to each other with no human
// in the loop, at a model call each, until somebody notices the bill.
func TestAnAgentNeverWakesOnAnotherAgent(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")
	KnownSender = func(string, string) bool { return true }
	defer func() { KnownSender = nil }()
	// mayDispatch declines to reason about who may wake what on an instance
	// where nothing is listening, so there has to be something listening.
	Inbound(AgentMailbox, func(InboundMail) {})

	from := wakeRequest{
		Owner: "someone", Shared: true, Tag: "markets",
		From: "agent+news@mu.example", To: "agent+markets@mu.example",
		Authenticated: true,
	}
	if mayDispatch(from) {
		t.Fatal("agent+markets@ would answer agent+news@ — two agents replying to " +
			"each other forever, at a model call each, with no human in the loop")
	}
	// The same message from a person does wake it, so the guard is not simply
	// refusing everything.
	human := from
	human.From = "asim@example.com"
	if !mayDispatch(human) {
		t.Error("a person on the thread cannot wake the agent either, so this guard " +
			"is refusing everything rather than refusing agents")
	}
}

// An address on this instance is never copied. An agent that copies itself
// answers its own answer, forever, at a model call a turn.
func TestWeAreNeverOnOurOwnReply(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")
	for _, addr := range []string{"agent@mu.example", "agent+news@mu.example", "asim@mu.example"} {
		if !Ours(addr) {
			t.Errorf("%s is not recognised as ours, so a reply-all would copy it "+
				"and wake the thing that just spoke", addr)
		}
	}
	if Ours("someone@example.net") {
		t.Error("an outside address was treated as ours, so they would be dropped from the reply")
	}
}

// When it speaks. This is the rule that decides whether the feature is usable:
// an agent that answers every message of a conversation between two other
// people is noise a turn and a model call a turn.
func TestItSpeaksWhenSpokenTo(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")
	others := []string{"brother@example.net"}

	cases := []struct {
		what    string
		toAgent bool
		others  []string
		spoken  bool
		body    string
		want    bool
	}{
		{"a plain message to the agent", true, nil, false, "what is the weather", true},
		{"a plain message, having spoken before", false, nil, true, "and tomorrow?", true},
		{"just copied in", false, others, false, "when does the train leave?", true},
		{"put in To on a group thread", true, others, true, "check that for me", true},
		{"two other people talking", false, others, true, "ok see you at 6", false},
		{"named by address", false, others, true, "agent@mu.example what time is it", true},
		{"named with an @", false, others, true, "@agent can you check", true},
		{"named as a word", false, others, true, "agent, what time is that train", true},
	}
	for _, c := range cases {
		if got := Addressed(c.toAgent, c.others, c.spoken, c.body); got != c.want {
			t.Errorf("%s: Addressed = %v, want %v", c.what, got, c.want)
		}
	}
}

// The mention check does not fire on a word that merely contains it.
func TestNamedDoesNotFireOnPartOfAWord(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "mu.example")
	for _, body := range []string{
		"I spoke to the estate agents yesterday",
		"management said no",
		"that was a strange agenda",
	} {
		if Named(body) {
			t.Errorf("%q was read as addressing the agent — it would interrupt a "+
				"private conversation, which is the expensive mistake here", body)
		}
	}
	// And it does fire on the real thing.
	if !Named("agent, can you check the times") {
		t.Error("being addressed by name was missed, so somebody has to write again")
	}
}
