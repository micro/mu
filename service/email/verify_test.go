package email

// Verifying somebody else's address, which is what this is for.
//
// Most of these hold down the *absence* of a rule. The first version of this
// read the caller as a person claiming addresses for themselves, and every
// limit that follows from that reading — a handful of codes a day, no
// re-checking, one address to one account instance-wide — breaks the actual
// use, which is a product verifying its users. Tests that only check the
// remaining rules would not have caught any of it.

import (
	"regexp"
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/userdb"
)

type outbound struct{ To, Name, Subject, Body string }

// verifier sets up an instance that can send, with an account, and hands back
// the last message that went out.
func verifier(t *testing.T, owner string) *outbound {
	t.Helper()
	withSettings(t, map[string]string{
		"EMAIL_DOMAIN": "email.micro.mu",
		"MAIL_DOMAIN":  "micro.mu",
	})
	account(t, owner)

	last := &outbound{}
	orig := SendVia
	SendVia = func(name, _, _, to, subject, plain, _ string) (string, error) {
		last.To, last.Name, last.Subject, last.Body = to, name, subject, plain
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

func codeIn(t *testing.T, last *outbound) string {
	t.Helper()
	code := sixDigits.FindString(last.Subject + " " + last.Body)
	if code == "" {
		t.Fatalf("no code in the message: %q / %q", last.Subject, last.Body)
	}
	return code
}

// TestTheCodeGoesToTheAddressAndNowhereElse — the whole proof. A code returned
// to the caller proves nothing, and one sent anywhere but the address being
// checked proves something about a different mailbox.
func TestTheCodeGoesToTheAddressAndNowhereElse(t *testing.T) {
	const owner = "email-verify-code"
	last := verifier(t, owner)

	v, err := StartVerify(owner, "  Someone@Example.com ", "Acme")
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if v.Status != "pending" || v.Address != "someone@example.com" {
		t.Fatalf("start returned %+v", v)
	}
	if last.To != "someone@example.com" {
		t.Errorf("the code went to %q, not the address being checked", last.To)
	}
	code := codeIn(t, last)

	got, err := Check(owner, "SOMEONE@example.com ", code)
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	if !got.OK() {
		t.Errorf("the right code was not approved: %+v", got)
	}
	// Used, so it is gone: a code that works twice is a code somebody else can
	// use once.
	if again, _ := Check(owner, "someone@example.com", code); again.OK() {
		t.Error("the same code worked a second time")
	}
}

// TestAWrongCodeIsAnAnswerNotAFailure — somebody mistyping six digits is the
// ordinary path through a signup form, and an agent showing "that code is not
// right" to its user should not have to read it out of an exception.
func TestAWrongCodeIsAnAnswerNotAFailure(t *testing.T) {
	const owner = "email-verify-wrong"
	last := verifier(t, owner)

	if _, err := StartVerify(owner, "someone@example.com", "Acme"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	code := codeIn(t, last)

	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	v, err := Check(owner, "someone@example.com", wrong)
	if err != nil {
		t.Fatalf("a wrong code came back as an error: %v", err)
	}
	if v.OK() || v.Status != "incorrect" {
		t.Errorf("a wrong code reads as %+v", v)
	}
	if v.Left != codeTries-1 {
		t.Errorf("it does not say how many tries are left: %d", v.Left)
	}
	// And the right one still works after a miss.
	if got, _ := Check(owner, "someone@example.com", code); !got.OK() {
		t.Error("one wrong guess killed a live code")
	}
}

// TestSixDigitsIsNotGuessable — a million combinations only matters while the
// number of attempts is small.
func TestSixDigitsIsNotGuessable(t *testing.T) {
	const owner = "email-verify-guessing"
	last := verifier(t, owner)

	if _, err := StartVerify(owner, "someone@example.com", ""); err != nil {
		t.Fatalf("starting: %v", err)
	}
	code := codeIn(t, last)
	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	for i := 0; i < codeTries; i++ {
		if v, _ := Check(owner, "someone@example.com", wrong); v.OK() {
			t.Fatal("a wrong code was approved")
		}
	}
	// The code is torn up rather than left there to be guessed at.
	v, _ := Check(owner, "someone@example.com", code)
	if v.OK() {
		t.Error("the code survived being guessed at five times")
	}
	if v.Status != "exhausted" && v.Status != "none" {
		t.Errorf("after five wrong guesses the state is %q", v.Status)
	}
}

// TestTheMessageIsAboutTheCallersProduct — an agent verifying its users sends
// on behalf of something those people have heard of. A code that turns up
// talking about this instance instead reads like a phish and gets deleted,
// which makes the tool useless however correct the plumbing is.
func TestTheMessageIsAboutTheCallersProduct(t *testing.T) {
	const owner = "email-verify-branding"
	last := verifier(t, owner)

	if _, err := StartVerify(owner, "user@example.com", "Acme Books"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	for what, s := range map[string]string{"subject": last.Subject, "body": last.Body} {
		if !strings.Contains(s, "Acme Books") {
			t.Errorf("the %s does not name the caller's product: %q", what, s)
		}
	}
	if last.Name != "Acme Books" {
		t.Errorf("the message is from %q rather than the product", last.Name)
	}
	// Somebody who did not ask for this must be told that ignoring it is enough.
	if !strings.Contains(last.Body, "ignore this") {
		t.Errorf("the message does not tell an unwitting recipient to ignore it: %q", last.Body)
	}

	// With no product named it falls back to the account, which is right for
	// somebody verifying an address of their own.
	if _, err := StartVerify(owner, "other@example.com", ""); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if last.Name != owner {
		t.Errorf("with no app given the message is from %q", last.Name)
	}
}

// TestTwoProductsCanVerifyTheSamePerson — the rule that broke it worst. Two
// accounts here are two products, and a person can hold an account at both;
// making the second one's verification fail because the first got there is a
// bug in the shape of a policy.
func TestTwoProductsCanVerifyTheSamePerson(t *testing.T) {
	const one, two = "email-verify-one", "email-verify-two"
	last := verifier(t, one)
	account(t, two)

	if _, err := StartVerify(one, "shared@example.com", "One"); err != nil {
		t.Fatalf("the first product: %v", err)
	}
	if v, _ := Check(one, "shared@example.com", codeIn(t, last)); !v.OK() {
		t.Fatal("the first product's check did not approve")
	}

	if _, err := StartVerify(two, "shared@example.com", "Two"); err != nil {
		t.Fatalf("the second product cannot verify the same person: %v", err)
	}
	if v, _ := Check(two, "shared@example.com", codeIn(t, last)); !v.OK() {
		t.Error("the second product's check did not approve")
	}
}

// TestAnAddressCanBeCheckedAgain — re-confirming on an address change, or after
// somebody lost the email, is ordinary. Refusing it is a limit that only makes
// sense if the caller is thought to be doing this once ever.
func TestAnAddressCanBeCheckedAgain(t *testing.T) {
	const owner = "email-verify-again"
	last := verifier(t, owner)

	for i := 0; i < 2; i++ {
		if _, err := StartVerify(owner, "user@example.com", "Acme"); err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		if v, _ := Check(owner, "user@example.com", codeIn(t, last)); !v.OK() {
			t.Fatalf("round %d did not approve", i)
		}
	}
}

// TestVolumeIsTheDailyEmailAllowanceAndNothingElse — the tier is what bounds
// how many people you can verify, because a verification *is* a send. A second
// per-day counter invented for this tool would meter the same thing twice and
// cap a paying account below what it bought.
func TestVolumeIsTheDailyEmailAllowanceAndNothingElse(t *testing.T) {
	const owner = "email-verify-volume"
	verifier(t, owner)

	// Comfortably more than any per-day ceiling the old version had, spread over
	// different addresses so the per-recipient throttle is not what is under
	// test.
	for i := 0; i < 12; i++ {
		if _, err := StartVerify(owner, addressN(i), "Acme"); err != nil {
			t.Fatalf("verification %d of twelve was refused: %v — something other "+
				"than the daily email allowance is capping this", i+1, err)
		}
	}
}

func addressN(i int) string {
	return string(rune('a'+i%26)) + string(rune('a'+i/26)) + "@example.com"
}

// TestOneAddressCannotBeBuried — the one limit here that is about the person
// receiving rather than the caller paying. Without it, "email a code to any
// address" is a way to fill somebody's inbox with a respectable name on it.
func TestOneAddressCannotBeBuried(t *testing.T) {
	const owner = "email-verify-throttle"
	last := verifier(t, owner)

	for i := 0; i < perAddress; i++ {
		if _, err := StartVerify(owner, "target@example.com", "Acme"); err != nil {
			t.Fatalf("code %d: %v", i, err)
		}
	}
	last.To = ""
	_, err := StartVerify(owner, "target@example.com", "Acme")
	if err == nil {
		t.Fatalf("a %dth code went to one address within the hour", perAddress+1)
	}
	if last.To != "" {
		t.Errorf("it was mailed anyway, to %q", last.To)
	}
	// And a different address is unaffected — the ceiling is per recipient, not
	// a cap on the caller.
	if _, err := StartVerify(owner, "someone-else@example.com", "Acme"); err != nil {
		t.Errorf("one busy address stopped the caller verifying anybody: %v", err)
	}
}

// TestVerifyingSomebodyElseDoesNotPutThemOnYourAccount — the caller's users are
// not accounts here, and asking whether somebody can read their mail is not a
// reason to record their address as one of yours.
func TestVerifyingSomebodyElseDoesNotPutThemOnYourAccount(t *testing.T) {
	const owner = "email-verify-notmine"
	last := verifier(t, owner)

	if _, err := StartVerify(owner, "auser@example.com", "Acme"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if v, _ := Check(owner, "auser@example.com", codeIn(t, last)); !v.OK() {
		t.Fatal("the check did not approve")
	}
	if Verified(owner, "auser@example.com") {
		t.Error("a user's address was recorded as the caller's own")
	}

	// Claim is the opt-in, and it is what the page uses.
	if err := Claim(owner, "auser@example.com"); err != nil {
		t.Fatalf("claiming: %v", err)
	}
	if !Verified(owner, "auser@example.com") {
		t.Error("claiming an address did not record it")
	}
	if err := Forget(owner, "auser@example.com"); err != nil {
		t.Fatalf("forgetting: %v", err)
	}
	if Verified(owner, "auser@example.com") {
		t.Error("the address is still the account's after being given up")
	}
}

// TestNonsenseIsRefusedBeforeAnythingIsSent.
func TestNonsenseIsRefusedBeforeAnythingIsSent(t *testing.T) {
	const owner = "email-verify-nonsense"
	last := verifier(t, owner)

	for _, addr := range []string{"", "asim", "asim@", "@example.com", "a b@example.com"} {
		last.To = ""
		if _, err := StartVerify(owner, addr, "Acme"); err == nil {
			t.Errorf("%q was accepted as an address", addr)
		}
		if last.To != "" {
			t.Errorf("%q was mailed anyway, at %q", addr, last.To)
		}
	}
}

// TestCheckingWithoutAskingSaysSo — no live code means there is nothing to
// check, and the caller has to be able to tell that apart from a wrong one.
func TestCheckingWithoutAskingSaysSo(t *testing.T) {
	const owner = "email-verify-unasked"
	verifier(t, owner)

	v, err := Check(owner, "someone@example.com", "123456")
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	if v.OK() || v.Status != "none" {
		t.Errorf("a code was checked against nothing and read as %+v", v)
	}
}

// TestAVerificationEmailIsNotSomethingYouSent — the history answers "what did I
// send to people". A code is charged and capped like a send, which is a
// different question from whether it belongs in that list.
func TestAVerificationEmailIsNotSomethingYouSent(t *testing.T) {
	const owner = "email-verify-history"
	verifier(t, owner)

	before := len(History(owner, 100))
	if _, err := StartVerify(owner, "someone@example.com", "Acme"); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if after := len(History(owner, 100)); after != before {
		t.Errorf("a verification email turned up in the sent history (%d → %d)", before, after)
	}
}
