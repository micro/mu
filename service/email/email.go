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
// It also fixes what was wrong with sending from mail. Outbound went under
// MAIL_DOMAIN, the domain the website is on, so agent mail and password resets
// shared a reputation and there was no way to separate them after the fact.
// This sends from its own authenticated domain, which carries nothing else.
//
// Two rules that are the service rather than decoration:
//
//   - **Replies go somewhere.** The sending domain is authenticated for sending
//     and has no inbox. A reply to it bounces, so every message carries a
//     Reply-To pointing at the sender's address on the instance's mail domain,
//     which mail does answer. Without it, sending is a broadcast and the sender
//     never learns anyone replied.
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
	"mu/internal/twilio"
	"mu/internal/settings"
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

	// Ours first. A provider is an option here, not a dependency: the
	// instance already runs an SMTP server with DKIM, and adding a second
	// credential to send from a subdomain of a domain we already sign for
	// would be paying a toll to cross our own bridge.
	var providerID string
	switch {
	case twilio.EmailConfigured():
		providerID, err = twilio.SendEmail(twilio.Email{
			From:     SenderFor(owner),
			FromName: acc.Name,
			ReplyTo:  ReplyFor(owner),
			To:       to,
			Subject:  subject,
			Text:     body,
			HTML:     asHTML(body),
		})
	default:
		providerID, err = SendVia(acc.Name, SenderFor(owner), ReplyFor(owner),
			to, subject, body, asHTML(body))
	}
	if err != nil {
		return nil, err
	}

	rec, err := userdb.Create(ns, owner, sent, map[string]interface{}{
		"to": to, "subject": subject, "body": body,
		"provider": providerID,
		"sent":     time.Now().Format(time.RFC3339),
	}, false)
	if err != nil {
		// It has gone. Failing the call now would tell the caller to send it
		// again, which is the one outcome worse than losing the record.
		app.Log("email", "sent for %s but not recorded: %v", owner, err)
		return &Sent{To: to, Subject: subject, Body: body, Sent: time.Now(), Provider: providerID}, nil
	}
	return fromRecord(*rec), nil
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
