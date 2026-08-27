package mail

// The welcome has one job beyond being polite: it has to be answerable.
//
// A message from no-reply@ saying "reply and I'll answer" is worse than no
// message, so what is asserted here is the address it comes from and the fact
// that it reaches the mailbox — not the prose, which will change.

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/service/mail"
)

func TestTheWelcomeArrivesAndCanBeAnsweredWithoutSetup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	if err := auth.Create(&auth.Account{ID: "newcomer", Name: "Ada Lovelace", Secret: "s"}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}
	before := len(mail.ListMessages("newcomer", 100))

	greet("newcomer", "Ada Lovelace")

	msgs := mail.ListMessages("newcomer", 100)
	if len(msgs) != before+1 {
		t.Fatalf("the welcome did not arrive: %d messages, want one more than %d",
			len(msgs), before)
	}
	got := msgs[0]

	// From the agent's address, because that is the one that wakes it. A
	// welcome from no-reply@ that says "reply to this" is a lie the product
	// tells on the first screen.
	if want := mail.SharedAgentAddress(); got.FromID != want {
		t.Errorf("the welcome comes from %q, want %q — a reply to anything else "+
			"lands in a mailbox nothing is watching", got.FromID, want)
	}

	// And it threads, so the answer joins this conversation rather than
	// starting a second one beside it.
	if !strings.HasPrefix(got.MessageID, "<welcome.") {
		t.Errorf("no Message-ID of ours on the welcome: %q", got.MessageID)
	}

	body := got.Body
	// Their own address, spelled out. It is the thing they have that they did
	// not have an hour ago.
	if !strings.Contains(body, "newcomer@mu.test") {
		t.Error("the welcome does not tell them their own address")
	}
	// Said hello to the person, not to the account id.
	if !strings.Contains(body, "Hello Ada") {
		t.Errorf("the greeting does not use their name:\n%s", body[:min(200, len(body))])
	}
	// The two things they will need and cannot guess.
	for _, want := range []string{"/tools", "/wallet/topup"} {
		if !strings.Contains(body, want) {
			t.Errorf("the welcome does not point at %s", want)
		}
	}
}

// An instance with no mail domain still greets, and does not invent an
// address to do it with.
//
// The inbox, the handle and the agent all work with nothing configured — see
// service/mail — so the welcome is as true there as anywhere. What it must not
// do is say "you are quiet@localhost", which is the bug that package already
// records: ConfiguredDomain answers "localhost" so the SMTP handshake has a
// hostname, and reading it out to a person turns a protocol default into a
// promise.
func TestNoMailDomainMeansNoInventedAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "")

	if err := auth.Create(&auth.Account{ID: "quiet", Name: "Quiet", Secret: "s"}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}
	greet("quiet", "Quiet")

	msgs := mail.ListMessages("quiet", 100)
	if len(msgs) != 1 {
		t.Fatalf("%d messages delivered, want the welcome", len(msgs))
	}
	if body := msgs[0].Body; strings.Contains(body, "localhost") {
		t.Errorf("the welcome tells them their address is at localhost:\n%s", body)
	}
	// It still says who they are, because a handle works with no domain.
	if !strings.Contains(msgs[0].Body, "<code>quiet</code>") {
		t.Errorf("the welcome does not say who they are here:\n%s", msgs[0].Body)
	}
}

// The operator is named by their address here, never by the one they signed up
// with — that is their personal mail and is not ours to hand out.
func TestTheOperatorIsNamedByTheirAddressHere(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	if err := auth.Create(&auth.Account{ID: "boss", Name: "Boss", Secret: "s",
		Admin: true, Email: "private@elsewhere.example"}); err != nil {
		t.Fatalf("creating the admin: %v", err)
	}
	// Whoever auth says the operator is, rather than the account just made
	// admin: the first account on a fresh instance is bootstrapped to admin, so
	// in a test binary that has already created one, "boss" is the second.
	// Asserting on the name would be asserting on test ordering.
	op := auth.Operator()
	if op == "" {
		t.Fatal("no admin on this instance, so this test asserts nothing")
	}

	body := welcomeBody("someone", "Someone")
	if !strings.Contains(body, op+"@mu.test") {
		t.Errorf("the welcome does not say who to write to when something is "+
			"wrong; the operator is %q", op)
	}
	if strings.Contains(body, "private@elsewhere.example") {
		t.Error("the welcome hands out the operator's personal address")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
