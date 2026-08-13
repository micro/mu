package email

// Proving an address is yours.
//
// The rules here are the ones that stop a two-step convenience being a way to
// mail strangers: the code goes to the address rather than back to the caller,
// a wrong one is not a retry you get for free, and asking for codes is capped
// harder than sending is.

import (
	"regexp"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/userdb"
)

// verifier sets an instance up that can send, with an account, and hands back
// the last message that went out.
func verifier(t *testing.T, owner string) *struct{ To, Subject, Body string } {
	t.Helper()
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN": "email.micro.mu",
		"MAIL_DOMAIN":  "micro.mu",
	})
	account(t, owner)

	last := &struct{ To, Subject, Body string }{}
	orig := SendVia
	SendVia = func(_, _, _, to, subject, plain, _ string) (string, error) {
		last.To, last.Subject, last.Body = to, subject, plain
		return "op-verify", nil
	}
	t.Cleanup(func() { SendVia = orig })
	return last
}

// account makes one for the duration of the test, and takes its records away
// afterwards.
func account(t *testing.T, id string) {
	t.Helper()
	if _, err := auth.GetAccount(id); err != nil {
		if err := auth.Create(&auth.Account{ID: id, Name: id}); err != nil {
			t.Skipf("cannot create an account here: %v", err)
		}
		t.Cleanup(func() { auth.DeleteAccount(id) }) //nolint:errcheck
	}
	t.Cleanup(func() { userdb.DeleteOwner(ns, id) }) //nolint:errcheck
}

var sixDigits = regexp.MustCompile(`\d{6}`)

// TestTheCodeGoesToTheAddressAndNowhereElse — the whole proof. A code returned
// to the caller proves nothing, and a code sent anywhere but the address being
// claimed proves something about a different mailbox.
func TestTheCodeGoesToTheAddressAndNowhereElse(t *testing.T) {
	const owner = "email-verify-code"
	last := verifier(t, owner)

	if err := StartVerify(owner, "  Someone@Example.com "); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if last.To != "someone@example.com" {
		t.Errorf("the code went to %q, not the address being claimed", last.To)
	}
	code := sixDigits.FindString(last.Subject)
	if code == "" {
		t.Fatalf("no code in the message: %q / %q", last.Subject, last.Body)
	}
	// Somebody who did not ask for this has to be able to tell what it is and
	// that ignoring it is enough.
	if !strings.Contains(last.Body, "ignore this") {
		t.Errorf("the message does not tell an unwitting recipient to ignore it: %q", last.Body)
	}

	// Not yours until the code comes back.
	if Verified(owner, "someone@example.com") {
		t.Fatal("the address was recorded as proved before any code was returned")
	}
	if pending, ok := Pending(owner); !ok || pending != "someone@example.com" {
		t.Errorf("Pending = %q/%v, so the page cannot tell which step it is on", pending, ok)
	}

	if err := Confirm(owner, "someone@example.com", "000000"); err == nil {
		t.Error("a wrong code was accepted")
	}
	if err := Confirm(owner, "SOMEONE@example.com ", code); err != nil {
		t.Fatalf("confirming with the right code: %v", err)
	}
	if !Verified(owner, "someone@example.com") {
		t.Error("the address is still not the account's after a correct code")
	}
	// Used, so it is gone: a code that works twice is a code somebody else can
	// use once.
	if _, ok := Pending(owner); ok {
		t.Error("the code survived being used")
	}
	if err := Confirm(owner, "someone@example.com", code); err == nil {
		t.Error("the same code worked a second time")
	}
}

// TestAVerificationEmailIsNotSomethingYouSent — the history answers "what did I
// send to people", and a code to yourself is not that. It is charged and capped
// like a send, which is a different question.
func TestAVerificationEmailIsNotSomethingYouSent(t *testing.T) {
	const owner = "email-verify-history"
	verifier(t, owner)

	before := len(History(owner, 100))
	if err := StartVerify(owner, "someone@example.com"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if after := len(History(owner, 100)); after != before {
		t.Errorf("a verification email turned up in the sent history (%d → %d)", before, after)
	}
}

// TestAskingForCodesIsCappedHarderThanSending — this is the one place the
// service mails an address the caller has shown no right to reach. Without a cap
// of its own it is an unbounded "send a message to any address" wearing a
// respectable name.
func TestAskingForCodesIsCappedHarderThanSending(t *testing.T) {
	const owner = "email-verify-cap"
	verifier(t, owner)

	for i := 0; i < verifyCap; i++ {
		if err := StartVerify(owner, addressN(i)); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
	}
	err := StartVerify(owner, addressN(verifyCap))
	if err == nil {
		t.Fatalf("a %dth verification email went out today", verifyCap+1)
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the refusal does not say it is a limit: %v", err)
	}
	if verifyCap >= LimitFor(owner) && LimitFor(owner) > 0 {
		t.Errorf("the cap on codes (%d) is not harder than the cap on sending (%d)",
			verifyCap, LimitFor(owner))
	}
}

func addressN(i int) string {
	return string(rune('a'+i)) + "@example.com"
}

// TestAnAddressAlreadyProvedIsNotOffered — both halves: your own is refused
// without spending a send, and somebody else's is refused before the code goes
// out, so this cannot be used to mail an address by claiming it.
func TestAnAddressAlreadyProvedIsNotOffered(t *testing.T) {
	const mine, theirs = "email-verify-mine", "email-verify-theirs"
	last := verifier(t, mine)
	account(t, theirs)

	if err := StartVerify(mine, "shared@example.com"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if err := Confirm(mine, "shared@example.com", sixDigits.FindString(last.Subject)); err != nil {
		t.Fatalf("confirming: %v", err)
	}

	if err := StartVerify(mine, "shared@example.com"); err == nil ||
		!strings.Contains(err.Error(), "already yours") {
		t.Errorf("re-proving my own address costs a send: %v", err)
	}

	last.To = ""
	if err := StartVerify(theirs, "shared@example.com"); err == nil {
		t.Error("a second account was allowed to claim an address the first proved")
	}
	if last.To != "" {
		t.Errorf("a code was mailed to %q anyway, which is the spam path this closes", last.To)
	}
}

// TestNonsenseIsRefusedBeforeAnythingIsSent.
func TestNonsenseIsRefusedBeforeAnythingIsSent(t *testing.T) {
	const owner = "email-verify-nonsense"
	last := verifier(t, owner)

	for _, addr := range []string{"", "asim", "asim@", "@example.com", "a b@example.com"} {
		last.To = ""
		if err := StartVerify(owner, addr); err == nil {
			t.Errorf("%q was accepted as an address", addr)
		}
		if last.To != "" {
			t.Errorf("%q was mailed anyway, at %q", addr, last.To)
		}
	}
}

// TestConfirmingWithoutAskingIsNotAGuessingGame — no live code means there is
// nothing to guess at, and the answer says which step is missing.
func TestConfirmingWithoutAskingIsNotAGuessingGame(t *testing.T) {
	const owner = "email-verify-unasked"
	verifier(t, owner)

	err := Confirm(owner, "someone@example.com", "123456")
	if err == nil {
		t.Fatal("a code was accepted for an address nothing was sent to")
	}
	if !strings.Contains(err.Error(), "ask for a code first") {
		t.Errorf("the refusal does not say what to do: %v", err)
	}
}

// TestGivingAnAddressUp — an address is not yours forever.
func TestGivingAnAddressUp(t *testing.T) {
	const owner = "email-verify-forget"
	last := verifier(t, owner)

	if err := StartVerify(owner, "old@example.com"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if err := Confirm(owner, "old@example.com", sixDigits.FindString(last.Subject)); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	if err := Forget(owner, "old@example.com"); err != nil {
		t.Fatalf("forgetting: %v", err)
	}
	if Verified(owner, "old@example.com") {
		t.Error("the address is still the account's after being given up")
	}
}
