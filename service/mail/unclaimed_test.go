package mail

// The front door is open, and only to senders who are who they say they are.
//
// agent@ used to drop mail from anybody without an account — silently, so a
// probe could not learn the address was live. The landing page meanwhile said
// "write to it and it answers", which for everybody without an account was
// false. It answers now, into an unclaimed account.
//
// The thing that must not slip: an address in a From header is whatever the
// sending machine typed. "Ten free exchanges per address" keyed on an
// unauthenticated From is an open model-call endpoint, billed to the operator,
// and the bill is the first anybody hears of it. SPF or DKIM has to pass.
//
// Read from the source rather than driven through an SMTP session, because what
// is being held is the shape of the gate — no unauthenticated sender reaches
// auth.Unclaimed — and that is a property of the code.

import (
	"os"
	"strings"
	"testing"
)

func TestNoUnauthenticatedSenderGetsAnAccount(t *testing.T) {
	b, err := os.ReadFile("smtp.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	i := strings.Index(src, "auth.Unclaimed(")
	if i < 0 {
		t.Fatal("nothing opens an account for a new sender any more; this test needs repointing")
	}

	// The authentication check has to come before it, in the same branch. Both
	// signals are already computed on this path — dkimPass at the top of the
	// message, s.spfPass at MAIL FROM — and combined for the wake request as
	// `dkimPass || s.spfPass`.
	before := src[:i]
	guard := strings.LastIndex(before, "!dkimPass && !s.spfPass")
	if guard < 0 {
		t.Fatal("an account is opened for a new sender with no check that the mail " +
			"authenticated. The From header is whatever the sending machine typed, " +
			"so this is free model calls for anybody who can forge one")
	}
	// And it has to bail rather than log and carry on.
	between := before[guard:]
	if !strings.Contains(between, "continue") {
		t.Error("the authentication check does not stop the message — it falls " +
			"through to opening an account anyway")
	}
}

// And it is still silent about it, which is why the drop was there in the first
// place: a bounce tells whoever is probing that the address is live.
func TestARejectedSenderIsNotToldWhy(t *testing.T) {
	b, err := os.ReadFile("smtp.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	i := strings.Index(src, "unauthenticated sender")
	if i < 0 {
		t.Fatal("the unauthenticated branch has moved; this test needs repointing")
	}
	// The next thing that happens is a log line and a continue, not a reply.
	window := src[i:min(i+400, len(src))]
	for _, leak := range []string{"SendOut(", "deliver(", "SendExternalEmail("} {
		if strings.Contains(window, leak) {
			t.Errorf("a rejected sender is answered with %s — that confirms the "+
				"address is live to whoever is probing it", leak)
		}
	}
}
