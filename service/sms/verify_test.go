package sms

import (
	"strings"
	"testing"
)

// The whole round trip, with the provider stood in front of.
//
// None of this was reachable by a test before: the only way past the send was a
// live Twilio account, so every path after it — storing the code, finding it
// again, the number ending up yours — was asserted by nobody. "Ask for a code
// first", in answer to somebody asking for a code, is exactly the kind of thing
// that hides there.
func TestVerifyRoundTrip(t *testing.T) {
	setup(t)
	const me, mine = "acct-1", "+447700900123"

	var sent string
	send = func(to, body string) (string, error) { sent = body; return "SM123", nil }
	t.Cleanup(func() { send = deliver })

	if _, waiting := Pending(me); waiting {
		t.Fatal("a code is pending before one was asked for")
	}

	if err := StartVerify(me, mine); err != nil {
		t.Fatalf("StartVerify: %v", err)
	}

	number, waiting := Pending(me)
	if !waiting || number != mine {
		t.Fatalf("Pending = %q %v, want %q true — the page has no second step without it", number, waiting, mine)
	}

	// The code in the message is the code that works. Nothing else checked that
	// the thing texted and the thing stored were the same thing.
	code := codeIn(t, sent)
	if err := Confirm(me, mine, "000000"); err == nil {
		t.Error("a wrong code was accepted")
	}
	if err := Confirm(me, mine, code); err != nil {
		t.Fatalf("Confirm with the code that was texted: %v", err)
	}
	if !Verified(me, mine) {
		t.Error("the number is not the caller's after confirming")
	}
	if _, waiting := Pending(me); waiting {
		t.Error("the code is still live after it was used")
	}
}

// Asking again replaces the code rather than adding a second live one.
func TestAskingAgainReplacesTheCode(t *testing.T) {
	setup(t)
	const me, mine = "acct-1", "+447700900123"

	var last string
	send = func(to, body string) (string, error) { last = body; return "SM1", nil }
	t.Cleanup(func() { send = deliver })

	if err := StartVerify(me, mine); err != nil {
		t.Fatal(err)
	}
	first := codeIn(t, last)
	if err := StartVerify(me, mine); err != nil {
		t.Fatal(err)
	}
	second := codeIn(t, last)

	if first == second {
		t.Skip("two random codes came out the same; nothing to tell apart")
	}
	if err := Confirm(me, mine, first); err == nil {
		t.Error("the superseded code still works — two live codes double the guessing surface")
	}
	if err := Confirm(me, mine, second); err != nil {
		t.Errorf("the current code does not work: %v", err)
	}
}

// codeIn pulls the digits out of "Your code is 123456. …".
func codeIn(t *testing.T, body string) string {
	t.Helper()
	var digits strings.Builder
	for _, r := range body {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	got := digits.String()
	if len(got) < 6 {
		t.Fatalf("no code in %q", body)
	}
	return got[:6]
}

// Verifying a number is what makes a message from it yours.
//
// This was the whole point and it did not happen: an inbound message went to
// whoever last texted that number from here, so texting the instance from your
// own phone reached nobody unless you had texted yourself first. "Verified"
// meant a label in an autocomplete, and the page said it was what let a reply
// reach you, which was not true.
func TestVerifyingClaimsWhatComesFromTheNumber(t *testing.T) {
	setup(t)
	const me, other, mine = "acct-1", "acct-2", "+447700900123"

	if got := OwnerOf(mine); got != Fallback() {
		t.Fatalf("OwnerOf = %q before anybody claimed it, want the fallback %q", got, Fallback())
	}

	if err := Verify(me, mine); err != nil {
		t.Fatal(err)
	}
	if got := OwnerOf(mine); got != me {
		t.Errorf("OwnerOf = %q, want %q — verifying a number should route it", got, me)
	}

	// Somebody else texting that phone from here does not take it over. They
	// have shown they know the number, not that it is theirs.
	Record(other, "out", mine, "hello", 1)
	if got := OwnerOf(mine); got != me {
		t.Errorf("OwnerOf = %q after somebody else texted it, want %q", got, me)
	}

	// Giving it up hands it back to the last conversation.
	Forget(me, mine)
	if got := OwnerOf(mine); got != other {
		t.Errorf("OwnerOf = %q after the owner gave it up, want the last conversation %q", got, other)
	}
}
