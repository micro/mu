package email

import (
	"context"
	"fmt"
	"strings"
	"time"

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
			Needs:       service.Account,
			Destructive: true,
			Doc: "Send a real email to an outside address, from this instance's " +
				"authenticated sending domain. Takes an address, a subject and a body; " +
				"resolve a name with contacts_find first. It really is delivered — there " +
				"is no draft state to undo from. To send as your own address on this " +
				"instance, so a reply comes back to your inbox, use mail_email; for " +
				"somebody on this instance use mail_send",
		},
		"History": {
			Needs: service.Account,
			Doc: "The emails this account has sent, newest first, and what became of " +
				"each one — delivered, undelivered, failed, or still going. Asks the " +
				"carrier about anything still in flight, so it is current when read",
		},
		"Sender": {
			Needs: service.Account,
			Doc: "The address email is sent from, where replies to it go, which addresses " +
				"this account has proved are its own, and how many messages are left today",
		},
		// No Cost, unlike Send, and the reason is that only one of its two steps
		// sends anything: asking for a code puts an email on the wire, checking
		// the code back does not. An endpoint charge cannot tell them apart, so
		// the service charges itself — see verify.go.
		//
		// How many people you may verify is the plan's emails-per-day and
		// nothing else. That is the point: it is the same allowance sending
		// uses, so it is a tier to buy your way up rather than a second meter
		// invented for this tool.
		"Verify": {
			Needs: service.Account,
			Doc: "Check that somebody can read an email address — the verification a " +
				"signup form runs. Call it with an address to have a six-digit code " +
				"emailed there, then again with the code the person typed back: the answer " +
				"says approved or why not, so a wrong code is an answer rather than an " +
				"error. Pass app to put your product's name in the message rather than " +
				"this instance's. Costs one send and counts against the daily email " +
				"allowance. Pass mine when the address is your own and you want mail from " +
				"it recognised here",
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
	rsp.Result = fmt.Sprintf("Sent to %s from %s, carried by %s. %s.",
		msg.To, SenderFor(who), Provider(), Allowance(who))
	return nil
}

// ── History ─────────────────────────────────────────────────────

type HistoryRequest struct {
	Limit int `json:"limit" description:"Max messages (default 20)"`
}

type HistoryResponse struct {
	Text string `json:"text" description:"What has been sent: recipient, subject, what became of it, and when"`
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
		fmt.Fprintf(&b, "- %s — %s — %s (%s)", m.To, clip(m.Subject, 80),
			m.Status(), m.Sent.Format("2 Jan 15:04"))
		if !m.OK() {
			fmt.Fprintf(&b, ": %s", m.Error)
		}
		b.WriteString("\n")
	}
	rsp.Text = b.String()
	return nil
}

// ── Sender ──────────────────────────────────────────────────────

type SenderRequest struct{}

type SenderResponse struct {
	From    string   `json:"from" description:"The address email is sent from"`
	ReplyTo string   `json:"reply_to" description:"Where replies to it arrive"`
	Yours   []string `json:"yours" description:"Addresses this account has proved are its own. Tell somebody to write to one of these if you need an answer"`
	Left    int      `json:"left" description:"How many more may be sent today"`
	Text    string   `json:"text" description:"The same, as a sentence"`
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
	rsp.From, rsp.ReplyTo = SenderFor(who), Answers(who)
	rsp.Yours = Addresses(who)
	rsp.Left, _ = LeftToday(who)
	rsp.Text = fmt.Sprintf("Email is sent from %s and replies arrive at %s. %s.",
		rsp.From, rsp.ReplyTo, Allowance(who))
	if len(rsp.Yours) > 0 {
		rsp.Text += fmt.Sprintf(" To be written back to, ask for an answer at %s.",
			strings.Join(rsp.Yours, " or "))
	}
	return nil
}

// ── Verify ──────────────────────────────────────────────────────

type VerifyRequest struct {
	Address string `json:"address" required:"true" description:"The address to verify — usually one of your users', not your own"`
	Code    string `json:"code,omitempty" description:"The code the person typed back. Omit to have one emailed to them"`
	App     string `json:"app,omitempty" description:"Your product's name, to put in the message. Defaults to this account's name"`
	Mine    bool   `json:"mine,omitempty" description:"Set only when the address is your own: on approval it is recorded here, so mail you send from it reaches your agent instead of a spam folder"`
}

type VerifyResponse struct {
	Address  string `json:"address" description:"The address, in the spelling everything here uses"`
	Status   string `json:"status" description:"pending — a code has been emailed; approved — the code was right; incorrect, expired, exhausted — it was not; none — no code is waiting for that address"`
	Approved bool   `json:"approved" description:"Whether the person proved they can read the address"`
	Left     int    `json:"left" description:"Guesses this code has left, while it is pending"`
	Expires  string `json:"expires,omitempty" description:"When the code stops working"`
	Result   string `json:"result" description:"The same, as a sentence you can show somebody"`
}

// Verify checks that somebody can read an address, in two steps.
//
// The check a signup form runs, and the caller is almost always asking about
// somebody else's address rather than their own — so nothing here records
// anything against the caller's account unless they ask for it with mine. What
// the answer means is theirs to decide and theirs to store; this says whether
// the code matched.
//
// A wrong code comes back as a status rather than an error, because it is the
// ordinary outcome of asking somebody to type six digits.
// @example {"address": "user@example.com", "app": "Acme"}
func (Server) Verify(ctx context.Context, req *VerifyRequest, rsp *VerifyResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to verify an address")
	}

	var (
		v   Verification
		err error
	)
	if code := strings.TrimSpace(req.Code); code != "" {
		v, err = Check(who, req.Address, code)
	} else {
		v, err = StartVerify(who, req.Address, req.App)
	}
	if err != nil {
		return err
	}

	if v.OK() && req.Mine {
		if err := Claim(who, v.Address); err != nil {
			return err
		}
	}

	rsp.Address, rsp.Status, rsp.Approved = v.Address, v.Status, v.OK()
	rsp.Left, rsp.Result = v.Left, v.Says()
	if !v.Expires.IsZero() {
		rsp.Expires = v.Expires.Format(time.RFC3339)
	}
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
