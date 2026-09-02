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

	"mu/internal/app"
)

// Nothing is offered that is not configured.
//
// With no Twilio account and no mail domain there is one client — the web, the
// one that needs no configuration and cannot be switched off.
func TestAnUnconfiguredInstanceOffersOnlyTheWeb(t *testing.T) {
	for _, k := range []string{"TWILIO_ACCOUNT_SID", "TWILIO_AUTH_TOKEN", "TWILIO_FROM",
		"TWILIO_WHATSAPP_FROM", "TWILIO_MESSAGING_SERVICE_SID", "MAIL_DOMAIN", "MU_DOMAIN"} {
		t.Setenv(k, "")
	}
	got := Clients()
	if len(got) != 1 {
		t.Fatalf("an unconfigured instance offers %d ways in: %+v", len(got), got)
	}
	if got[0].Label != "Web" {
		t.Errorf("the one client on a bare instance is %q", got[0].Label)
	}
}

// And a number that is there is offered, with the channel it rides on.
func TestAConfiguredNumberIsOffered(t *testing.T) {
	t.Setenv("MU_DOMAIN", "example.test")
	t.Setenv("TWILIO_ACCOUNT_SID", "AC"+strings.Repeat("0", 32))
	t.Setenv("TWILIO_AUTH_TOKEN", strings.Repeat("a", 32))
	t.Setenv("TWILIO_FROM", "+14155550100")
	t.Setenv("TWILIO_WHATSAPP_FROM", "+14155550199")

	byLabel := map[string]Client{}
	for _, c := range Clients() {
		byLabel[c.Label] = c
	}
	if got := byLabel["SMS"].Address; got != "+14155550100" {
		t.Errorf("SMS address = %q", got)
	}
	if got := byLabel["WhatsApp"].Address; got != "+14155550199" {
		t.Errorf("WhatsApp address = %q", got)
	}
	// The WhatsApp number is the WhatsApp sender, not the SMS one. They are two
	// different numbers and printing the text number under a WhatsApp heading
	// sends people to a number that is not registered with Meta at all.
	if byLabel["SMS"].Address == byLabel["WhatsApp"].Address {
		t.Error("both channels are showing the same number")
	}
	if got := byLabel["API"].Address; !strings.Contains(got, "example.test") {
		t.Errorf("the API line does not name this instance: %q", got)
	}
}

// The names on the card are the names in the record, so what the page offers
// and what /recall calls a conversation are the same word.
//
// Two lists of client names would be one that had drifted, and the drift is
// invisible from either side: the page says WhatsApp, the record says sms, and
// nothing looks wrong on either.
func TestTheClientNamesAreTheRecordsNames(t *testing.T) {
	src, err := os.ReadFile("contact.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"thread.WebClient", "thread.SMSClient", "thread.WhatsAppClient"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("the card spells a client name itself instead of using %s", want)
		}
	}
}

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

// And it is where a stranger looks: the footer, first.
//
// The footer renders on exactly one kind of page — the signed-out one — so it
// is the whole of this page's discoverability for somebody with no account.
func TestTheFooterLeadsWithIt(t *testing.T) {
	f := app.FooterLinks()
	if !strings.Contains(f, `href="/contact"`) {
		t.Fatal("the footer does not link to the contact card")
	}
	if i, j := strings.Index(f, `href="/contact"`), strings.Index(f, `href="/archive"`); i > j {
		t.Error("Archive still leads the footer — the first link is the question a\n" +
			"stranger is actually asking, and on a personal assistant that is\n" +
			"how to reach it")
	}
}
