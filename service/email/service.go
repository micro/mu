package email

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for email.
type Server struct{}

var Spec = service.Spec{
	Name:        "email",
	Label:       "Email",
	Handler:     new(Server),
	Description: "Send a real email to somebody outside this instance",
	Page:        "/email",
	Icon:        "email.svg",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		// AccountOnly, not merely priced, and for the reason sms gives: every
		// other paid tool can be reached by an anonymous caller paying over
		// x402, and that is right for a search, where the money covers the cost
		// and nobody else is affected. Sending is not like that. An anonymous
		// spammer paying four pence a message is still a spammer, and what they
		// spend is the sending domain's reputation, which belongs to everybody
		// on this instance and cannot be topped up.
		//
		// A flat Cost, unlike sms: an email is one price however long it is, so
		// the gateway charges it once and this service does not charge itself.
		// Not an alias for mail_email, and that is the distinction worth
		// holding on to. Both send a real email; they send it as different
		// people, for different reasons.
		//
		//   mail_email  goes out as you@MAIL_DOMAIN, over this instance's own
		//               SMTP with DKIM. The address is the point: it is the one
		//               with an inbox behind it, the one an agent receives at
		//               as you+tag@, and the one a reply has to come from for a
		//               thread to hold together.
		//
		//   email_send  goes out from the authenticated sending domain, which
		//               carries nothing else and has no inbox. Deliverability
		//               and volume are the point, and a reply is pointed back
		//               at the mailbox above rather than answered here.
		//
		// Collapsing them would cost one or the other: send everything from the
		// bulk domain and a per-agent mailbox stops being two-way, send
		// everything from the root domain and agent mail is staking the
		// deliverability of password resets.
		"Send": {
			Cost:        quota.OpExternalEmail,
			AccountOnly: true,
			Destructive: true,
			Doc: "Send a real email to an outside address, from this instance's " +
				"authenticated sending domain. Takes an address, a subject and a body; " +
				"resolve a name with contacts_find first. It really is delivered — there " +
				"is no draft state to undo from. To send as your own address on this " +
				"instance, so a reply comes back to your inbox, use mail_email; for " +
				"somebody on this instance use mail_send",
		},
		"History": {
			AccountOnly: true,
			Doc:         "The emails this account has sent, newest first",
		},
		"Sender": {
			AccountOnly: true,
			Doc: "The address email is sent from, where replies to it go, and how many " +
				"messages are left today",
		},
	},
}

// ── Send ────────────────────────────────────────────────────────

type SendRequest struct {
	To      string `json:"to" required:"true" description:"Recipient email address. Resolve a name with contacts_find first"`
	Subject string `json:"subject" required:"true" description:"Message subject"`
	Body    string `json:"body" required:"true" description:"Message body, plain text"`
}

type SendResponse struct {
	Result string `json:"result" description:"Confirmation, with the address it was sent from"`
}

// Send delivers one email off this instance.
// @example {"to": "sarah@example.com", "subject": "Hello", "body": "..."}
func (Server) Send(ctx context.Context, req *SendRequest, rsp *SendResponse) error {
	who := service.AccountFrom(ctx)
	msg, err := Send(who, req.To, req.Subject, req.Body)
	if err != nil {
		return err
	}
	rsp.Result = fmt.Sprintf("Sent to %s from %s. Replies come back to %s. %d left today.",
		msg.To, SenderFor(who), ReplyFor(who), LeftToday(who))
	return nil
}

// ── History ─────────────────────────────────────────────────────

type HistoryRequest struct {
	Limit int `json:"limit" description:"Max messages (default 20)"`
}

type HistoryResponse struct {
	Text string `json:"text" description:"What has been sent: recipient, subject and when"`
}

// History lists what this account has sent.
// @example {}
func (Server) History(ctx context.Context, req *HistoryRequest, rsp *HistoryResponse) error {
	who := service.AccountFrom(ctx)
	msgs := History(who, req.Limit)
	if len(msgs) == 0 {
		rsp.Text = "Nothing sent yet."
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sent (%d):\n", len(msgs))
	for _, m := range msgs {
		fmt.Fprintf(&b, "- %s — %s (%s)\n", m.To, clip(m.Subject, 80),
			m.Sent.Format("2 Jan 15:04"))
	}
	rsp.Text = b.String()
	return nil
}

// ── Sender ──────────────────────────────────────────────────────

type SenderRequest struct{}

type SenderResponse struct {
	From    string `json:"from" description:"The address email is sent from"`
	ReplyTo string `json:"reply_to" description:"Where replies to it arrive"`
	Left    int    `json:"left" description:"How many more may be sent today"`
	Text    string `json:"text" description:"The same, as a sentence"`
}

// Sender says what this account sends as, and how much is left.
//
// The reply address is in the answer rather than left implicit, because it is
// the one thing about sending here that is not obvious: mail goes out from a
// domain with no inbox, and answers arrive somewhere else. An agent composing a
// message that says "get back to me" should be able to read where that is.
// @example {}
func (Server) Sender(ctx context.Context, _ *SenderRequest, rsp *SenderResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to send email")
	}
	if !Configured() {
		return fmt.Errorf("this instance has no sending domain configured")
	}
	rsp.From, rsp.ReplyTo, rsp.Left = SenderFor(who), ReplyFor(who), LeftToday(who)
	rsp.Text = fmt.Sprintf("Email is sent from %s and replies arrive at %s. %d of %d left today.",
		rsp.From, rsp.ReplyTo, rsp.Left, LimitFor(who))
	return nil
}

// clip trims to n runes, safely.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
