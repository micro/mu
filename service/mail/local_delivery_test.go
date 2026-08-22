package mail

// Mail to this instance is delivered here, not relayed to it.
//
// IsExternalEmail tested for an @ and nothing else, so asim@micro.mu on the
// instance that serves micro.mu counted as external. The send path relayed it
// out over SMTP, it arrived back at our own MX, and the anti-spoofing check
// refused it: "Sender address rejected: not authorized to send from this
// domain". Writing to yourself on your own server failed, and so did writing
// to one of your own agents at you+name@ — the address the product hands out
// and the one the inbound-agent trigger listens on.
//
// IsExternalAddress, three lines away in mail.go, had it right the whole time.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"mu/internal/auth"
)

func withDomain(t *testing.T, d string) {
	t.Helper()
	prev := os.Getenv("MAIL_DOMAIN")
	os.Setenv("MAIL_DOMAIN", d)
	t.Cleanup(func() { os.Setenv("MAIL_DOMAIN", prev) })
}

func TestOurOwnDomainIsNotExternal(t *testing.T) {
	withDomain(t, "micro.mu")

	for _, local := range []string{
		"asim@micro.mu",
		"asim+claude@micro.mu", // an agent's address
		"ASIM@MICRO.MU",        // headers arrive in any case
		" asim@micro.mu ",
		"asim", // a bare username was always local
	} {
		if IsExternalEmail(local) {
			t.Errorf("%q was treated as external, so it is relayed out and bounces back", local)
		}
	}

	for _, external := range []string{"someone@example.com", "asim@aslam.me", "a@micro.mu.evil.com"} {
		if !IsExternalEmail(external) {
			t.Errorf("%q was treated as local, so it would never be sent", external)
		}
	}
}

// Whatever form the address is written in, it resolves to one account.
func TestALocalAddressResolvesToItsAccount(t *testing.T) {
	withDomain(t, "micro.mu")

	for _, c := range []struct{ in, want string }{
		{"asim", "asim"},
		{"asim@micro.mu", "asim"},
		{"asim+claude@micro.mu", "asim"},
		{"asim+claude", "asim"},
		{"  ASIM@micro.mu ", "asim"},
	} {
		if got := LocalRecipient(c.in); got != c.want {
			t.Errorf("LocalRecipient(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every send path resolves the recipient, not just the one that got fixed.
//
// There are two: a form post from the page, and a JSON body from mail_send.
// LocalRecipient went into the form path only, the unit tests above passed,
// and the tool the bug was reported against still answered "recipient not
// found" — because auth.GetAccount takes a username and the caller had been
// handed an address. A resolver helps nobody on the paths that skip it.
func TestNoSendPathLooksUpARecipientWithoutResolvingItFirst(t *testing.T) {
	src, err := os.ReadFile("mail.go")
	if err != nil {
		t.Fatal(err)
	}
	raw := regexp.MustCompile(`auth\.GetAccount\((?:to|req\.To|recipient)\)`)
	for i, line := range strings.Split(string(src), "\n") {
		if raw.MatchString(line) {
			t.Errorf("mail.go:%d resolves a recipient with the raw address — "+
				"a caller writing to the address mail_address gave them gets "+
				"\"recipient not found\". Wrap it in LocalRecipient:\n\t%s",
				i+1, strings.TrimSpace(line))
		}
	}
}

// An instance with no domain configured has no local domain to compare
// against, so anything with an @ is somewhere else.
func TestWithNoDomainEverythingAddressedIsExternal(t *testing.T) {
	withDomain(t, "")
	if !IsExternalEmail("someone@example.com") {
		t.Error("mail would never leave an instance with no MAIL_DOMAIN set")
	}
	if IsExternalEmail("asim") {
		t.Error("a bare username is always local")
	}
}

// Answering somebody on this instance delivers; it does not relay.
//
// This is the same bug one layer up, and it was live. Writing to agent@ from
// your own address on your own instance produced an answer that was written,
// recorded, and visible on /inbox — and never delivered, because agent/mail
// called SendExternalReplyAll whatever the address was. The relay looked up the
// MX for our own domain, arrived at our own SMTP server, and was refused:
//
//	550 5.0.0 Sender address rejected: not authorized to send from this domain
//
// IsExternalAddress was right; nothing on that path asked it.
func TestAnsweringSomebodyHereDeliversRatherThanRelaying(t *testing.T) {
	withDomain(t, "mu.test")
	if err := auth.Create(&auth.Account{ID: "localasim", Name: "Local Asim", Secret: "s"}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}

	before := len(ListMessages("localasim", 100))

	// No relay host and no MX for mu.test, so any attempt to relay fails. A nil
	// error is the assertion: nothing was relayed.
	_, err := SendReplyAll("localasim", "Agent", "agent@mu.test", "localasim@mu.test", nil,
		"Re: is this working", "yes", "<p>yes</p>", "<in@reply.to>", "")
	if err != nil {
		t.Fatalf("answering somebody on this instance failed: %v", err)
	}

	after := ListMessages("localasim", 100)
	if len(after) != before+1 {
		t.Fatalf("the answer was not delivered: %d messages before, %d after",
			before, len(after))
	}
	if got := after[0].Subject; got != "Re: is this working" {
		t.Errorf("the delivered message is %q", got)
	}
}

// And somebody outside still goes out. A router that delivered everything
// locally would pass the test above and break every real reply.
func TestAnsweringSomebodyOutsideStillRelays(t *testing.T) {
	withDomain(t, "mu.test")

	// Nothing can relay in a test, so the proof that it tried is that it
	// failed. Silence would mean the message was quietly dropped.
	_, err := SendReplyAll("someone", "Agent", "agent@mu.test", "someone@example.com", nil,
		"Re: hello", "hi", "<p>hi</p>", "", "")
	if err == nil {
		t.Error("an external reply reported success with no relay configured, so it " +
			"went nowhere and said nothing")
	}
}

// A thread with one person here and one outside is both.
//
// The fix that suggests itself is a branch at the call site — if the recipient
// is local, deliver — and it gets this wrong: the local person is in Cc, the
// branch looks at To, and one of the two never hears back.
func TestAThreadWithBothKindsOfRecipientReachesBoth(t *testing.T) {
	withDomain(t, "mu.test")
	if err := auth.Create(&auth.Account{ID: "ccasim", Name: "Cc Asim", Secret: "s"}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}

	before := len(ListMessages("ccasim", 100))

	// To is outside, so the relay is attempted and fails — but the local
	// recipient in Cc must still be delivered to.
	_, _ = SendReplyAll("ccasim", "Agent", "agent@mu.test", "someone@example.com",
		[]string{"ccasim@mu.test"}, "Re: both", "hi", "<p>hi</p>", "", "")

	if after := ListMessages("ccasim", 100); len(after) != before+1 {
		t.Errorf("the local person on a mixed thread got %d messages, want one more than %d",
			len(after), before)
	}
}
