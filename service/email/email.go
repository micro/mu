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
//   - **A daily cap per account**, tighter for an account nobody has vouched
//     for. The price is the first control and the cap is the second, because
//     they fail differently: a price stops somebody who has to pay, a cap stops
//     a loop that found a way not to. External email had no cap of any kind
//     before this, which on a product that has already been abused this way is
//     not a missing feature, it is an exposure.
package email

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/sendgrid"
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

// Configured reports whether this instance can send at all.
func Configured() bool { return sendgrid.Configured() }

// Domain is the authenticated domain messages are sent from.
func Domain() string { return sendgrid.Domain() }

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
func localPart(owner string) string {
	acc, err := auth.GetAccount(owner)
	name := owner
	if err == nil && acc.Name != "" {
		name = acc.Name
	}
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

// DailyLimit is how many messages one account may send in a day. Zero turns
// sending off instance-wide, which is the kill switch — the same setting rather
// than a second one, because an operator reaching for it is in a hurry.
func DailyLimit() int { return limitSetting("EMAIL_DAILY_LIMIT", 50) }

// limitSetting reads a cap, and unlike app.EnvInt it believes a zero. EnvInt
// treats 0 as "not set" and hands back the default, which is right for a size
// and wrong for a limit: an operator typing zero to stop the mail would have
// been told fifty.
func limitSetting(key string, def int) int {
	v := strings.TrimSpace(settings.Get(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// LimitFor is the cap for one account. An account made in the last day gets a
// much smaller one: signing up takes a minute, so the cap on a fresh account is
// the only thing between a script and a mailing list.
func LimitFor(owner string) int {
	limit := DailyLimit()
	if limit == 0 {
		return 0
	}
	if auth.IsNewAccount(owner) {
		if n := limitSetting("EMAIL_NEW_ACCOUNT_LIMIT", 5); n < limit {
			return n
		}
	}
	return limit
}

// SentToday is how many this account has sent since midnight.
func SentToday(owner string) int {
	n := 0
	midnight := time.Now().Truncate(24 * time.Hour)
	for _, m := range History(owner, 500) {
		if m.Sent.After(midnight) {
			n++
		}
	}
	return n
}

// LeftToday is how many more this account may send.
func LeftToday(owner string) int {
	left := LimitFor(owner) - SentToday(owner)
	if left < 0 {
		return 0
	}
	return left
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

	if limit := LimitFor(owner); limit == 0 {
		return nil, fmt.Errorf("sending is turned off on this instance")
	} else if SentToday(owner) >= limit {
		return nil, fmt.Errorf("that is %d emails today, which is this account's limit — it resets at midnight", limit)
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

	res, err := sendgrid.Send(sendgrid.Message{
		From:     SenderFor(owner),
		FromName: acc.Name,
		ReplyTo:  ReplyFor(owner),
		To:       to,
		Subject:  subject,
		Plain:    body,
		HTML:     asHTML(body),
	})
	if err != nil {
		return nil, err
	}

	rec, err := userdb.Create(ns, owner, sent, map[string]interface{}{
		"to": to, "subject": subject, "body": body,
		"provider": res.ID,
		"sent":     time.Now().Format(time.RFC3339),
	}, false)
	if err != nil {
		// It has gone. Failing the call now would tell the caller to send it
		// again, which is the one outcome worse than losing the record.
		app.Log("email", "sent for %s but not recorded: %v", owner, err)
		return &Sent{To: to, Subject: subject, Body: body, Sent: time.Now(), Provider: res.ID}, nil
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
	if sendgrid.APIKey() == "" {
		out = append(out, "SENDGRID_API_KEY — the provider credential")
	}
	if Domain() == "" {
		out = append(out, "EMAIL_DOMAIN — the authenticated sending domain, e.g. email.example.com")
	}
	if ReplyDomain() == "" {
		out = append(out, "EMAIL_REPLY_DOMAIN — where replies should go, which needs an inbox behind it")
	}
	return out
}
