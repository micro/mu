package account

// What /account says about each channel, in both directions.

import (
	"strings"
	"testing"
)

// The phone card says both directions, not just the one it listens on.
//
// Every section on /account asks the same question — prove this is yours, so we
// will listen — and none of them said how the agent reaches you or what number
// it comes from. So the number a person needs in order to save the contact was
// on the page nowhere, and arrived unannounced in a text about a code. See
// #1485.
func TestThePhoneCardSaysWhatItTextsYouFrom(t *testing.T) {
	t.Setenv("TWILIO_FROM", "+447700900999")

	got := agentNumber()
	if got == "" {
		t.Fatal("a configured sending number produces no line on the card")
	}
	if !strings.Contains(got, "+447700900999") {
		t.Errorf("the sending number is not on the card: %s", got)
	}
	if !strings.Contains(got, "Micro") {
		t.Errorf("nothing says what to save the contact as: %s", got)
	}
}

// And says nothing at all where the instance cannot text.
//
// A line about where texts come from, on an instance that sends none, is
// furniture that teaches the page is broken.
func TestWithNoSendingNumberThePhoneCardIsSilentAboutIt(t *testing.T) {
	t.Setenv("TWILIO_FROM", "")
	t.Setenv("SMS_FROM", "")

	if got := agentNumber(); got != "" {
		t.Errorf("an instance with no number still says where it texts from: %s", got)
	}
}
