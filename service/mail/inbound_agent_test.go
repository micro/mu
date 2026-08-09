package mail

// The guards on waking an agent from inbound mail.
//
// The hook itself needs the agent and the roster, so it is wired in main.go.
// What lives here is the decision to call it at all, and the three cases where
// calling it would be wrong: spam, our own outbound, and a plus address that
// nobody claimed. Each of those is a loop or a bill if it gets through — an
// agent replying to its own reply costs a model call per turn, forever.

import (
	"strings"
	"testing"
)

// shouldWake mirrors the condition in handleData. Kept as a function so the
// rule can be read and tested without standing up an SMTP session.
func shouldWake(hooked bool, tag, from string, isSpam bool, domain string) bool {
	return hooked && tag != "" && !isSpam &&
		!strings.EqualFold(from, "agent@"+domain)
}

func TestWhenInboundMailWakesAnAgent(t *testing.T) {
	const domain = "micro.mu"

	cases := []struct {
		name   string
		hooked bool
		tag    string
		from   string
		spam   bool
		want   bool
	}{
		{"mail to an agent", true, "research", "someone@example.com", false, true},
		{"no tag at all", true, "", "someone@example.com", false, false},
		{"flagged as spam", true, "research", "someone@example.com", true, false},
		{"our own outbound, which would loop", true, "research", "agent@micro.mu", false, false},
		{"our own outbound, cased differently", true, "research", "Agent@Micro.MU", false, false},
		{"no agent configured on this instance", false, "research", "someone@example.com", false, false},
	}
	for _, c := range cases {
		if got := shouldWake(c.hooked, c.tag, c.from, c.spam, domain); got != c.want {
			t.Errorf("%s: woke=%v, want %v", c.name, got, c.want)
		}
	}
}

// The source has to keep matching the rule above, since the rule is the thing
// standing between one reply and an unbounded exchange with ourselves.
func TestTheLoopGuardIsStillInThePath(t *testing.T) {
	src := readSource(t, "smtp.go")
	for _, want := range []string{
		"InboundAgent != nil",
		"!spamResult.IsSpam",
		`!strings.EqualFold(fromAddr.Address, "agent@"+GetConfiguredDomain())`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the inbound-agent guard lost %q — an agent can now answer its own reply", want)
		}
	}
	// And it runs after the message is stored, so a failure to answer never
	// loses the mail.
	store := strings.Index(src, "if err := SendMessageTo(")
	wake := strings.Index(src, "if InboundAgent != nil")
	if store < 0 || wake < 0 || wake < store {
		t.Error("the agent is woken before the message is stored, so a panic there loses the mail")
	}
}
