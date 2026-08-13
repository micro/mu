// Package email is email that leaves this instance: send one, and see what you
// have sent.
//
// It is a sibling of sms and whatsapp rather than a second half of mail, and
// the split is the same one the pricing argument turns on. **mail is a
// mailbox** — an inbox on this instance, an address that reaches you, a message
// to somebody else here. Nothing it does is visible outside the building.
// **email is a channel** — it puts a message in front of a person who did not
// ask for it, at an address we do not control, and the cost of getting that
// wrong is not the credits. It is a domain's reputation, which is shared by
// everybody here and cannot be topped up.
//
// That is exactly what sms and whatsapp are, and the three now read the same
// way: a provider underneath in internal/, a per-account daily cap, an account
// required, and a history of what went out. A caller reasoning about "what can
// this thing send on my behalf" finds three services with one shape instead of
// two services and a method hidden inside a third.
//
// It sends from its own domain, and that is the decision this service exists
// to hold. Outbound used to go under MAIL_DOMAIN, the domain the website is on.
//
// The reason is not that agent mail is untidy, it is who writes it. **Anyone
// can sign up here and send.** What leaves this instance is user-generated
// content going to strangers under one shared identity, and a domain's
// reputation is the one thing a single bad account can spend on everybody
// else's behalf — including on the mail that has to arrive, which is the
// password reset and the receipt. A separate sending domain is how that stays
// somebody else's problem.
//
// The isolation is real and it is partial: receivers treat a subdomain as
// mostly its own sender, but they connect related identities, so a bad enough
// month bleeds upward and a poor root reputation is inherited downward. It is a
// blast radius, not a wall. It is still worth having, because the alternative
// is no separation at all.
//
// **Replies to it bounce, and that is accepted for now.** Twilio will not carry
// a Reply-To — no field, and the header is refused — so a message is answered
// at its From, and the sending domain has no inbox. Fixing it means an MX
// record and teaching the SMTP server to accept a second domain and route it,
// which is inbound work on a service that currently exists to send. The
// alternative, sending as MAIL_DOMAIN, is the thing above that must not happen.
//
// Two rules that are the service rather than decoration:
//
//   - **A daily cap per account**, which a plan raises. The price is the first
//     control and the cap is the second, because they fail differently: a price
//     stops somebody who has to pay, a cap stops a loop that found a way not
//     to. External email had no cap of any kind before this, which on a product
//     that has already been abused this way is not a missing feature, it is an
//     exposure. The number lives in quota.json beside the price.
package email

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/settings"
	"mu/internal/twilio"
	"mu/internal/userdb"
)

const (
	ns   = "email"
	sent = "sent"
)

// Sent is one message this account put on the wire.
type Sent struct {
	ID      string    `json:"id"`
	To      string    `json:"to"`
	Subject string    `json:"subject"`
	Body    string    `json:"body"`
	Sent    time.Time `json:"sent"`
	// Provider is the id the provider gave it, which is the handle for asking
	// what happened to it afterwards.
	Provider string `json:"provider,omitempty"`
	// Delivery is what became of it, once the carrier has been asked:
	// delivered, undelivered, failed, sending, and so on. Empty until asked, or
	// where there is nothing to ask.
	//
	// Separate from Error, which is whether it was accepted at all. The two
	// answer different questions and a message can pass the first and fail the
	// second — accepted, sent, then refused by the receiving server — which is
	// exactly the case that made "sent" a lie.
	Delivery string `json:"delivery,omitempty"`
	// Checked is when the carrier was last asked.
	Checked time.Time `json:"checked,omitempty"`
	// Error is what went wrong, empty when the carrier accepted it.
	//
	// A failed send used to leave no trace at all: the error went back to
	// whoever called and the history showed only what worked, so the page could
	// not answer the first question anybody has after setting a sending domain
	// up — did it go? A send that was refused is the part of the history worth
	// keeping, because it is the part you act on.
	Error string `json:"error,omitempty"`
}

// OK reports whether the carrier took it.
func (s *Sent) OK() bool { return s != nil && s.Error == "" }

// Status is the outcome in a word, for a page or a tool.
//
// What the carrier last said, when it has said anything. "Accepted" is the
// honest word for the gap in between: Twilio takes a message and answers before
// anything has been delivered, so a history that called that "sent" was
// reporting a receipt for a promise.
func (s *Sent) Status() string {
	switch {
	case s == nil:
		return ""
	case s.Error != "":
		return "failed"
	case s.Delivery != "":
		return s.Delivery
	}
	return "accepted"
}

// Settled reports whether this record's outcome can still change — a message
// still in flight, or one nobody has asked about yet.
func (s *Sent) Settled() bool {
	if s == nil {
		return true
	}
	if s.Error != "" {
		return true // it never left
	}
	if s.Provider == "" {
		return true // nothing to ask about
	}
	if time.Since(s.Sent) > twilio.EmailRetention {
		// Past retention the carrier has forgotten. Not knowing is the final
		// answer, and asking again would only produce the same refusal.
		return true
	}
	return twilio.Settled(s.Delivery)
}

// SendVia is this instance's own SMTP sender, filled in by
// internal/server/hooks.go from the mail service. The fallback for an instance
// with no Twilio account.
//
// It is a hook rather than an import because email and mail are two services
// and services do not import each other — see hooks.go, which is the ledger of
// exactly these.
//
// It exists because a provider is not required. This instance already runs an
// SMTP server with DKIM; what was wrong with sending from mail was never the
// transport, it was the domain — agent mail going out under MAIL_DOMAIN, the
// one the website is on, staking the deliverability of password resets on
// whatever an agent decides to send. Sending from a subdomain of our own fixes
// that with no third party and no second credential: DMARC alignment is
// relaxed by default, so a signature for micro.mu covers a From on
// email.micro.mu, which is the same organisational domain.
//
// Twilio carries it when there is an account, over the same credentials the
// texts use — there is no second key for email. This is what sends when there
// is not.
var SendVia func(displayName, from, replyTo, to, subject, plain, html string) (string, error)

// Configured reports whether this instance can send at all: a domain to send
// from, and something to send with.
func Configured() bool {
	if Domain() == "" {
		return false
	}
	return twilio.EmailConfigured() || SendVia != nil
}

// Provider names what will carry the message, for the page.
func Provider() string {
	if twilio.EmailConfigured() {
		return "Twilio"
	}
	if SendVia != nil {
		return "this instance's own SMTP"
	}
	return ""
}

// Domain is the domain messages are sent from.
//
// Read here rather than from the provider, because the provider is optional and
// the domain is not. It is the whole point of this service: agent mail leaves
// under a name of its own, whoever carries it.
func Domain() string {
	return strings.ToLower(strings.TrimSpace(settings.Get("EMAIL_DOMAIN")))
}

// ReplyDomain is where replies are pointed, which must be a domain with an
// inbox behind it.
//
// Its own setting rather than a read of the mail service, because email must
// not import mail — they are two services and services do not import each
// other. It is also a real choice: an operator relaying replies elsewhere sets
// this and nothing else changes.
func ReplyDomain() string {
	if d := strings.TrimSpace(settings.Get("EMAIL_REPLY_DOMAIN")); d != "" {
		return strings.ToLower(d)
	}
	return strings.ToLower(strings.TrimSpace(settings.Get("MAIL_DOMAIN")))
}

// SenderFor is the address an account's mail goes out as.
func SenderFor(owner string) string {
	if Domain() == "" {
		return ""
	}
	return localPart(owner) + "@" + Domain()
}

// Answers is where a reply to this account's mail will actually arrive.
//
// Not the same question as ReplyFor, and the difference is the carrier's.
// Twilio will not carry a Reply-To — there is no field for it and the header is
// refused — so a message sent that way is answered at its From, which on the
// sending domain is an address with no inbox behind it. Over this instance's
// own SMTP the header is ours to set, and answers reach the mail domain.
//
// A caller that needs an answer should say where in the body until the sending
// domain can receive. See the package comment for why the obvious fix — send as
// MAIL_DOMAIN — is the one thing this service exists to avoid.
func Answers(owner string) string {
	if twilio.EmailConfigured() {
		return SenderFor(owner)
	}
	return ReplyFor(owner)
}

// ReplyFor is where answers to that account should go, or "" when this instance
// has no inbox to point at.
func ReplyFor(owner string) string {
	if ReplyDomain() == "" {
		return ""
	}
	return localPart(owner) + "@" + ReplyDomain()
}

// localPart makes an address-safe name from an account.
//
// The account's own id, which is its username — asim becomes
// asim@<sending domain>, and asim@<mail domain> is where replies go. It used to
// prefer the display name, so an account called "Asim Aslam" sent as
// asimaslam@ and was answered at asimaslam@, which is not a mailbox. mail keys
// every address on the id for the same reason.
func localPart(owner string) string {
	name := owner
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "agent"
	}
	return b.String()
}

// ── The daily cap ───────────────────────────────────────────────

// The cap itself is not here. It is quota.json, on the same line as the price,
// under limit — because "what does this cost" and "how much of it may somebody
// do" are the same kind of decision made by the same person, and keeping them
// apart is how sms came to own SMS_DAILY_LIMIT while this was about to invent
// EMAIL_DAILY_LIMIT: two names for one idea in two packages, and an operator
// looking for either had to know which package to read.
//
// What a plan raises it to is wallet's business, and quota asks it through a
// hook. Everything below is a view of that one answer.

// LimitFor is how many this account may send today.
func LimitFor(owner string) int { return quota.LimitFor(owner, quota.OpExternalEmail) }

// SentToday is how many it has sent.
func SentToday(owner string) int { return quota.UsedToday(owner, quota.OpExternalEmail) }

// Allowance says how many more this account may send today, in words, because
// the two states a caller must not confuse — none left, and no limit — read the
// same if either is printed as a number.
func Allowance(owner string) string {
	left, capped := LeftToday(owner)
	if !capped {
		return "no daily limit"
	}
	return fmt.Sprintf("%d of %d left today", left, LimitFor(owner))
}

// LeftToday is how many more it may send, and whether there is a cap at all.
//
// The second return is not decoration. An admin, and the instance's own agent,
// are uncapped — LimitFor answers quota.NoLimit for them, which is -1, and a
// page that printed it read "0 of -1 left today". A caller that cannot tell
// "none left" from "no limit" will always render one of them as the other.
func LeftToday(owner string) (int, bool) {
	return quota.LeftToday(owner, quota.OpExternalEmail)
}

// ── Sending ─────────────────────────────────────────────────────

// Send puts one message on the wire and records it.
//
// The charge is not here. Send declares a flat Cost on its endpoint and the
// gateway applies it once, after this returns without error — which is what
// makes a failed send free. sms charges itself only because a text is priced
// per segment and the gateway charges a flat price.
func Send(owner, to, subject, body string) (*Sent, error) {
	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	if owner == "" {
		return nil, fmt.Errorf("sign in to send email")
	}
	if !Configured() {
		return nil, fmt.Errorf("this instance has no sending domain configured")
	}
	if to == "" || subject == "" || strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("a recipient, a subject and a body are all required")
	}
	if !strings.Contains(to, "@") {
		return nil, fmt.Errorf("%s is not an email address — for somebody on this instance use mail_send", to)
	}

	// The gateway checks this too, before the handler runs. It is here as well
	// because the page reaches past the endpoint into this function, and a cap
	// that only one of the two doors honours is not a cap.
	if over, why := quota.OverLimit(owner, quota.OpExternalEmail); over {
		return nil, fmt.Errorf("%s", why)
	}

	// The same message to the same address twice running is refused, for the
	// reason sms refuses it: a price stops somebody who has to pay and does
	// nothing about a loop, and a loop is what a runaway agent is.
	if recent := History(owner, 1); len(recent) == 1 &&
		recent[0].To == to && recent[0].Subject == subject &&
		time.Since(recent[0].Sent) < time.Minute {
		return nil, fmt.Errorf("that exact message went to %s a moment ago", to)
	}

	acc, err := auth.GetAccount(owner)
	if err != nil {
		return nil, fmt.Errorf("account not found")
	}

	providerID, err := deliver(owner, acc.Name, to, subject, body)

	// Recorded either way, and before the error is returned. What went wrong is
	// the half of the history worth having: a message that was refused is the
	// one you have to do something about, and it used to disappear.
	failed := ""
	if err != nil {
		failed = err.Error()
	}
	rec := record(owner, to, subject, body, providerID, failed)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// deliver puts a message on the wire, whoever is carrying it, and returns the
// carrier's id for it.
//
// The one place the transport is chosen. Ours first: a provider is an option
// here, not a dependency — the instance already runs an SMTP server with DKIM,
// and adding a second credential to send from a subdomain of a domain we
// already sign for would be paying a toll to cross our own bridge.
//
// Separated from Send because verification sends an email too, and it must go
// out the same way. What is *not* here is every rule about sending — the caps,
// the charge, the duplicate check, the history. Those are Send's, and a
// verification code is the one message this service sends that is not somebody
// writing to somebody else.
func deliver(owner, fromName, to, subject, body string) (string, error) {
	if twilio.EmailConfigured() {
		return twilio.SendEmail(twilio.Email{
			From:     SenderFor(owner),
			FromName: fromName,
			ReplyTo:  ReplyFor(owner),
			To:       to,
			Subject:  subject,
			Text:     body,
			HTML:     asHTML(body),
		})
	}
	return SendVia(fromName, SenderFor(owner), ReplyFor(owner),
		to, subject, body, asHTML(body))
}

// record files one attempt.
func record(owner, to, subject, body, providerID, failed string) *Sent {
	fields := map[string]interface{}{
		"to": to, "subject": subject, "body": body,
		"provider": providerID,
		"sent":     time.Now().Format(time.RFC3339),
	}
	if failed != "" {
		fields["error"] = failed
	}
	rec, err := userdb.Create(ns, owner, sent, fields, false)
	if err != nil {
		// It may already have gone. Failing now would tell the caller to send it
		// again, which is the one outcome worse than losing the record.
		app.Log("email", "sent for %s but not recorded: %v", owner, err)
		return &Sent{To: to, Subject: subject, Body: body, Sent: time.Now(),
			Provider: providerID, Error: failed}
	}
	return fromRecord(*rec)
}

// Refresh asks the carrier what became of anything still in flight, and writes
// the answer down.
//
// Called when somebody looks, rather than from a loop. A send settles in
// seconds to minutes and the only person who cares is the one asking, so a
// background poller would be work done on the chance that somebody might — and
// a loop that runs whether or not anybody is watching is exactly the thing that
// spends a provider's rate limit on nothing.
//
// Bounded: only records that can still change, only the recent ones, and never
// more than a handful per look.
func Refresh(owner string, msgs []*Sent) {
	if !twilio.EmailConfigured() {
		return
	}
	const most = 10
	asked := 0
	for _, m := range msgs {
		if asked >= most {
			return
		}
		if m.Settled() || time.Since(m.Checked) < 10*time.Second {
			continue
		}
		asked++
		op, err := twilio.EmailStatus(m.Provider)
		if err != nil {
			app.Log("email", "status for %s: %v", m.Provider, err)
			continue
		}
		outcome := op.Outcome()
		if outcome == "" {
			continue
		}
		m.Delivery, m.Checked = outcome, time.Now()
		// Update replaces the record's data, so the rest of it has to go back
		// with the two new fields — a partial update here would lose the
		// message.
		if _, err := userdb.Update(ns, owner, sent, m.ID, map[string]interface{}{
			"to": m.To, "subject": m.Subject, "body": m.Body,
			"provider": m.Provider,
			"sent":     m.Sent.Format(time.RFC3339),
			"delivery": outcome,
			"checked":  m.Checked.Format(time.RFC3339),
		}, false); err != nil {
			app.Log("email", "recording delivery for %s: %v", m.ID, err)
		}
	}
}

// History is what this account has sent, newest first.
func History(owner string, limit int) []*Sent {
	if limit <= 0 {
		limit = 20
	}
	recs, err := userdb.List(ns, owner, sent, "mine", nil, "", "", limit)
	if err != nil {
		return nil
	}
	out := make([]*Sent, 0, len(recs))
	for _, r := range recs {
		if m := fromRecord(r); m != nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sent.After(out[j].Sent) })
	// Asked when somebody looks. History is the one place that answers "what
	// happened to my mail", so it is the place that has to be current — a list
	// that says "accepted" for ever is a receipt for a promise.
	Refresh(owner, out)
	return out
}

func fromRecord(rec userdb.Record) *Sent {
	str := func(k string) string {
		if v, ok := rec.Data[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	m := &Sent{
		ID:       rec.ID,
		To:       str("to"),
		Subject:  str("subject"),
		Body:     str("body"),
		Provider: str("provider"),
		Delivery: str("delivery"),
		Error:    str("error"),
	}
	if t, err := time.Parse(time.RFC3339, str("checked")); err == nil {
		m.Checked = t
	}
	if t, err := time.Parse(time.RFC3339, str("sent")); err == nil {
		m.Sent = t
	}
	return m
}

// asHTML is the plain body as simple HTML, because a message with only one part
// is scored worse by every filter that looks at it.
func asHTML(body string) string {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(body)
	var b strings.Builder
	for _, para := range strings.Split(esc, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		b.WriteString("<p>" + strings.ReplaceAll(para, "\n", "<br>") + "</p>")
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String()
}

// DeleteAll removes everything email holds for an owner (account deletion).
//
// All of it goes. Unlike sms, where an opt-out belongs to the number that asked
// to be left alone rather than to the account that annoyed them, nothing here
// is anybody else's: a record of what you sent is yours.
func DeleteAll(owner string) {
	if owner == "" {
		return
	}
	if n, err := userdb.DeleteOwner(ns, owner); err != nil {
		app.Log("email", "deleting %s's sent mail: %v", owner, err)
	} else if n > 0 {
		app.Log("email", "deleted %d sent records for %s", n, owner)
	}
}

// Missing names what an operator has to set before anything can be sent, in the
// words of the settings themselves.
func Missing() []string {
	var out []string
	if !twilio.EmailConfigured() && SendVia == nil {
		out = append(out, "a way to send — Twilio credentials (the same ones SMS uses), "+
			"or this instance's own SMTP with MAIL_DOMAIN and a DKIM key")
	}
	if Domain() == "" {
		out = append(out, "EMAIL_DOMAIN — the authenticated sending domain, e.g. email.example.com")
	}
	if ReplyDomain() == "" {
		out = append(out, "EMAIL_REPLY_DOMAIN — where replies should go, which needs an inbox behind it")
	}
	return out
}
