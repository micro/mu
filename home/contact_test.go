package home

// How to reach the assistant, and the two ways this page can lie.
//
// It can offer a door that is not there — a number on an instance with no
// Twilio account, which sends somebody to text nothing and reads as broken
// rather than unconfigured. And it can be behind a session, which makes "how do
// I contact you" a question you have to already be a customer to ask.

import (
	"os"
	"strings"
	"testing"
)

// The page is public, because the question is.
//
// Every address on it is one this instance publishes anyway — a number that
// answers texts, an address that answers mail. Gating it is asking somebody to
// sign up before they can find out what they would be signing up to.
func TestTheCardIsReadableWithoutAnAccount(t *testing.T) {
	body := contactBody(nil)
	if !strings.Contains(body, "How to reach Micro") {
		t.Error("the card has no heading")
	}
	// And it says what a signed-out reader has to do to use the phone half,
	// rather than showing a number that will not recognise them.
	if !strings.Contains(body, `href="/signup"`) {
		t.Error("signed out, the card does not say that texting needs an account first")
	}
	// The route table has to agree. A public page listed as gated is a page
	// nobody outside can read, whatever this function returns.
	src, err := os.ReadFile("../internal/server/routes.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"/contact":                       false,`) {
		t.Error("/contact is not public in the route table")
	}
}
