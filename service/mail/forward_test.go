package mail

// Mail that arrives here also reaches you where you already read mail.
//
// Two properties, and the second is the one that makes the first defensible:
// nothing is forwarded that should not be, and every forwarded message carries
// a way to stop them that needs no login.

import (
	"strings"
	"testing"

	"mu/internal/app"
	"mu/internal/auth"
)

func TestAnUnsubscribeTokenNamesOneAccountAndCannotBeForged(t *testing.T) {
	tok := UnsubscribeToken("alice")
	if tok == "" {
		t.Fatal("no token issued")
	}
	if got := accountFromToken(tok); got != "alice" {
		t.Errorf("token resolved to %q, want alice", got)
	}

	// Bob cannot unsubscribe Alice by editing the link.
	forged := "bob." + strings.SplitN(tok, ".", 2)[1]
	if got := accountFromToken(forged); got != "" {
		t.Errorf("a forged token resolved to %q — anybody could turn off\n"+
			"anybody else's mail by editing a link", got)
	}
	for _, junk := range []string{"", "nodot", "alice.", ".sig", "alice.wrongsig"} {
		if got := accountFromToken(junk); got != "" {
			t.Errorf("%q resolved to %q", junk, got)
		}
	}
}

// Default on: every account that already exists has an inbox nobody told them
// about, which is the whole reason for this.
func TestForwardingIsOnUntilSomebodySaysOtherwise(t *testing.T) {
	if !ForwardingOn("someone-new") {
		t.Error("forwarding is off by default, so the accounts this exists for —\n" +
			"the ones that have never been told they have mail — stay untold")
	}
	SetForwarding("someone-new", false)
	if ForwardingOn("someone-new") {
		t.Error("unsubscribing did not take")
	}
	SetForwarding("someone-new", true)
	if !ForwardingOn("someone-new") {
		t.Error("it cannot be turned back on")
	}
}

// Nobody is nobody. An empty account id must not read as "on".
func TestNoAccountIsNotForwarded(t *testing.T) {
	if ForwardingOn("") {
		t.Error("forwarding is on for the empty account")
	}
	if UnsubscribeToken("") != "" {
		t.Error("the empty account got a token")
	}
}

// Every copy says how to stop, in the body — not only in a header a client may
// or may not surface.
func TestEveryForwardedMessageSaysHowToStop(t *testing.T) {
	m := InboundMail{
		Owner: "alice", From: "bob", FromName: "Bob",
		Subject: "hello", Text: "the body of it",
	}
	plain, htmlBody := forwardBody(m, "alice")

	for name, body := range map[string]string{"plain": plain, "html": htmlBody} {
		if !strings.Contains(body, "the body of it") {
			t.Errorf("the %s copy does not contain the message", name)
		}
		if !strings.Contains(body, "/mail/unsubscribe?t=") {
			t.Errorf("the %s copy has no unsubscribe link", name)
		}
		if !strings.Contains(body, "Bob") {
			t.Errorf("the %s copy does not say who it is from", name)
		}
	}
}

// A message's text is somebody else's, so the HTML copy escapes it.
func TestTheHTMLCopyEscapesTheMessage(t *testing.T) {
	m := InboundMail{Owner: "alice", From: "bob", Text: `<script>alert(1)</script>`}
	_, htmlBody := forwardBody(m, "alice")
	if strings.Contains(htmlBody, "<script>") {
		t.Errorf("a message went into the forwarded HTML as markup: %q", htmlBody)
	}
}

// A reply to a forwarded message reaches whoever wrote it.
//
// Every one of these went out as From: no-reply@<domain> with nothing else, so
// hitting reply produced a mail that looked sent and reached nobody. The body's
// "read and reply at /inbox" was the only route that worked, which is asking
// somebody to open a website to answer an email — and it is the one thing a
// person will not do, so a forwarded message was effectively a dead end.
//
// The transport had carried a Reply-To since it was written; nothing exported
// it. Caught by writing a mail to two hundred people asking them to reply.
func TestAReplyToAForwardedMessageReachesTheSender(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	var got struct{ to, replyTo string }
	orig := app.EmailSender
	app.EmailSender = func(to, subject, plain, html, replyTo string) error {
		got.to, got.replyTo = to, replyTo
		return nil
	}
	t.Cleanup(func() { app.EmailSender = orig })

	// From outside: the address arrived with the message and is the answer.
	if err := auth.Create(&auth.Account{
		ID: "recipient", Email: "real@elsewhere.test", EmailVerified: true,
	}); err != nil {
		t.Skipf("no account store to forward against: %v", err)
	}
	forward(InboundMail{Owner: "recipient", From: "alice@outside.test",
		Subject: "hello", Text: "a message"})
	if got.replyTo != "alice@outside.test" {
		t.Errorf("Reply-To = %q, want the sender's address — without it, replying\n"+
			"to a forwarded message goes to no-reply@ and reaches nobody", got.replyTo)
	}

	// From a local account: the sender is a username, and a Reply-To of
	// "florian" is not one. Their address on this instance is, and a reply to
	// it is forwarded to them exactly like this one — the loop closing.
	got.replyTo = ""
	forward(InboundMail{Owner: "recipient", From: "florian",
		Subject: "hello", Text: "a message"})
	if got.replyTo != "florian@example.test" {
		t.Errorf("Reply-To = %q, want florian@example.test — a bare username in\n"+
			"Reply-To is an address a mail client cannot deliver to", got.replyTo)
	}
}
