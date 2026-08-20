package mail

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mu/internal/app"

	"github.com/emersion/go-msgauth/dkim"
)

// DKIMConfig holds DKIM signing configuration
type DKIMConfig struct {
	Domain     string
	Selector   string
	PrivateKey *rsa.PrivateKey
}

// Global DKIM config - optional, auto-loaded if keys exist
var dkimConfig *DKIMConfig

// LoadDKIMConfig loads DKIM configuration from the DKIM_PRIVATE_KEY environment
// variable or from a file at ~/.mu/keys/dkim.key (env var takes precedence).
// Domain defaults to "localhost" if not specified
func LoadDKIMConfig(domain, selector string) error {
	var keyData []byte

	// Prefer the environment variable over the key file
	if envKey := os.Getenv("DKIM_PRIVATE_KEY"); envKey != "" {
		keyData = []byte(envKey)
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %v", err)
		}

		keyPath := filepath.Join(homeDir, ".mu", "keys", "dkim.key")

		// Check if private key exists
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			return fmt.Errorf("DKIM private key not found at %s", keyPath)
		}

		// Read private key file
		keyData, err = os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("failed to read DKIM key: %v", err)
		}
	}

	// Parse PEM block
	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	// Parse private key
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return fmt.Errorf("failed to parse private key (tried PKCS1 and PKCS8): %v, %v", err, err2)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("not an RSA private key")
		}
	}

	if domain == "" {
		domain = "localhost"
	}
	if selector == "" {
		selector = "default"
	}

	dkimConfig = &DKIMConfig{
		Domain:     domain,
		Selector:   selector,
		PrivateKey: privateKey,
	}

	app.Log("dkim", "DKIM signing enabled for domain %s with selector %s", domain, selector)
	return nil
}

// SendExternalEmail sends an email to an external address via direct relay
// Sends multipart/alternative with both plain text and HTML versions (like Gmail)
// Returns the generated Message-ID for threading purposes
func SendExternalEmail(displayName, from, to, subject, bodyPlain, bodyHTML string, replyToMsgID string) (string, error) {
	return sendExternal(displayName, from, "", to, subject, bodyPlain, bodyHTML, replyToMsgID, "")
}

// SendExternalReply answers into a thread that already exists.
//
// It differs from SendExternalEmail by carrying References — the whole chain of
// message ids, not just the parent. A receiving client threads on that, and one
// that sees only In-Reply-To will file a long conversation as a series of
// unrelated messages. The caller passes what arrived, and the message being
// answered is appended to it.
func SendExternalReply(displayName, from, to, subject, bodyPlain, bodyHTML,
	inReplyTo, references string) (string, error) {
	return sendExternal(displayName, from, "", to, subject, bodyPlain, bodyHTML,
		inReplyTo, references)
}

// SendExternalReplyAll is a reply that keeps everybody else on the thread.
//
// The one-to-one SendExternalReply answers the sender, which is right when the
// sender is the only other person there. Once the agent has been copied into a
// conversation, answering the sender alone leaves the rest of the thread
// watching half of it — and the next reply-all from anybody carries the agent's
// answer to them anyway, out of order and without its question.
//
// cc is everybody else, already stripped of this instance's own addresses by
// mail.Others: an agent that copies itself answers its own answer, forever, at
// a model call a turn.
func SendExternalReplyAll(displayName, from, to string, cc []string, subject, bodyPlain, bodyHTML,
	inReplyTo, references string) (string, error) {
	return sendExternalTo(displayName, from, "", to, cc, subject, bodyPlain, bodyHTML,
		inReplyTo, references)
}

// SendExternalAs sends under a From this instance signs for, with answers
// directed somewhere else.
//
// The two differ because the address a message comes *from* and the address it
// should be answered *at* stopped being the same thing once agent mail moved to
// its own subdomain: it goes out as you@<sending domain>, which carries no
// inbox, and a reply to it would bounce. Reply-To is what makes a sent message
// the start of a conversation rather than a broadcast.
func SendExternalAs(displayName, from, replyTo, to, subject, bodyPlain, bodyHTML string) (string, error) {
	return sendExternal(displayName, from, replyTo, to, subject, bodyPlain, bodyHTML, "", "")
}

func sendExternal(displayName, from, replyTo, to, subject, bodyPlain, bodyHTML string, replyToMsgID, references string) (string, error) {
	return sendExternalTo(displayName, from, replyTo, to, nil, subject, bodyPlain, bodyHTML, replyToMsgID, references)
}

// sendExternalTo is sendExternal with other people copied in.
//
// Separate rather than another positional parameter on the four functions above
// it, because every one of those call sites would then carry a nil that means
// "nobody else is here" — and the one case where somebody is is the interesting
// one. See SendExternalReplyAll.
func sendExternalTo(displayName, from, replyTo, to string, cc []string, subject, bodyPlain, bodyHTML string, replyToMsgID, references string) (string, error) {
	message, messageID := buildExternalTo(displayName, from, replyTo, to, cc, subject,
		bodyPlain, bodyHTML, replyToMsgID, references)
	return finishExternal(message, from, to, cc, messageID)
}

// buildExternal assembles the message, and is separate from sending it so the
// wire format can be tested.
//
// It is worth a seam of its own: the agent's answers went out as a
// multipart/alternative whose HTML half was empty, which is a well-formed
// message that displays as nothing, and no test could see it because building
// and relaying were one function.
func buildExternal(displayName, from, replyTo, to, subject, bodyPlain, bodyHTML string, replyToMsgID, references string) ([]byte, string) {
	return buildExternalTo(displayName, from, replyTo, to, nil, subject, bodyPlain, bodyHTML, replyToMsgID, references)
}

func buildExternalTo(displayName, from, replyTo, to string, cc []string, subject, bodyPlain, bodyHTML string, replyToMsgID, references string) ([]byte, string) {
	// Extract username from email for Message-ID
	username := from
	if strings.Contains(from, "@") {
		username = strings.Split(from, "@")[0]
	}

	// Generate unique Message-ID for threading
	messageID := fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), username, ConfiguredDomain())

	// Generate boundary for multipart
	boundary := fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())

	// Build email message
	var msg bytes.Buffer

	// Headers
	fromHeader := from
	if displayName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", displayName, from)
	}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	// Everybody else on the thread, so their clients show the answer as part of
	// the conversation rather than as a separate message that happens to say
	// the same thing.
	if len(cc) > 0 {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(cc, ", ")))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	if replyTo != "" {
		msg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", replyTo))
	}

	if replyToMsgID != "" {
		msg.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", replyToMsgID))
		// References is everything the thread has referenced, ending with the
		// message being answered. Naming only the parent is enough for the
		// second message in a thread and loses it by the fourth, which is where
		// a client gives up and files the answers as unrelated mail.
		//
		// The parent is appended here rather than by the caller, because a
		// caller that passes a chain without it produces a References that does
		// not mention what In-Reply-To names — which is worse than no chain.
		chain := strings.TrimSpace(references)
		if !strings.Contains(chain, replyToMsgID) {
			chain = strings.TrimSpace(chain + " " + replyToMsgID)
		}
		msg.WriteString(fmt.Sprintf("References: %s\r\n", chain))
	}

	msg.WriteString("MIME-Version: 1.0\r\n")

	bodyPlain = strings.ReplaceAll(bodyPlain, "\r\n", "\n")
	bodyPlain = strings.ReplaceAll(bodyPlain, "\n", "\r\n")

	// One part when there is only one thing to say.
	//
	// This wrote both halves of a multipart/alternative unconditionally, so a
	// caller with no HTML — the agent answering mail is one — sent a valid,
	// empty HTML part alongside the real text. A client shows the *last*
	// alternative it can render, which was the empty one, so the answer arrived
	// blank. The text was always there and never displayed.
	if strings.TrimSpace(bodyHTML) == "" {
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(bodyPlain)
		msg.WriteString("\r\n")
		return msg.Bytes(), messageID
	}

	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// Plain text part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(bodyPlain)
	msg.WriteString("\r\n\r\n")

	// HTML part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")

	// Wrap HTML content in proper HTML structure for better email client compatibility
	// Check if HTML already has proper structure
	htmlLower := strings.ToLower(bodyHTML)
	if !strings.Contains(htmlLower, "<html") && !strings.Contains(htmlLower, "<!doctype") {
		// No HTML structure - wrap in basic HTML document
		bodyHTML = fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; font-size: 14px; line-height: 1.6; color: #333; max-width: 100%%;">
<!-- Inline, deliberately: this is an email body, and mail clients strip
     <style> blocks. mu.css never reaches it, so the style attribute is the
     only styling that survives delivery. -->
%s
</body>
</html>`, bodyHTML)
	}

	bodyHTML = strings.ReplaceAll(bodyHTML, "\r\n", "\n")
	bodyHTML = strings.ReplaceAll(bodyHTML, "\n", "\r\n")
	msg.WriteString(bodyHTML)
	msg.WriteString("\r\n\r\n")

	// End boundary
	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return msg.Bytes(), messageID
}

// finishExternal signs, records and relays a built message.
//
// Split out because a message with no HTML alternative is finished earlier and
// must be signed and relayed the same way — a second copy of this would be a
// second place for DKIM to be forgotten.
func finishExternal(message []byte, from, to string, cc []string, messageID string) (string, error) {
	// Apply DKIM signing if configured
	if dkimConfig != nil {
		options := &dkim.SignOptions{
			Domain:                 dkimConfig.Domain,
			Selector:               dkimConfig.Selector,
			Signer:                 dkimConfig.PrivateKey,
			HeaderCanonicalization: dkim.CanonicalizationRelaxed,
			BodyCanonicalization:   dkim.CanonicalizationRelaxed,
			// Cc is signed too. A recipient list that is not covered by the
			// signature can be rewritten in transit, which on a reply-all is
			// the header that says who else is in the room.
			HeaderKeys: []string{"from", "to", "cc", "subject", "date", "message-id", "mime-version", "content-type"},
		}

		var signedBuf bytes.Buffer
		if err := dkim.Sign(&signedBuf, bytes.NewReader(message), options); err != nil {
			app.Log("dkim", "WARNING: DKIM signing failed: %v", err)
		} else {
			message = signedBuf.Bytes()
			app.Log("dkim", "Signed with DKIM (d=%s s=%s relaxed/relaxed)", dkimConfig.Domain, dkimConfig.Selector)
		}
	}

	// Auto-whitelist: record outbound message ID + recipient so replies
	// and future mail from this address are allowed through.
	RecordOutbound(messageID, to)

	app.Log("mail", "=== Direct Relay (Internal) ===")
	app.Log("mail", "From: %s", from)
	app.Log("mail", "To: %s", to)
	app.Log("mail", "Message-ID: %s", messageID)

	// Call relay function directly (no SMTP needed!)
	if err := RelayToExternal(from, to, message); err != nil {
		app.Log("mail", "✗ Failed to relay email: %v", err)
		return "", fmt.Errorf("failed to relay email: %v", err)
	}
	// And to everybody copied. One relay each, with the same message: the Cc
	// header already names them all, so each client shows the whole room.
	// A failure here is logged and not returned — the answer reached the person
	// who asked, and losing that because a third recipient's server was down
	// would be the wrong trade.
	for _, addr := range cc {
		RecordOutbound(messageID, addr)
		if err := RelayToExternal(from, addr, message); err != nil {
			app.Log("mail", "✗ could not copy %s on the reply: %v", addr, err)
		}
	}

	app.Log("mail", "✓ Email relayed successfully")
	return messageID, nil
}

// SendCalendarInvite sends an email carrying an iCalendar (.ics) invite so the
// event lands in the recipient's real calendar (Gmail/Google Calendar, Apple,
// Outlook). The message is multipart/mixed: an HTML body plus a text/calendar
// attachment. Sent and DKIM-signed from a Mu address (deliverable), with the
// event itself attributed to the user inside the .ics.
func SendCalendarInvite(displayName, from, to, subject, bodyHTML, ics string) (string, error) {
	username := from
	if strings.Contains(from, "@") {
		username = strings.Split(from, "@")[0]
	}
	messageID := fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), username, ConfiguredDomain())
	boundary := fmt.Sprintf("----=_Mixed_%d", time.Now().UnixNano())

	var msg bytes.Buffer
	fromHeader := from
	if displayName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", displayName, from)
	}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// HTML body part.
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n\r\n")
	bodyHTML = strings.ReplaceAll(bodyHTML, "\r\n", "\n")
	bodyHTML = strings.ReplaceAll(bodyHTML, "\n", "\r\n")
	msg.WriteString(bodyHTML)
	msg.WriteString("\r\n\r\n")

	// Calendar attachment part.
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/calendar; charset=utf-8; method=PUBLISH; name=\"invite.ics\"\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("Content-Disposition: attachment; filename=\"invite.ics\"\r\n\r\n")
	msg.WriteString(ics)
	msg.WriteString("\r\n\r\n")

	msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	message := msg.Bytes()
	if dkimConfig != nil {
		options := &dkim.SignOptions{
			Domain:                 dkimConfig.Domain,
			Selector:               dkimConfig.Selector,
			Signer:                 dkimConfig.PrivateKey,
			HeaderCanonicalization: dkim.CanonicalizationRelaxed,
			BodyCanonicalization:   dkim.CanonicalizationRelaxed,
			HeaderKeys:             []string{"from", "to", "subject", "date", "message-id", "mime-version", "content-type"},
		}
		var signedBuf bytes.Buffer
		if err := dkim.Sign(&signedBuf, bytes.NewReader(message), options); err != nil {
			app.Log("dkim", "WARNING: DKIM signing failed: %v", err)
		} else {
			message = signedBuf.Bytes()
		}
	}

	RecordOutbound(messageID, to)
	if err := RelayToExternal(from, to, message); err != nil {
		return "", fmt.Errorf("failed to relay calendar invite: %v", err)
	}
	return messageID, nil
}

// IsExternalEmail checks if an email address is external (contains @domain)
func IsExternalEmail(email string) bool {
	return IsExternalAddress(strings.TrimSpace(email))
}

// EmailForUser generates an email address for a local user
func EmailForUser(username, domain string) string {
	if domain == "" {
		domain = "localhost"
	}
	return username + "@" + domain
}

// ConfiguredDomain returns the configured mail domain
func ConfiguredDomain() string {
	domain := os.Getenv("MAIL_DOMAIN")
	if domain == "" {
		domain = "localhost"
	}
	return domain
}

// DKIMStatus returns the current DKIM configuration status
func DKIMStatus() (enabled bool, domain, selector string) {
	if dkimConfig == nil {
		return false, "", ""
	}
	return true, dkimConfig.Domain, dkimConfig.Selector
}
