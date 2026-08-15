package mail

// The two addresses this instance offers that nobody holds.
//
// support@ and agent@ are reserved usernames — internal/auth/username.go keeps
// them so nobody can register one — which means the account lookup in Rcpt
// refuses them exactly like a stranger's typo. support@ had an explicit allow
// for that reason. agent@ did not, so mail to it was refused 550 at RCPT TO and
// Data, which knows precisely what to do with it, never ran. The whole
// write-to-your-agent-and-it-writes-back path was unreachable while the code
// answering it sat there working.
//
// The pairing is the test. Whatever Data resolves without an account, Rcpt has
// to let through, and the two lists drifting apart is the bug — so this checks
// them against each other rather than against a fixture.

import (
	"errors"
	"strings"
	"testing"

	smtpd "github.com/emersion/go-smtp"
)

// rcpt runs one RCPT TO from a non-localhost peer, which is the path a real
// sender takes. From localhost every recipient is accepted and none of this is
// exercised.
func rcpt(t *testing.T, to string) error {
	t.Helper()
	s := &Session{remoteIP: "203.0.113.9", from: "someone@example.com"}
	return s.Rcpt(to, nil)
}

func code(err error) int {
	var e *smtpd.SMTPError
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

func TestTheReservedMailboxesAreAcceptedAtTheDoor(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	for _, box := range []string{SupportMailbox, AgentMailbox} {
		// Both spellings, because a sending server may send either and the
		// local part is case-insensitive here.
		for _, addr := range []string{box + "@micro.mu", strings.ToUpper(box) + "@micro.mu"} {
			if err := rcpt(t, addr); err != nil {
				t.Errorf("%s refused at RCPT TO (%v) — it is a reserved username with no "+
					"account, so Data never gets to answer it", addr, err)
			}
		}
	}
}

func TestAnUnknownUserIsStillRefused(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	err := rcpt(t, "nobody-by-that-name@micro.mu")
	if err == nil {
		t.Fatal("mail for a user who does not exist was accepted — the reserved-mailbox " +
			"allow has swallowed the account check and this is an open door")
	}
	if got := code(err); got != 550 {
		t.Errorf("refused with %d, want 550 so the sender's own server tells them", got)
	}
}

func TestAnotherDomainIsStillNotRelayed(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	// The reserved names must not be a way past the relay check, which runs
	// first. agent@gmail.com is not ours.
	for _, box := range []string{SupportMailbox, AgentMailbox} {
		if err := rcpt(t, box+"@example.net"); err == nil {
			t.Errorf("%s@example.net was accepted — this instance is an open relay", box)
		}
	}
}

// A tagged reserved address bounces rather than vanishing.
//
// Data resolves support@ and agent@ only when there is no +tag: agent+foo@
// falls through to an account lookup for "agent", finds nothing and drops the
// message with a log line. Accepting it at the door would mean taking
// responsibility for mail and then silently discarding it, which is the one
// outcome worse than a bounce.
func TestATaggedReservedAddressIsRefusedRatherThanDropped(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	for _, box := range []string{SupportMailbox, AgentMailbox} {
		if err := rcpt(t, box+"+anything@micro.mu"); err == nil {
			t.Errorf("%s+anything@micro.mu was accepted at the door, but Data resolves "+
				"the reserved mailboxes only with an empty tag — so this message is "+
				"accepted and then thrown away", box)
		}
	}
}

// Rcpt and Data have to agree about which names need no account.
//
// They are two lists in two functions forty lines apart, and the bug this file
// exists for was exactly them disagreeing: Data grew a case for agent@ and Rcpt
// did not. Reading the source is crude, but it fails when somebody adds a third
// reserved mailbox to one of them.
func TestBothEndsOfTheDoorKnowTheSameMailboxes(t *testing.T) {
	src := readSource(t, "smtp.go")

	i := strings.Index(src, "func (s *Session) Rcpt(")
	j := strings.Index(src, "func (s *Session) Data(")
	if i < 0 || j < 0 || i >= j {
		t.Fatal("cannot find Rcpt and Data in smtp.go, in that order")
	}
	door, delivery := src[i:j], src[j:]

	for _, name := range []string{"SupportMailbox", "AgentMailbox"} {
		if !strings.Contains(door, name) {
			t.Errorf("Data handles %s but Rcpt does not mention it, so that mail is "+
				"refused 550 before Data ever runs", name)
		}
		if !strings.Contains(delivery, name) {
			t.Errorf("Rcpt lets %s through the door and Data has nowhere to put it", name)
		}
	}
}
