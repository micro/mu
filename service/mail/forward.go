package mail

// Mail that arrives here also reaches you where you already read mail.
//
// # Why
//
// An account on this instance has an address, and mail sent to it lands in
// /inbox. Which is correct and is not the same as being told: somebody who
// signed up in March and has not opened the tab since has an inbox with things
// in it and no idea. Push notifications exist and cover the case where a device
// has been asked and said yes, which is a minority of accounts and none of the
// old ones.
//
// So a message to your username is copied to the address you signed up with, if
// you verified one. Not the content of a relationship this instance is party
// to — a copy of your own mail, sent to you, at your own address.
//
// # On by default, and one click off
//
// Default on, because an inbox nobody is told about is an inbox nobody reads,
// and every account that already exists is in exactly that position. The cost
// of being wrong about that is somebody getting a mail they did not want, which
// is why every one of them carries an unsubscribe link that needs no login and
// no explanation — and a List-Unsubscribe header, so a mail client's own
// unsubscribe button works too.
//
// # What is not forwarded
//
// Spam, obviously. Mail you sent yourself, or to your own agent, which is the
// product's main loop rather than something arriving. And anything to an
// account with no verified address, because there is nowhere to send it and an
// unverified address is a claim rather than a fact.
//
// # Why it is a subscriber and not a line in the delivery path
//
// event.MailReceived is published from the one place a message is stored, so
// every way mail can arrive reaches this by definition — SMTP, the local
// deliver path, an agent writing to somebody. A line in one of those paths
// would forward the mail that came in by that door and quietly not the rest,
// which is the bug the event exists to have already fixed once.

import (
	"fmt"
	"html"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/origin"
)

// StartForwarding copies arriving mail to verified addresses.
//
// Started by the server alongside the other subscribers. Silent and doing
// nothing on an instance with no way to send mail, which is most self-hosted
// ones: there is no address to forward from and nothing to complain about.
func StartForwarding() {
	go func() {
		sub := event.Subscribe(event.MailReceived)
		for e := range sub.Chan {
			m, ok := MessageFrom(e.Data)
			if !ok {
				continue
			}
			forward(m)
		}
	}()
}

// forward sends one arriving message on to the recipient's own address.
func forward(m InboundMail) {
	if app.EmailSender == nil {
		return // this instance cannot send mail at all
	}
	if m.Owner == "" {
		return
	}
	// Your own mail coming back to you is not mail arriving. Writing to your
	// own agent files a copy in your own inbox — see SendMessageTo — and
	// forwarding that would mail you a copy of what you just typed.
	if strings.EqualFold(strings.TrimSpace(m.From), strings.TrimSpace(m.Owner)) {
		return
	}

	acc, err := auth.GetAccount(m.Owner)
	if err != nil || acc == nil {
		return
	}
	if !acc.EmailVerified || strings.TrimSpace(acc.Email) == "" {
		return
	}
	if !ForwardingOn(acc.ID) {
		return
	}

	subject := strings.TrimSpace(m.Subject)
	if subject == "" {
		subject = "New message"
	}

	plain, htmlBody := forwardBody(m, acc.ID)
	if err := app.EmailSender(acc.Email, subject, plain, htmlBody); err != nil {
		app.Log("mail", "forwarding to %s failed: %v", acc.ID, err)
	}
}

// forwardBody is the copy, in both shapes a mail client might want.
//
// The message as it arrived, then a rule, then who it was from and where to
// read it, then the way out. The unsubscribe line is in the body of every one
// and not only in a header, because a header is a thing a client may or may not
// surface and somebody who wants this to stop should not have to find out which
// client they have.
func forwardBody(m InboundMail, accountID string) (plain, htmlBody string) {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		text = strings.TrimSpace(stripHTMLTags(m.Body))
	}
	from := strings.TrimSpace(m.FromName)
	if from == "" {
		from = strings.TrimSpace(m.From)
	}
	if from == "" {
		from = "someone"
	}

	base := origin.Self()
	stop := unsubscribeURL(accountID)

	plain = fmt.Sprintf(`%s

—
From %s · read and reply at %s/inbox

You are getting this because mail sent to your Mu address is copied to you here.
Stop these emails: %s`, text, from, base, stop)

	htmlBody = fmt.Sprintf(`<div style="font:14px/1.6 -apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;color:#222">
<div style="white-space:pre-wrap">%s</div>
<hr style="border:0;border-top:1px solid #e5e5e5;margin:24px 0 12px">
<p style="color:#666;font-size:13px;margin:0 0 6px">From %s · <a href="%s/inbox" style="color:#333">read and reply</a></p>
<p style="color:#999;font-size:12px;margin:0">You are getting this because mail sent to your Mu address is copied to you here.
<a href="%s" style="color:#999">Stop these emails</a>.</p>
</div>`,
		html.EscapeString(text), html.EscapeString(from),
		html.EscapeString(base), html.EscapeString(stop))

	return plain, htmlBody
}

// unsubscribeURL is the one-click way out.
func unsubscribeURL(accountID string) string {
	return origin.Self() + "/mail/unsubscribe?t=" + UnsubscribeToken(accountID)
}
