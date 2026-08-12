// Package sendgrid is the provider underneath email that leaves this instance.
//
// It holds the credentials, the POST that hands a message over, and nothing
// else. Who may send, to whom, how often and under whose name belongs to the
// service asking — the same division internal/twilio keeps for texts and
// WhatsApp.
//
// Why a provider at all, when this repo already runs an SMTP server with DKIM.
// The SMTP server is how mail *arrives*, and it sends too — under MAIL_DOMAIN,
// which is the domain the website is on. Sending agent mail from there stakes
// the deliverability of password resets and receipts on whatever an agent
// decides to send, and there is no way to separate the two once they share a
// domain: a spam complaint is counted against the domain, not the message.
//
// A separate authenticated subdomain is the separation. It has its own SPF,
// DKIM and reputation, it carries only what agents send, and if it burns it
// burns alone. Doing that ourselves would mean a second SMTP setup, a second
// set of DNS records, warm-up, and bounce and complaint handling we would have
// to write; a provider that does nothing else does it better, and it is the
// same vendor relationship SMS and WhatsApp already run on.
//
// No dependency for it. The send is a JSON POST with a bearer token.
package sendgrid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mu/internal/settings"
)

// endpoint is SendGrid's v3 send API.
const endpoint = "https://api.sendgrid.com/v3/mail/send"

// Message is one email to hand over.
//
// Plain and HTML both, because a message with only one of them is scored worse
// by every filter that looks. ReplyTo is separate from From and matters more
// than it looks: mail is sent from the authenticated sending domain, which has
// no inbox behind it, so a reply addressed there goes nowhere. See Send.
type Message struct {
	From     string
	FromName string
	ReplyTo  string
	To       string
	Subject  string
	Plain    string
	HTML     string
}

// Result is what came back from a send.
type Result struct {
	// ID is the provider's id for the message, from the X-Message-Id header.
	// It is what a delivery question is asked with later.
	ID string
}

// APIKey is the credential, or "" when sending is not configured.
func APIKey() string { return strings.TrimSpace(settings.Get("SENDGRID_API_KEY")) }

// Domain is the authenticated sending domain — the one whose SPF and DKIM
// records are in place and whose reputation this carries.
//
// Deliberately not defaulted to MAIL_DOMAIN. Falling back to the root domain
// would quietly undo the entire reason this package exists, and it would do it
// on the instance where somebody forgot to set this rather than on the one
// where they thought about it. Empty means not configured, and Configured
// reports false.
func Domain() string {
	return strings.ToLower(strings.TrimSpace(settings.Get("EMAIL_DOMAIN")))
}

// Configured reports whether this instance can send through the provider.
func Configured() bool { return APIKey() != "" && Domain() != "" }

// Send hands one message over.
//
// A reply-to is set whenever the caller gives one, and the caller should. The
// sending domain is authenticated for sending and has no MX record, so a reply
// to a From on it is undeliverable — the person answering gets a bounce and
// the sender never learns their message was answered. Pointing replies back at
// the instance's own mail domain, which does have an inbox, is what makes a
// sent message the start of a conversation rather than a broadcast.
func Send(m Message) (Result, error) {
	if !Configured() {
		return Result{}, fmt.Errorf("email sending is not configured")
	}
	if strings.TrimSpace(m.To) == "" || strings.TrimSpace(m.From) == "" {
		return Result{}, fmt.Errorf("a sender and a recipient are both required")
	}

	from := map[string]string{"email": m.From}
	if m.FromName != "" {
		from["name"] = m.FromName
	}

	body := map[string]any{
		"personalizations": []map[string]any{{
			"to": []map[string]string{{"email": m.To}},
		}},
		"from":    from,
		"subject": m.Subject,
		"content": content(m),
	}
	if r := strings.TrimSpace(m.ReplyTo); r != "" {
		body["reply_to"] = map[string]string{"email": r}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+APIKey())
	req.Header.Set("Content-Type", "application/json")

	rsp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("could not reach the mail provider: %w", err)
	}
	defer rsp.Body.Close() //nolint:errcheck

	if rsp.StatusCode >= 300 {
		return Result{}, fmt.Errorf("the mail provider refused it: %s", reason(rsp))
	}
	return Result{ID: rsp.Header.Get("X-Message-Id")}, nil
}

// content is the parts of the message, plain first.
//
// Order is not cosmetic: RFC 2046 says the last part of a multipart/alternative
// is the one preferred, and SendGrid builds the MIME in the order given. Plain
// then HTML is what every client expects.
func content(m Message) []map[string]string {
	var out []map[string]string
	plain := m.Plain
	if strings.TrimSpace(plain) == "" {
		plain = " " // the API rejects an empty part, and rejects having none
	}
	out = append(out, map[string]string{"type": "text/plain", "value": plain})
	if strings.TrimSpace(m.HTML) != "" {
		out = append(out, map[string]string{"type": "text/html", "value": m.HTML})
	}
	return out
}

// reason pulls the provider's own explanation out of a refusal, so an operator
// reads "The from address does not match a verified Sender Identity" rather
// than "403".
func reason(rsp *http.Response) string {
	raw, _ := io.ReadAll(io.LimitReader(rsp.Body, 8<<10))
	var parsed struct {
		Errors []struct {
			Message string `json:"message"`
			Field   string `json:"field"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &parsed) == nil && len(parsed.Errors) > 0 {
		var parts []string
		for _, e := range parsed.Errors {
			if e.Field != "" {
				parts = append(parts, e.Field+": "+e.Message)
				continue
			}
			parts = append(parts, e.Message)
		}
		return strings.Join(parts, "; ")
	}
	if len(raw) > 0 {
		return fmt.Sprintf("%s: %s", rsp.Status, strings.TrimSpace(string(raw)))
	}
	return rsp.Status
}
