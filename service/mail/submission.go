package mail

// Submission: sending from the mail client you read in.
//
// IMAP made the mailbox openable in Thunderbird and left it read-only, because
// a mail client sends through SMTP submission and there was none. The SMTP
// server here is an MTA — it accepts mail *for* local users and has never
// accepted mail *from* one, which is the right shape for port 25 and the wrong
// shape for a person replying on their phone.
//
// Backend.Login looks like the missing half and is not. go-smtp has
// authenticated through the AuthSession interface since well before v0.24, so
// that method satisfies nothing and was never called; giving it a success path
// would have changed no behaviour at all. This adds the interface the library
// actually asks for, on a listener of its own.
//
// # One way out
//
// Every message here leaves through ReplyOut, which is outbound.go's single
// door: it asks MaySendOut and it charges. That is not tidiness. It is the
// whole argument of that file — three send paths once had three copies of the
// quota check and each could send without the others' rules — so submission is
// a new *front door* onto the existing exit and never a second exit.
//
// What follows from that, rather than being decided again here: a mail client
// costs what the compose form costs, an account that may not cold-mail
// strangers from the web cannot do it from Thunderbird either, and a reply to
// somebody who wrote first is ungated in both.
//
// # It will not send as somebody else
//
// MAIL FROM must be an address the authenticated account owns. A token
// authenticates an account and says nothing about which address it may put in
// MAIL FROM, so without that check a stolen token would be a licence to forge
// every address on the domain — including the ones carrying password resets.
// See ownsAddress.
//
// # TLS
//
// None here, for the reason imap.go gives: nothing in this repo terminates
// TLS. The operator puts the same proxy in front on 465. The port defaults
// high because an unprivileged process cannot bind 587, and a default that
// cannot start is a feature nobody finds.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"

	"github.com/emersion/go-sasl"
	smtpd "github.com/emersion/go-smtp"

	"mu/internal/app"
	"mu/internal/auth"
)

// submissionMaxRecipients caps one message. Generous for a person writing to a
// group, small enough that a stolen token is not a mailing list.
const submissionMaxRecipients = 20

// StartSubmissionServer serves SMTP submission on addr until it fails.
func StartSubmissionServer(addr string) error {
	s := smtpd.NewServer(&submissionBackend{})
	s.Addr = addr
	s.Domain = ConfiguredDomain()
	s.AllowInsecureAuth = true // the proxy terminates TLS — see the package comment
	s.MaxMessageBytes = 1024 * 1024 * 10
	s.MaxRecipients = submissionMaxRecipients

	app.Log("mail", "Starting submission server on %s", addr)
	app.Log("mail", "  - Log in with your username and an access token as the password")
	return s.ListenAndServe()
}

// StartSubmissionServerIfEnabled starts submission unless it is turned off.
func StartSubmissionServerIfEnabled() {
	addr, on := app.ListenAddr("SUBMISSION_PORT", app.SubmissionPort)
	if !on {
		return
	}
	go func() {
		if err := StartSubmissionServer(addr); err != nil {
			app.Log("mail", "submission server error: %v", err)
		}
	}()
}

type submissionBackend struct{}

func (*submissionBackend) NewSession(c *smtpd.Conn) (smtpd.Session, error) {
	ip := ""
	if c != nil && c.Conn() != nil {
		ip, _, _ = net.SplitHostPort(c.Conn().RemoteAddr().String())
	}
	return &submissionSession{remoteIP: ip}, nil
}

// submissionSession is one authenticated client sending mail.
type submissionSession struct {
	remoteIP string
	acc      *auth.Account // nil until AUTH succeeds
	from     string
	to       []string
}

func (s *submissionSession) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *submissionSession) Auth(mech string) (sasl.Server, error) {
	if mech != sasl.Plain {
		return nil, smtpd.ErrAuthUnsupported
	}
	return sasl.NewPlainServer(func(identity, username, password string) error {
		acc, err := accountForToken(username, password)
		if err != nil {
			app.Log("mail", "submission sign-in refused for %q from %s", username, s.remoteIP)
			return &smtpd.SMTPError{Code: 535, Message: err.Error()}
		}
		s.acc = acc
		app.Log("mail", "submission: %s signed in from %s", acc.ID, s.remoteIP)
		return nil
	}), nil
}

// errNotAuthenticated is what every command answers before AUTH.
//
// 530 is the code a mail client understands as "authenticate and try again",
// which is what makes it show the password box rather than an error.
var errNotAuthenticated = &smtpd.SMTPError{
	Code:    530,
	Message: "authenticate first — your username and an access token from /token",
}

func (s *submissionSession) Mail(from string, opts *smtpd.MailOptions) error {
	if s.acc == nil {
		return errNotAuthenticated
	}
	if !ownsAddress(s.acc, from) {
		app.Log("mail", "submission refused: %s tried to send as %q", s.acc.ID, from)
		return &smtpd.SMTPError{
			Code:    550,
			Message: fmt.Sprintf("you can only send as your own address, not %s", from),
		}
	}
	s.from = strings.Trim(strings.TrimSpace(from), "<>")
	s.to = nil
	return nil
}

func (s *submissionSession) Rcpt(to string, opts *smtpd.RcptOptions) error {
	if s.acc == nil {
		return errNotAuthenticated
	}
	to = strings.Trim(strings.TrimSpace(to), "<>")
	if to == "" {
		return &smtpd.SMTPError{Code: 501, Message: "empty recipient"}
	}
	if len(s.to) >= submissionMaxRecipients {
		return &smtpd.SMTPError{Code: 452, Message: "too many recipients"}
	}
	s.to = append(s.to, to)
	return nil
}

// Data sends the message, one recipient at a time.
//
// Per recipient rather than in one go, because the two destinations are not the
// same act: somebody outside is mail leaving the instance, priced and gated by
// ReplyOut, and somebody here is a message filed in their inbox, which never
// touches the network. A client addressing both in one message means both.
func (s *submissionSession) Data(r io.Reader) error {
	if s.acc == nil {
		return errNotAuthenticated
	}
	if s.from == "" || len(s.to) == 0 {
		return &smtpd.SMTPError{Code: 503, Message: "need MAIL FROM and RCPT TO first"}
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, r); err != nil {
		return err
	}
	msg, err := mail.ReadMessage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return &smtpd.SMTPError{Code: 554, Message: "could not parse that message"}
	}

	subject := decodeMIMEHeader(msg.Header.Get("Subject"))
	inReplyTo := strings.TrimSpace(msg.Header.Get("In-Reply-To"))
	references := strings.TrimSpace(msg.Header.Get("References"))
	plain, html := submissionBody(msg)

	// The display name the client put in From, which is what the recipient
	// sees. Falls back to the account rather than being empty.
	display := s.acc.Name
	if a, err := mail.ParseAddress(msg.Header.Get("From")); err == nil && a.Name != "" {
		display = a.Name
	}
	if strings.TrimSpace(display) == "" {
		display = s.acc.ID
	}

	var failed []string
	for _, to := range s.to {
		var err error
		// A conversation that did not arrive by mail — a text, a WhatsApp
		// message — answered from a mail client. The address names it and the
		// filler resolves it against what this account already has, so nothing
		// here can start one.
		if BridgedReply != nil {
			body := plain
			if strings.TrimSpace(body) == "" {
				body = html
			}
			if handled, ferr := BridgedReply(s.acc.ID, to, body); handled {
				if ferr != nil {
					app.Log("mail", "submission: %s -> %s failed: %v", s.acc.ID, to, ferr)
					failed = append(failed, fmt.Sprintf("%s (%v)", to, ferr))
				} else {
					app.Log("mail", "submission: %s -> %s sent", s.acc.ID, to)
				}
				continue
			}
		}
		if IsExternalEmail(to) {
			// ReplyOut sends and does not record — every caller files its own
			// copy afterwards, which is why this one has to as well. Without it
			// mail sent from a client left no trace anywhere on the instance:
			// not in /inbox, not in the Sent view, and not in what the agent
			// can see, so the agent did not know what you had already said.
			//
			// IMAP has no APPEND either, so the client cannot upload the copy
			// itself the way it would to any other server. That makes this the
			// only place the record can come from.
			var messageID string
			messageID, err = ReplyOut(s.acc.ID, display, to, subject, plain, html, inReplyTo, references)
			if err == nil {
				if e := SendMessage(display, s.acc.ID, to, to, subject, plain,
					inReplyTo, messageID); e != nil {
					app.Log("mail", "submission: sent to %s but not recorded: %v", to, e)
				}
			}
		} else {
			err = s.deliverLocally(to, display, subject, plain, html, inReplyTo, references)
		}
		if err != nil {
			app.Log("mail", "submission: %s -> %s failed: %v", s.acc.ID, to, err)
			failed = append(failed, fmt.Sprintf("%s (%v)", to, err))
			continue
		}
		app.Log("mail", "submission: %s -> %s sent", s.acc.ID, to)
	}

	if len(failed) > 0 {
		// The reason, not a generic failure. Every refusal MaySendOut can give
		// is something the person can act on — verify your address, add credit,
		// this is not a reply — and a client only shows them what we say here.
		return &smtpd.SMTPError{
			Code:    550,
			Message: "could not send to " + strings.Join(failed, "; "),
		}
	}
	return nil
}

// deliverLocally files a message for somebody on this instance, and wakes
// whatever was listening for it.
//
// It used to say there was nothing to charge, "the same act as sending a
// message from one page of this instance to another" — which was true and was
// the hole: that act was free everywhere, so a signed-in account with a mail
// client could write to every username here at no cost and no cap. Deliver is
// the gate now, and it is the same one the page and the tool go through.
//
// This was the only door that had the whole rule, and having it here is why the
// other three each got a piece of it wrong. It is the shared one now, and this
// is what is left: an address, a body, and the IP the client connected from.
func (s *submissionSession) deliverLocally(to, display, subject, plain, html, replyTo, references string) error {
	if !strings.Contains(to, "@") {
		return fmt.Errorf("not an address")
	}
	_, err := Deliver(Outgoing{
		FromID: s.acc.ID, Display: display, To: to,
		Subject: subject, Body: plain, HTML: html,
		InReplyTo: replyTo, References: references,
		SenderIP: s.remoteIP,
	})
	return err
}

func (s *submissionSession) Reset() { s.from, s.to = "", nil }

func (s *submissionSession) Logout() error { return nil }

// submissionBody pulls the text and HTML out of what a client sent.
//
// Both, where both are offered. A mail client sends multipart/alternative with
// a text part and an HTML part, and taking only the text would quietly discard
// every link and every bit of formatting the person wrote — the message would
// arrive, looking like somebody else had written it.
func submissionBody(msg *mail.Message) (plain, html string) {
	contentType := msg.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		b, _ := io.ReadAll(msg.Body)
		body := decodeTransfer(string(b), msg.Header.Get("Content-Transfer-Encoding"))
		if strings.HasPrefix(mediaType, "text/html") {
			return stripHTMLTags(body), body
		}
		return body, ""
	}

	plain, html = walkParts(msg.Body, params["boundary"])
	if plain == "" && html != "" {
		plain = stripHTMLTags(html)
	}
	return plain, html
}

// walkParts reads a multipart body, one level deep and then recursively.
//
// Recursive because multipart/mixed wraps multipart/alternative the moment
// there is an attachment, so the text a person typed is two levels down in the
// most ordinary message a client sends.
func walkParts(body io.Reader, boundary string) (plain, html string) {
	if boundary == "" {
		return "", ""
	}
	mr := multipart.NewReader(body, boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			return plain, html
		}
		mediaType, params, err := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(mediaType, "multipart/"):
			p, h := walkParts(part, params["boundary"])
			if plain == "" {
				plain = p
			}
			if html == "" {
				html = h
			}
		case mediaType == "text/plain" && plain == "":
			b, _ := io.ReadAll(part)
			plain = decodeTransfer(string(b), part.Header.Get("Content-Transfer-Encoding"))
		case mediaType == "text/html" && html == "":
			b, _ := io.ReadAll(part)
			html = decodeTransfer(string(b), part.Header.Get("Content-Transfer-Encoding"))
		}
	}
}

// decodeTransfer undoes the encoding a client applied to a part.
//
// quoted-printable is what every client uses the moment a message contains a
// non-ASCII character or a long line, so without this an apostrophe arrives as
// =E2=80=99 — which is exactly the mail somebody notices.
func decodeTransfer(body, encoding string) string {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "quoted-printable":
		if out, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body))); err == nil {
			return string(out)
		}
	case "base64":
		if out, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(body), "")); err == nil {
			return string(out)
		}
	}
	return body
}
