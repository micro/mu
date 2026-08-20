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

	// Both spellings, because a sending server may send either and the local
	// part is case-insensitive here.
	box := AgentMailbox
	for _, addr := range []string{box + "@micro.mu", strings.ToUpper(box) + "@micro.mu"} {
		if err := rcpt(t, addr); err != nil {
			t.Errorf("%s refused at RCPT TO (%v) — it is a reserved username with no "+
				"account, so Data never gets to answer it", addr, err)
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

	// The reserved name must not be a way past the relay check, which runs
	// first. agent@gmail.com is not ours.
	if err := rcpt(t, AgentMailbox+"@example.net"); err == nil {
		t.Errorf("%s@example.net was accepted — this instance is an open relay", AgentMailbox)
	}
}

// The tag names which agent answers, so it is part of the address.
//
// This used to be a contrast: agent+research@ accepted, support+anything@
// refused, because Data resolved support only with an empty tag and accepting a
// tagged one meant taking mail and then throwing it away. support@ is gone, so
// what is left is the half that was always the point.
func TestTheTagIsPartOfTheAgentAddress(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	if err := rcpt(t, AgentMailbox+"+research@micro.mu"); err != nil {
		t.Errorf("agent+research@micro.mu refused (%v) — the tag names which agent "+
			"answers, so this is an address, not a typo", err)
	}
}

// The loop guard has to see through the tag.
//
// An agent answering from agent+research@ and being written back to is a fresh
// run every turn, forever, at a model call each. The guard compared against the
// plain shared address, which stopped being enough the moment the tagged form
// existed.
func TestAnAgentsOwnReplyDoesNotWakeItAgain(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "micro.mu")

	for _, from := range []string{
		"agent@micro.mu", "AGENT@micro.mu", "agent+research@micro.mu",
		"agent+anything+nested@micro.mu",
	} {
		if !fromSharedAgent(from) {
			t.Errorf("%s is not recognised as this instance's own agent address, so its "+
				"reply wakes the agent again and the two of them talk forever", from)
		}
	}
	// And it is not a way to wave through anybody who happens to be called
	// agent somewhere else, or a local user whose name starts the same way.
	for _, from := range []string{
		"agent@example.net", "agentsmith@micro.mu", "notagent@micro.mu", "",
	} {
		if fromSharedAgent(from) {
			t.Errorf("%s was treated as this instance's own agent address, so real mail "+
				"from it is silently ignored", from)
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

	for _, name := range []string{"AgentMailbox"} {
		if !strings.Contains(door, name) {
			t.Errorf("Data handles %s but Rcpt does not mention it, so that mail is "+
				"refused 550 before Data ever runs", name)
		}
		if !strings.Contains(delivery, name) {
			t.Errorf("Rcpt lets %s through the door and Data has nowhere to put it", name)
		}
	}
}

// Whatever address was written to is what answers.
//
// The reply address used to be rebuilt from whichever agent answered rather
// than taken from the message, so mail to agent@ came back from
// agent+micro@ — the catch-all resolves to the agent named micro, and naming
// it changed the address mid-conversation. To the recipient that is a stranger
// arriving out of nowhere in a thread they started, which is confusing and a
// spam signal, and it is why the first real reply this instance sent was
// filtered.
//
// The recipient now travels with the message, so there is nothing to rebuild.
func TestTheAddressWrittenToTravelsWithTheMessage(t *testing.T) {
	src := readSource(t, "smtp.go")

	i := strings.Index(src, "deliverInbound(InboundMail{")
	if i < 0 {
		t.Fatal("cannot find where inbound mail is handed on")
	}
	handoff := src[i:]
	if end := strings.Index(handoff, "wakeRequest{"); end > 0 {
		handoff = handoff[:end]
	}
	if !strings.Contains(handoff, "To:") {
		t.Error("the address the message was sent to is not passed to the handler, " +
			"so an answer has to guess which address it came from")
	}
}
