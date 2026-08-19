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
	cc := []string{"asim@example.com", "BROTHER@example.net", "asim+research@mu.example"}

	got := Others(to, cc, "asim@example.com")
	want := []string{"brother@example.net"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v — the sender is the To of the reply, our own "+
			"addresses are a loop, and a duplicate is a person copied twice", got, want)
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
