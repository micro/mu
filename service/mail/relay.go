package mail

// Handing outbound mail to somebody whose IP the internet trusts.
//
// By default Mu delivers its own mail: look up the recipient's MX, dial port 25,
// speak SMTP. That is the correct thing and it is not the hard part. The hard
// part is reputation — a new IP with no history, no feedback loop and no bounce
// processing gets its mail filed as spam by Gmail and Outlook however carefully
// it is signed, and there is nothing in the protocol to fix that. It is a
// property of the address the packets came from.
//
// So: an optional relay. Set SMTP_RELAY_HOST and outbound goes through it
// instead — a provider with a warmed pool, which is what you are actually buying.
// Everything else is unchanged: the message is built here, DKIM-signed here with
// your own key, and logged here. The relay is one hop, not a rewrite.
//
// Provider-agnostic on purpose, and named for the protocol rather than for
// whoever you sign up with. SendGrid, Postmark, Resend, SES and a mail server
// down the hall all speak submission with a username and a password, so there
// is nothing to choose between them in code and no reason for this file to know
// which one you picked.
//
// Note what this does *not* change: inbound. Mu still runs its own SMTP server
// and still owns the mailbox, which is the half that matters — the address is
// yours and the mail is here. Paying somebody to carry the outbound leg so the
// caller does not have to hold that relationship is the ordinary trade this
// product makes everywhere else.

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/settings"
)

// relayHost is the submission server to hand outbound mail to, or empty for
// delivering it ourselves. host:port; 587 is assumed when no port is given.
func relayHost() string {
	host := strings.TrimSpace(settings.Get("SMTP_RELAY_HOST"))
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "587")
	}
	return host
}

// Relaying reports whether outbound mail goes through a submission server
// rather than straight to the recipient's MX. For the status page: which of the
// two is happening is the first thing to know when mail is not arriving.
func Relaying() bool { return relayHost() != "" }

// RelayHostname is the configured relay, for display. Empty when there is none.
func RelayHostname() string { return relayHost() }

// relayViaSubmission hands one message to the configured relay.
//
// STARTTLS is required rather than preferred: the credential goes over this
// connection, and a submission server that will not encrypt is one to fail
// against rather than to send a password to.
func relayViaSubmission(host, from, to string, data []byte) error {
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		return fmt.Errorf("SMTP_RELAY_HOST is not a host and port: %v", err)
	}

	conn, err := net.DialTimeout("tcp", host, 30*time.Second)
	if err != nil {
		return fmt.Errorf("could not reach the relay at %s: %v", host, err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, name)
	if err != nil {
		return err
	}
	defer c.Close()

	if err := c.Hello(ConfiguredDomain()); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); !ok {
		return fmt.Errorf("the relay at %s does not offer STARTTLS", host)
	}
	if err := c.StartTLS(&tls.Config{ServerName: name}); err != nil {
		return err
	}

	user := strings.TrimSpace(settings.Get("SMTP_RELAY_USER"))
	pass := settings.Get("SMTP_RELAY_PASS")
	if user != "" {
		if err := c.Auth(smtp.PlainAuth("", user, pass, name)); err != nil {
			return fmt.Errorf("the relay refused the credentials: %v", err)
		}
	}

	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	c.Quit()

	app.Log("mail", "✓ Relayed %s via %s", to, host)
	return nil
}
