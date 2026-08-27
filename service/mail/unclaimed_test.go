package mail

// Mail to agent@ from somebody with no account goes nowhere, silently.
//
// This used to be the opposite. agent@ opened an account for a stranger who
// wrote in — unclaimed, holding the conversation, with an allowance of turns —
// because the landing said "write to it and it answers" and for anybody without
// an account that was false.
//
// The reason it is gone is not the cost. A free front door is recovered from
// somewhere, and on most platforms the somewhere is the person who walked
// through it. Tools are sold and the agent comes with an account, so the way in
// is signing up.
//
// Two properties, and the second is the older one: nothing opens an account off
// the back of an inbound message, and a sender who is turned away is not told
// they were, because a bounce confirms the address is live to whoever is
// probing it.
//
// Read from the source rather than driven through an SMTP session, because what
// is held is the shape of the gate and that is a property of the code.

import (
	"os"
	"strings"
	"testing"
)

func TestNoInboundMailOpensAnAccount(t *testing.T) {
	src := smtpSource(t)

	if strings.Contains(src, "auth.Unclaimed(") {
		t.Error("inbound mail opens an account for a sender who has none — that is " +
			"a free front door, billed to the operator, for anybody who can send " +
			"an email")
	}
}

// And the sender is not told. The drop has to be a log line and a continue,
// with nothing that would put a message back on the wire.
func TestARejectedSenderIsNotToldWhy(t *testing.T) {
	src := smtpSource(t)

	i := strings.Index(src, "who has no account")
	if i < 0 {
		t.Fatal("the branch that turns away a sender with no account has moved; " +
			"this test needs repointing")
	}
	window := src[i:min(i+400, len(src))]
	for _, leak := range []string{"SendOut(", "deliver(", "SendExternalEmail("} {
		if strings.Contains(window, leak) {
			t.Errorf("a rejected sender is answered with %s — that confirms the "+
				"address is live to whoever is probing it", leak)
		}
	}
}

func smtpSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("smtp.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
