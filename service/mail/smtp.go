package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mu/internal/app"
	"mu/internal/auth"

	"github.com/emersion/go-msgauth/dkim"
	smtpd "github.com/emersion/go-smtp"
)

// Rate limiting configuration
var (
	rateLimitMutex    sync.RWMutex
	ipConnections     = make(map[string]*ipRateLimit)
	senderMessages    = make(map[string]*senderRateLimit)
	cleanupInterval   = 1 * time.Hour
	maxIPConnections  = 10  // Max connections per IP per hour
	maxSenderMessages = 100 // Max messages per sender per day
)

// ipRateLimit tracks connection attempts per IP
type ipRateLimit struct {
	count     int
	resetTime time.Time
}

// senderRateLimit tracks messages per sender
type senderRateLimit struct {
	count     int
	resetTime time.Time
}

// Backend implements SMTP server backend for RECEIVING mail only
// This is NOT an open relay - it only accepts mail for local users
type Backend struct{}

// Login authenticates a user. Required for AUTH support.
func (bkd *Backend) Login(conn *smtpd.Conn, username, password string) (smtpd.Session, error) {
	// Extract IP address
	remoteAddr := conn.Conn().RemoteAddr().String()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}

	// Only allow auth from localhost
	isLocalhost := ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "127.0.0.") || ip == "[::1]"

	if !isLocalhost {
		app.Log("mail", "Backend AUTH rejected: not from localhost (IP: %s)", ip)
		return nil, &smtpd.SMTPError{
			Code:    530,
			Message: "Authentication not available",
		}
	}

	app.Log("mail", "Backend AUTH failed: no valid credentials configured")
	return nil, &smtpd.SMTPError{
		Code:    535,
		Message: "Authentication failed",
	}
}

// NewSession creates a new SMTP session
// No authentication required - this server only RECEIVES mail
func (bkd *Backend) NewSession(conn *smtpd.Conn) (smtpd.Session, error) {
	// Extract IP address
	remoteAddr := conn.Conn().RemoteAddr().String()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}

	// Check rate limit for this IP
	if !checkIPRateLimit(ip) {
		app.Log("mail", "Rate limit exceeded for IP: %s", ip)
		return nil, &smtpd.SMTPError{
			Code:    421,
			Message: "Too many connections from your IP. Please try again later.",
		}
	}

	app.Log("mail", "New SMTP session from IP: %s", ip)
	return &Session{remoteIP: ip}, nil
}

// Session represents an SMTP session for RECEIVING mail
type Session struct {
	from        string
	to          []string
	remoteIP    string
	isLocalhost bool // True if connecting from localhost (trusted internal client)
	spfPass     bool // Whether SPF verification passed
}

// Mail is called when the MAIL FROM command is received
func (s *Session) Mail(from string, opts *smtpd.MailOptions) error {
	s.from = from

	// Check if connection is from localhost (trusted internal Go app)
	s.isLocalhost = s.remoteIP == "127.0.0.1" || s.remoteIP == "::1" || strings.HasPrefix(s.remoteIP, "127.0.0.") || s.remoteIP == "[::1]"

	if s.isLocalhost {
		app.Log("mail", "Mail from: %s (localhost - trusted internal client)", from)
		return nil // Trust localhost connections from our web app
	}

	app.Log("mail", "Mail from: %s (IP: %s)", from, s.remoteIP)

	// Parse and validate the sender address.
	fromAddr, _ := mail.ParseAddress(from)
	if fromAddr == nil {
		app.Log("mail", "Rejected invalid sender address: %s", from)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Sender address rejected: invalid format",
		}
	}
	fromParts := strings.Split(fromAddr.Address, "@")
	if len(fromParts) != 2 || fromParts[1] == "" {
		app.Log("mail", "Rejected sender without domain: %s", from)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Sender address rejected: missing domain",
		}
	}
	senderDomain := fromParts[1]

	// Reject domains without a dot — "wetransfer" is not an FQDN,
	// "wetransfer.com" is. This blocks a common spam pattern.
	if !strings.Contains(senderDomain, ".") {
		app.Log("mail", "Rejected non-FQDN sender domain: %s (from %s)", senderDomain, from)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Sender address rejected: domain must be fully qualified",
		}
	}

	// Reject external senders claiming to be from our domain (anti-spoofing)
	if strings.EqualFold(senderDomain, ConfiguredDomain()) {
		app.Log("mail", "Rejected domain spoofing: external IP %s claiming to send from %s", s.remoteIP, from)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Sender address rejected: not authorized to send from this domain",
		}
	}

	// Check blocklist first
	if IsBlocked(from, s.remoteIP) {
		app.Log("mail", "Rejected blocked sender: %s (IP: %s)", from, s.remoteIP)
		return &smtpd.SMTPError{
			Code:    554,
			Message: "Transaction failed: sender blocked",
		}
	}

	// Check sender rate limit
	if !checkSenderRateLimit(from) {
		app.Log("mail", "Rate limit exceeded for sender: %s", from)
		return &smtpd.SMTPError{
			Code:    421,
			Message: "Too many messages from this sender. Please try again later.",
		}
	}

	// Verify SPF record for sender domain
	s.spfPass = verifySPF(from, s.remoteIP)
	if !s.spfPass {
		app.Log("mail", "SPF verification failed for %s from IP %s", from, s.remoteIP)
	}

	return nil
}

// Logout is called when the connection is closed
func (s *Session) Logout() error {
	return nil
}

// Rcpt is called when the RCPT TO command is received
// Validates that the recipient is a local user OR allows external if authenticated OR from localhost
func (s *Session) Rcpt(to string, opts *smtpd.RcptOptions) error {
	// Extract username from email address
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		toAddr = &mail.Address{Address: to}
	}

	// Get username (part before @)
	parts := strings.Split(toAddr.Address, "@")
	if len(parts) == 0 {
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Invalid recipient address",
		}
	}

	// asim+research@ is asim's inbox, tagged research — see alias.go. The
	// account lookup uses the part before the plus; without this, every
	// plus-address is rejected as a non-existent user.
	username, _ := SplitAlias(parts[0])

	// If from localhost (trusted internal client), allow any recipient
	// But still require SMTP AUTH to prevent abuse
	if s.isLocalhost {
		s.to = append(s.to, to)
		app.Log("mail", "Accepting recipient %s from localhost (authenticated internal client)", to)
		return nil
	}

	// Not from localhost - ONLY accept mail for LOCAL users (not an open relay)
	// First check if recipient domain matches our domain
	if len(parts) < 2 {
		app.Log("mail", "Rejected mail: no domain specified in recipient")
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Invalid recipient address",
		}
	}

	recipientDomain := parts[1]
	if recipientDomain != ConfiguredDomain() {
		app.Log("mail", "Rejected mail for external domain %s (not an open relay)", recipientDomain)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Relay access denied",
		}
	}

	// Two addresses this instance offers that nobody holds. Both are reserved
	// usernames (internal/auth/username.go), so the account lookup below refuses
	// them and the mail is rejected at the door — 550 before Data ever runs.
	// Data knows what to do with each of them; without this it never got the
	// chance, which is how agent@ was unreachable while the code answering it
	// sat there working.
	//
	//   agent@   — write to your agent and it writes back. Data attributes the
	//              mail by *sender*, so a verified address on this instance
	//              reaches its own agent with nothing to remember.
	//
	// support@ used to be the other one, delivering to the first admin. It was
	// the only address here the whitelist did not apply to, which made it the
	// only address a spammer could reach, and that is what it filled up with.
	if strings.EqualFold(username, AgentMailbox) {
		s.to = append(s.to, to)
		app.Log("mail", "Accepting mail for reserved mailbox %s", to)
		return nil
	}

	// Domain matches - verify user exists and has mail access
	acc, err := auth.GetAccount(username)
	if err != nil {
		app.Log("mail", "Rejected mail for non-existent user: %s", username)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "User not found",
		}
	}

	// All registered users can receive mail
	_ = acc

	s.to = append(s.to, to)
	app.Log("mail", "Accepting mail for local user: %s", username)
	return nil
}

// RelayToExternal sends email directly to an external SMTP server (exported
// for internal use). Every outbound external message passes through here —
// user mail, calendar invites, verification mail — so this is where the relay
// log is written. See relay_log.go.
func RelayToExternal(from, to string, data []byte) error {
	err := relayToExternal(from, to, data)
	recordRelay(from, to, data, err)
	return err
}

// relayToExternal delivers email to an external SMTP server
func relayToExternal(from, to string, data []byte) error {
	// Through a submission server where one is configured. Deliverability is
	// about the reputation of the IP the packets came from, which is not
	// something the protocol can fix from this end. See relay.go.
	if host := relayHost(); host != "" {
		return relayViaSubmission(host, from, to, data)
	}

	// Extract domain from recipient address
	parts := strings.Split(to, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid email address: %s", to)
	}
	domain := parts[1]

	// Look up MX records for the domain
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		app.Log("mail", "No MX records found for %s, trying domain directly", domain)
		// Fallback to domain directly if no MX records
		mxRecords = []*net.MX{{Host: domain, Pref: 10}}
	}

	// Sort MX records by preference (lower is higher priority)
	sort.Slice(mxRecords, func(i, j int) bool {
		return mxRecords[i].Pref < mxRecords[j].Pref
	})

	// Try each MX record until one succeeds
	var lastErr error
	for _, mx := range mxRecords {
		host := strings.TrimSuffix(mx.Host, ".")
		app.Log("mail", "Attempting relay to %s (MX for %s)", host, domain)

		// Try port 25 (standard SMTP)
		addr := net.JoinHostPort(host, "25")

		// Connect with timeout
		conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
		if err != nil {
			app.Log("mail", "Failed to connect to %s: %v", addr, err)
			lastErr = err
			continue
		}
		defer conn.Close()

		// Create SMTP client
		client, err := smtp.NewClient(conn, host)
		if err != nil {
			app.Log("mail", "Failed to create SMTP client for %s: %v", host, err)
			lastErr = err
			continue
		}
		defer client.Close()

		// Say HELO/EHLO
		hostname := ConfiguredDomain()
		if err := client.Hello(hostname); err != nil {
			app.Log("mail", "HELO failed for %s: %v", host, err)
			lastErr = err
			continue
		}

		// MAIL FROM
		if err := client.Mail(from); err != nil {
			app.Log("mail", "MAIL FROM failed for %s: %v", host, err)
			lastErr = err
			continue
		}

		// RCPT TO
		if err := client.Rcpt(to); err != nil {
			app.Log("mail", "RCPT TO failed for %s: %v", host, err)
			lastErr = err
			continue
		}

		// DATA
		wc, err := client.Data()
		if err != nil {
			app.Log("mail", "DATA command failed for %s: %v", host, err)
			lastErr = err
			continue
		}

		// Write email data
		if _, err := wc.Write(data); err != nil {
			app.Log("mail", "Failed to write data to %s: %v", host, err)
			lastErr = err
			wc.Close()
			continue
		}

		// Close data writer
		if err := wc.Close(); err != nil {
			app.Log("mail", "Failed to close data writer for %s: %v", host, err)
			lastErr = err
			continue
		}

		// QUIT
		client.Quit()

		app.Log("mail", "✓ Successfully relayed email to %s via %s", to, host)
		return nil
	}

	return fmt.Errorf("failed to relay to any MX server for %s: %v", domain, lastErr)
}

// Data is called when the DATA command is received
func (s *Session) Data(r io.Reader) error {
	// Read the email data
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, r); err != nil {
		return err
	}

	// Verify DKIM signature before parsing consumes the reader
	dkimPass := false
	if !s.isLocalhost {
		verifications, err := dkim.Verify(bytes.NewReader(buf.Bytes()))
		if err == nil && len(verifications) > 0 {
			for _, v := range verifications {
				if v.Err == nil {
					dkimPass = true
					app.Log("mail", "DKIM verification passed for domain %s", v.Domain)
					break
				}
				app.Log("mail", "DKIM verification failed for domain %s: %v", v.Domain, v.Err)
			}
		} else if err != nil {
			app.Log("mail", "DKIM verification error: %v", err)
		} else {
			app.Log("mail", "No DKIM signature found")
		}
	} else {
		dkimPass = true // Trust localhost
	}

	// Parse the email
	msg, err := mail.ReadMessage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		app.Log("mail", "Error parsing email: %v", err)
		return err
	}

	// Extract headers
	from := msg.Header.Get("From")
	subject := decodeMIMEHeader(msg.Header.Get("Subject"))
	contentType := msg.Header.Get("Content-Type")
	messageID := msg.Header.Get("Message-ID")
	inReplyTo := msg.Header.Get("In-Reply-To")
	references := msg.Header.Get("References")
	// Everybody the message went to, which is a different question from the one
	// recipient this pass is delivering to.
	//
	// SMTP does not distinguish To from Cc — both arrive as RCPT TO — so being
	// copied into somebody's conversation already reached this far. What was
	// missing is any record of who else is on it, so the reply went to the
	// sender alone: the agent answered one person in a room of three. See cc.go.
	headerTo := Recipients(msg.Header.Get("To"))
	headerCc := Recipients(msg.Header.Get("Cc"))

	// A message with no id of its own gets one here.
	//
	// RFC 5322 says a sender should always write one and a few do not, and every
	// downstream question keys on it: which stored message this is
	// (FindMessageByMessageID), which conversation a later reply continues, and
	// whether a message already in the record is this one. Without it those all
	// answer "no such thing", so two copies of one mail become two of everything.
	// Ours is marked so it cannot be mistaken for the sender's.
	if strings.TrimSpace(messageID) == "" {
		messageID = fmt.Sprintf("<%d.received@%s>", time.Now().UnixNano(), ConfiguredDomain())
	}

	// Capture raw headers for View Raw display
	var rawHeaders strings.Builder
	// Add a Received header with our server info
	rawHeaders.WriteString(fmt.Sprintf("Received: from %s (%s)\r\n        by %s with SMTP; %s\r\n",
		s.remoteIP, s.remoteIP, ConfiguredDomain(), time.Now().UTC().Format(time.RFC1123Z)))
	// Preserve all original headers
	for key, vals := range msg.Header {
		for _, v := range vals {
			rawHeaders.WriteString(fmt.Sprintf("%s: %s\r\n", key, v))
		}
	}
	rawHeaderStr := rawHeaders.String()

	// Parse sender email
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		fromAddr = &mail.Address{Address: from}
	}

	// ANTI-SPOOFING: Reject external mail where the From header claims our domain.
	// The envelope MAIL FROM check (in Mail()) catches direct spoofing, but attackers
	// can use a different envelope sender and forge the From: header in the body.
	if !s.isLocalhost {
		headerParts := strings.Split(fromAddr.Address, "@")
		if len(headerParts) == 2 && strings.EqualFold(headerParts[1], ConfiguredDomain()) {
			app.Log("mail", "Rejected header spoofing: external IP %s with From header %s", s.remoteIP, fromAddr.Address)
			return &smtpd.SMTPError{
				Code:    550,
				Message: "Sender address rejected: not authorized to send from this domain",
			}
		}
	}

	// ── Strict inbound filter ──────────────────────────────────
	// The whole policy is at the top of inbound_filter.go.
	if !s.isLocalhost {
		reason, allowed := CheckInboundAllowed(fromAddr.Address, s.to, inReplyTo, references)
		if !allowed {
			app.Log("mail", "Rejected inbound from %s: %s", fromAddr.Address, reason)
			return &smtpd.SMTPError{
				Code:    550,
				Message: "Mail rejected: " + reason,
			}
		}
	}

	// Parse body based on content type
	var body string
	// What the message carried besides its text, kept out of body and stored
	// beside it so the message view can still render a DMARC report.
	var inboundAttachment *Attachment
	if strings.Contains(contentType, "multipart/") {
		// Parse multipart message
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			boundary := params["boundary"]
			if boundary != "" {
				body, inboundAttachment = parseMultipart(msg.Body, boundary)
			} else {
				// Fallback to reading raw body
				bodyBytes, _ := io.ReadAll(msg.Body)
				body = string(bodyBytes)
			}
		} else {
			bodyBytes, _ := io.ReadAll(msg.Body)
			body = string(bodyBytes)
		}
	} else {
		// Plain text or HTML - read directly
		bodyBytes, err := io.ReadAll(msg.Body)
		if err != nil {
			app.Log("mail", "Error reading body: %v", err)
			return err
		}

		// Decode based on transfer encoding
		transferEncoding := msg.Header.Get("Content-Transfer-Encoding")
		if strings.ToLower(transferEncoding) == "base64" {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(bodyBytes))); err == nil {
				bodyBytes = decoded
				app.Log("mail", "Decoded base64 body (%d bytes)", len(bodyBytes))
			}
		} else if strings.ToLower(transferEncoding) == "quoted-printable" {
			reader := quotedprintable.NewReader(bytes.NewReader(bodyBytes))
			if decoded, err := io.ReadAll(reader); err == nil {
				bodyBytes = decoded
				app.Log("mail", "Decoded quoted-printable body (%d bytes)", len(bodyBytes))
			}
		}

		// Store the decoded content. Binary gets described rather than
		// encoded, for the same reason as the multipart path above: the body
		// is what the inbox previews, what the index searches and what an
		// agent reads, and a wall of base64 corrupts all three.
		body, inboundAttachment = singlePartBody(bodyBytes, contentType)

		// Additional check: if the body looks entirely like base64 (no header specified),
		// try decoding it as a fallback for improperly formatted emails
		if transferEncoding == "" && looksLikeBase64(body) {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body)); err == nil {
				// Verify the decoded content is valid UTF-8 text
				if isValidUTF8Text(decoded) {
					body = string(decoded)
					app.Log("mail", "Decoded base64-looking email body (no encoding header)")
				}
			}
		}
	}

	// Process each recipient
	for _, recipient := range s.to {
		// Parse recipient email
		toAddr, err := mail.ParseAddress(recipient)
		if err != nil {
			toAddr = &mail.Address{Address: recipient}
		}

		// Extract domain from email
		parts := strings.Split(toAddr.Address, "@")
		if len(parts) != 2 {
			app.Log("mail", "Invalid recipient address: %s", recipient)
			continue
		}
		toUsername, toTag := SplitAlias(parts[0])
		toDomain := parts[1]

		// Check if this is an external recipient
		isExternal := toDomain != ConfiguredDomain()

		if isExternal && s.isLocalhost {
			// Relay to external SMTP server
			app.Log("mail", "Relaying to external address: %s", toAddr.Address)
			if err := RelayToExternal(s.from, toAddr.Address, buf.Bytes()); err != nil {
				app.Log("mail", "Error relaying to %s: %v", toAddr.Address, err)
				continue
			}
			app.Log("mail", "✓ Successfully relayed to %s", toAddr.Address)
			continue
		}

		// Look up the recipient account (local user).
		//
		// agent@<domain> is the exception: it belongs to the instance rather
		// than to a person, so whose mail it is comes from who sent it. That
		// address needs nothing remembered — no plus convention, no recalling
		// which agent you named what — and it is the address agent replies
		// already come from, so it is what makes replying to your agent
		// continue the conversation instead of bouncing.
		//
		// A tag on it names which of your agents answers: agent+research@ is
		// the same address with an addressee. It carries the agent's name and
		// not the owner's, which is the half worth dropping — you know what you
		// called the thing, and you should not also have to spell your own
		// username to reach it.
		var toAcc *auth.Account
		sharedAgentMail := !isExternal && strings.EqualFold(toUsername, AgentMailbox)
		if sharedAgentMail {
			toAcc = AccountForVerifiedEmail(fromAddr.Address)
			if toAcc == nil {
				// Somebody nobody here has heard of, writing to the address the
				// front page advertises.
				//
				// This was dropped — silently, so a probe could not learn the
				// address was live. Good reasoning, bad outcome: the landing
				// page says "write to it and it answers", and for everybody
				// without an account it did not. The first thing anybody does
				// with an agent that has an address is write to it, and the
				// answer was nothing.
				//
				// They get an account instead — unclaimed, no password, holding
				// the conversation until they sign up and claim it. See
				// auth.Unclaimed.
				//
				// Only if the mail authenticated. Without SPF or DKIM the
				// sender address is whatever the sending machine typed, and an
				// allowance per address becomes an open model-call endpoint
				// costing an operator money per request. Still silent when it
				// fails, for the original reason.
				if !dkimPass && !s.spfPass {
					app.Log("mail", "Shared agent mail from unauthenticated sender %s: dropped",
						fromAddr.Address)
					continue
				}
				var err error
				if toAcc, err = auth.Unclaimed(fromAddr.Address); err != nil || toAcc == nil {
					app.Log("mail", "Shared agent mail from %s: could not open an account: %v",
						fromAddr.Address, err)
					continue
				}
				app.Log("mail", "Shared agent mail from new sender %s: opened unclaimed account %s",
					fromAddr.Address, toAcc.ID)
			} else {
				app.Log("mail", "Shared agent mail from %s resolved to account %s", fromAddr.Address, toAcc.ID)
			}
		} else {
			var err error
			if toAcc, err = auth.GetAccount(toUsername); err != nil {
				app.Log("mail", "Recipient not found: %s", toUsername)
				continue
			}
		}

		// Create and save the message
		// Use email address as sender name for external emails
		senderName := fromAddr.Address
		if fromAddr.Name != "" {
			senderName = fromAddr.Name
		}

		app.Log("mail", "Saving message from %s to %s: %s", senderName, toAcc.Name, subject)

		// Try to find original message for threading
		var replyToID string

		// Try In-Reply-To first
		if inReplyTo != "" {
			app.Log("mail", "Looking for In-Reply-To: %s", inReplyTo)
			if origMsg := FindMessageByMessageID(inReplyTo); origMsg != nil {
				replyToID = origMsg.ID
				app.Log("mail", "✓ Threading reply using In-Reply-To: %s -> %s", inReplyTo, replyToID)
			} else {
				app.Log("mail", "✗ In-Reply-To not found: %s", inReplyTo)
			}
		}

		// If In-Reply-To didn't work, try ALL References headers
		if replyToID == "" && references != "" {
			app.Log("mail", "Trying References: %s", references)
			refs := strings.Fields(references)
			for _, ref := range refs {
				if origMsg := FindMessageByMessageID(ref); origMsg != nil {
					replyToID = origMsg.ID
					app.Log("mail", "✓ Threading reply using References: %s -> %s", ref, replyToID)
					break
				}
			}
			if replyToID == "" {
				app.Log("mail", "✗ No matching references found in: %s", references)
			}
		}

		if replyToID == "" && (inReplyTo != "" || references != "") {
			app.Log("mail", "⚠ Failed to thread message - will appear as new conversation")
		}

		// Run spam detection on inbound external mail — unless the sender is
		// the recipient's own verified address.
		//
		// Mailing your own agent is the first thing anyone does with an agent
		// that has an address, and asim@aslam.me → asim+foobar@micro.mu landed
		// in Filtered. The score does not care who you are: an address the
		// account holder proved they own scores the same as a stranger, so a
		// short message with a link in it from your own inbox reads as spam.
		//
		// Verifying an email is the strongest signal this instance has about a
		// person. Spending it on nothing was the bug.
		spamResult := SpamResult{}
		if !isOwnVerifiedAddress(toAcc, fromAddr.Address) {
			spamResult = CheckSpam(fromAddr.Address, subject, body, s.remoteIP, s.spfPass, dkimPass)
		}
		if spamResult.IsSpam {
			app.Log("mail", "Spam detected (score=%d) from %s: %v", spamResult.Score, fromAddr.Address, spamResult.Reasons)

			// Auto-block the sender domain if enabled
			sf := GetSpamFilter()
			if sf.AutoBlockDomains {
				if parts := strings.SplitN(fromAddr.Address, "@", 2); len(parts) == 2 {
					_ = BlockEmail("*@" + parts[1])
				}
			}

			if sf.RejectSpam {
				// Still save with spam flag so user can review in filtered tab
			}
		}

		if err := SendMessageTo(
			senderName,
			fromAddr.Address, // Use email as sender ID
			toAcc.Name,
			toAcc.ID,
			toTag,
			subject,
			body,
			replyToID,
			messageID,
			spamResult.IsSpam,
			spamResult.Score,
			spamResult.Reasons,
			s.remoteIP,
			rawHeaderStr,
			inboundAttachment,
		); err != nil {
			app.Log("mail", "Error saving message: %v", err)
			continue
		}

		// Mail to agent@ is filed read, because you wrote it.
		//
		// That address resolves to whoever sent it, so the message lands in the
		// sender's own inbox. Keeping it is right — it is the record of the
		// conversation and nothing should lose it — but it is not
		// correspondence waiting to be dealt with. Left unread it told you that
		// you had unread mail from yourself, and told the agent the same thing:
		// the unread count is part of every run's context, so asking a question
		// by email made the agent report on the inbox the question was in.
		if sharedAgentMail {
			if saved := FindMessageByMessageID(messageID); saved != nil {
				if err := MarkAsRead(saved.ID, toAcc.ID); err != nil {
					app.Log("mail", "could not mark agent mail read: %v", err)
				}
			}
		}

		// Anything registered for this address gets the message.
		//
		// Every agent already had an address — you+name@ — and writing to one
		// put a message in the owner's inbox and did nothing else. An agent
		// with an address that cannot answer is a mailbox with a name on it,
		// and "email your agent" is the first thing anyone tries.
		//
		// The whole rule is mayDispatch, in inbound_agent.go: not spam, not our
		// own reply coming back, the sender authenticated by SPF or DKIM, and
		// known to this account. Those last two matter — the address used to be
		// protected by nothing but being hard to guess. A handler resolves the
		// tag itself and returns quietly when it names nothing, so plain tagged
		// mail — you+receipts@ — still just files.
		deliverInbound(InboundMail{
			Owner:      toAcc.ID,
			Tag:        toTag,
			Shared:     sharedAgentMail,
			From:       fromAddr.Address,
			To:         toAddr.Address,
			FromName:   senderName,
			Subject:    subject,
			Body:       body,
			Text:       stripHTMLTags(body),
			Others:     Others(headerTo, headerCc, fromAddr.Address, toAddr.Address),
			ToAgent:    inList(headerTo, toAddr.Address),
			MessageID:  messageID,
			InReplyTo:  inReplyTo,
			References: references,
		}, wakeRequest{
			Owner:         toAcc.ID,
			Tag:           toTag,
			Shared:        sharedAgentMail,
			From:          fromAddr.Address,
			To:            toAddr.Address,
			IsSpam:        spamResult.IsSpam,
			Authenticated: dkimPass || s.spfPass,
		})
	}

	app.Log("mail", "Email processed successfully")
	return nil
}

// parseMultipart extracts text content from a multipart MIME message
// If there's only one attachment and no text body, returns the attachment content
// Recursively handles nested multipart content (e.g., multipart/related containing multipart/alternative)
func parseMultipart(body io.Reader, boundary string) (string, *Attachment) {
	return parseMultipartRecursive(body, boundary, 0)
}

// parseMultipartRecursive handles nested multipart with depth tracking to prevent infinite loops
func parseMultipartRecursive(body io.Reader, boundary string, depth int) (string, *Attachment) {
	// Prevent infinite recursion
	if depth > 5 {
		app.Log("mail", "Maximum multipart nesting depth reached")
		return "", nil
	}

	mr := multipart.NewReader(body, boundary)
	var textPlain, textHTML string
	var attachmentBody []byte
	var attachmentContentType, attachmentName string
	var allParts []string // Store all parts to avoid data loss

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		contentType := part.Header.Get("Content-Type")
		transferEncoding := part.Header.Get("Content-Transfer-Encoding")
		contentDisposition := part.Header.Get("Content-Disposition")

		// Log what we're seeing
		app.Log("mail", "MIME part (depth %d): Content-Type=%s, Transfer-Encoding=%s, Disposition=%s",
			depth, contentType, transferEncoding, contentDisposition)

		// Handle nested multipart content (multipart/alternative, multipart/related, etc.)
		if strings.Contains(contentType, "multipart/") {
			mediaType, params, err := mime.ParseMediaType(contentType)
			if err == nil && strings.HasPrefix(mediaType, "multipart/") {
				nestedBoundary := params["boundary"]
				if nestedBoundary != "" {
					app.Log("mail", "Recursively parsing nested %s (boundary: %s)", mediaType, nestedBoundary)
					nestedContent, nestedAtt := parseMultipartRecursive(part, nestedBoundary, depth+1)
					if nestedAtt != nil && len(attachmentBody) == 0 {
						attachmentBody = nestedAtt.Content
						attachmentContentType = nestedAtt.Type
						attachmentName = nestedAtt.Name
					}
					if nestedContent != "" {
						// If we got HTML from nested content, treat it as HTML
						if strings.Contains(nestedContent, "<") && strings.Contains(nestedContent, ">") {
							if textHTML == "" {
								textHTML = nestedContent
							}
						} else if textPlain == "" {
							textPlain = nestedContent
						}
					}
					continue
				}
			}
		}

		partBody, err := io.ReadAll(part)
		if err != nil {
			continue
		}

		app.Log("mail", "MIME part body size: %d bytes", len(partBody))

		// Decode based on transfer encoding
		if strings.ToLower(transferEncoding) == "base64" {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(partBody))); err == nil {
				partBody = decoded
				app.Log("mail", "Decoded base64 part (%d bytes)", len(partBody))
			}
		} else if strings.ToLower(transferEncoding) == "quoted-printable" {
			reader := quotedprintable.NewReader(bytes.NewReader(partBody))
			if decoded, err := io.ReadAll(reader); err == nil {
				partBody = decoded
				app.Log("mail", "Decoded quoted-printable part (%d bytes)", len(partBody))
			}
		}

		// Store PGP signatures with marker - don't discard any data
		if strings.Contains(contentType, "application/pgp-signature") {
			app.Log("mail", "Found PGP signature part (%d bytes)", len(partBody))
			allParts = append(allParts, fmt.Sprintf("\n\n[PGP Signature]\n%s", string(partBody)))
			continue
		}

		// Check if this is an attachment
		isAttachment := strings.Contains(contentDisposition, "attachment")

		// Prefer text/plain, fallback to text/html
		if strings.Contains(contentType, "text/plain") && !isAttachment {
			textPlain = string(partBody)
			app.Log("mail", "Found text/plain part (%d bytes)", len(partBody))
		} else if strings.Contains(contentType, "text/html") && !isAttachment {
			textHTML = string(partBody)
			app.Log("mail", "Found text/html part (%d bytes)", len(partBody))
		} else if isAttachment || strings.Contains(contentType, "application/") ||
			strings.HasPrefix(contentType, "image/") ||
			strings.HasPrefix(contentType, "audio/") ||
			strings.HasPrefix(contentType, "video/") {
			// Store attachment info (we'll only use it if there's no text body)
			attachmentBody = partBody
			attachmentContentType = contentType
			attachmentName = partFilename(part.Header.Get("Content-Disposition"), contentType)
			app.Log("mail", "Found attachment: %s (%d bytes)", contentType, len(partBody))
		} else {
			// Unknown part type - skip binary content, preserve text-like parts only
			if utf8.Valid(partBody) {
				app.Log("mail", "Unknown part type: %s (%d bytes) - preserving", contentType, len(partBody))
				allParts = append(allParts, fmt.Sprintf("\n\n[%s]\n%s", contentType, string(partBody)))
			} else {
				app.Log("mail", "Unknown part type: %s (%d bytes) - skipping (binary)", contentType, len(partBody))
			}
		}
	}

	// Build result - prefer HTML/plain text but append any extra parts
	var result string

	// Prefer HTML for rich content (images, formatting), fallback to plain text
	if textHTML != "" {
		result = strings.TrimSpace(textHTML)
	} else if textPlain != "" {
		result = strings.TrimSpace(textPlain)
	} else if len(attachmentBody) > 0 {
		// A message whose only part is an attachment — a DMARC report is the
		// common one, and it is a zip.
		//
		// This used to base64-encode the bytes into the body. The body is what
		// the inbox list previews, what the message view renders, what the
		// index searches and what an agent reads when it calls mail_inbox, so
		// one DMARC report put "UEsDBAoAAAAIAOlI/VyKomDL8AEAAMUEAAAt…" — the
		// zip's own PK header — into every one of them. It scrolled off the
		// side of the home card because fifty characters of base64 contain no
		// space to break at.
		//
		// If we cannot render it, we do not paste it in. Say what arrived.
		if utf8.Valid(attachmentBody) && !isBinaryish(attachmentContentType) {
			result = string(attachmentBody)
		} else {
			result = describeAttachment(attachmentName, attachmentContentType, len(attachmentBody))
		}
	}

	// Append any other parts we found (like PGP signatures)
	for _, part := range allParts {
		result += part
	}

	// Hand the bytes back rather than dropping them. Keeping them out of the
	// body was right; losing them was not — the message view renders a DMARC
	// report from exactly these, and describing the attachment instead left it
	// with nothing to render.
	var att *Attachment
	if len(attachmentBody) > 0 {
		att = &Attachment{Name: attachmentName, Type: attachmentContentType, Content: attachmentBody}
	}
	return result, att
}

// Reset is called when the RSET command is received
func (s *Session) Reset() {
	s.from = ""
	s.to = []string{}
}

// StartSMTPServer starts the SMTP server for RECEIVING mail only
// This is NOT an open relay - it only accepts mail for local users
func StartSMTPServer(addr string) error {
	be := &Backend{}

	s := smtpd.NewServer(be)

	s.Addr = addr
	s.Domain = ConfiguredDomain()
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024 * 10 // 10 MB
	s.MaxRecipients = 50
	s.AllowInsecureAuth = true // Allow AUTH on localhost for outbound

	// Start rate limit cleanup goroutine
	go cleanupRateLimits()

	app.Log("mail", "Starting SMTP server on %s", addr)
	app.Log("mail", "  - Inbound: Accepts mail for local users (no auth required)")
	app.Log("mail", "  - Outbound: Relays mail for authenticated users only")
	app.Log("mail", "Rate limits: %d connections/hour per IP, %d messages/day per sender",
		maxIPConnections, maxSenderMessages)

	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
		return err
	}

	return nil
}

// StartSMTPServerIfEnabled starts the SMTP server
func StartSMTPServerIfEnabled() bool {
	// Get server port from environment
	smtpServerAddr := os.Getenv("MAIL_PORT")
	if smtpServerAddr == "" {
		smtpServerAddr = ":2525" // Default to 2525 for local testing
	}

	// Add : prefix if not present
	if !strings.HasPrefix(smtpServerAddr, ":") {
		smtpServerAddr = ":" + smtpServerAddr
	}

	// Start server in goroutine
	go func() {
		if err := StartSMTPServer(smtpServerAddr); err != nil {
			app.Log("mail", "SMTP server error: %v", err)
		}
	}()

	return true
}

// checkIPRateLimit checks if an IP has exceeded connection limits
func checkIPRateLimit(ip string) bool {
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()

	// Get or create rate limit entry
	limit, exists := ipConnections[ip]
	if !exists || now.After(limit.resetTime) {
		// Create new entry or reset
		ipConnections[ip] = &ipRateLimit{
			count:     1,
			resetTime: now.Add(1 * time.Hour),
		}
		return true
	}

	// Check if limit exceeded
	if limit.count >= maxIPConnections {
		return false
	}

	// Increment counter
	limit.count++
	return true
}

// checkSenderRateLimit checks if a sender has exceeded message limits
func checkSenderRateLimit(sender string) bool {
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	now := time.Now()

	// Normalize sender email
	sender = strings.ToLower(strings.TrimSpace(sender))

	// Get or create rate limit entry
	limit, exists := senderMessages[sender]
	if !exists || now.After(limit.resetTime) {
		// Create new entry or reset
		senderMessages[sender] = &senderRateLimit{
			count:     1,
			resetTime: now.Add(24 * time.Hour),
		}
		return true
	}

	// Check if limit exceeded
	if limit.count >= maxSenderMessages {
		return false
	}

	// Increment counter
	limit.count++
	return true
}

// verifySPF performs basic SPF verification for sender domain
func verifySPF(from string, ip string) bool {
	// Extract domain from email address
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		fromAddr = &mail.Address{Address: from}
	}

	parts := strings.Split(fromAddr.Address, "@")
	if len(parts) != 2 {
		app.Log("mail", "Invalid email format for SPF check: %s", from)
		return false
	}

	domain := parts[1]

	// Look up SPF record (TXT records starting with "v=spf1")
	txtRecords, err := net.LookupTXT(domain)
	if err != nil {
		app.Log("mail", "No SPF record found for domain %s: %v", domain, err)
		return false
	}

	// Find SPF record
	var spfRecord string
	for _, record := range txtRecords {
		if strings.HasPrefix(record, "v=spf1") {
			spfRecord = record
			break
		}
	}

	if spfRecord == "" {
		app.Log("mail", "No SPF record found for domain %s", domain)
		return false
	}

	// Parse SPF record for IP matches
	// This is a simplified check - full SPF validation is complex
	tokens := strings.Fields(spfRecord)
	for _, token := range tokens {
		// Check for "ip4:" or "ip6:" matches
		if strings.HasPrefix(token, "ip4:") {
			allowedIP := strings.TrimPrefix(token, "ip4:")
			if strings.Contains(allowedIP, "/") {
				_, cidr, err := net.ParseCIDR(allowedIP)
				if err == nil && cidr.Contains(net.ParseIP(ip)) {
					app.Log("mail", "SPF passed: IP %s matches CIDR %s", ip, allowedIP)
					return true
				}
			} else if allowedIP == ip {
				app.Log("mail", "SPF passed: IP %s matches %s", ip, allowedIP)
				return true
			}
		}
		if strings.HasPrefix(token, "ip6:") {
			allowedIP := strings.TrimPrefix(token, "ip6:")
			if strings.Contains(allowedIP, "/") {
				_, cidr, err := net.ParseCIDR(allowedIP)
				if err == nil && cidr.Contains(net.ParseIP(ip)) {
					app.Log("mail", "SPF passed: IP %s matches CIDR %s", ip, allowedIP)
					return true
				}
			} else if allowedIP == ip {
				app.Log("mail", "SPF passed: IP %s matches %s", ip, allowedIP)
				return true
			}
		}
		// "a" mechanism - domain's A record should match
		if token == "a" {
			ips, err := net.LookupIP(domain)
			if err == nil {
				for _, domainIP := range ips {
					if domainIP.String() == ip {
						app.Log("mail", "SPF passed: IP %s matches domain A record", ip)
						return true
					}
				}
			}
		}
		// "all" mechanism - check qualifier
		if token == "+all" || token == "~all" || token == "?all" {
			app.Log("mail", "SPF permissive policy: %s", token)
			return true
		}
		if token == "-all" || token == "all" {
			app.Log("mail", "SPF hard fail: IP %s not authorized by %s (policy: %s)", ip, domain, token)
			return false
		}
	}

	app.Log("mail", "SPF check failed for %s from IP %s (record: %s)", from, ip, spfRecord)
	return false
}

// cleanupRateLimits periodically removes old rate limit entries
func cleanupRateLimits() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rateLimitMutex.Lock()

		now := time.Now()

		// Cleanup IP connections
		for ip, limit := range ipConnections {
			if now.After(limit.resetTime) {
				delete(ipConnections, ip)
			}
		}

		// Cleanup sender messages
		for sender, limit := range senderMessages {
			if now.After(limit.resetTime) {
				delete(senderMessages, sender)
			}
		}

		ipCount := len(ipConnections)
		senderCount := len(senderMessages)

		rateLimitMutex.Unlock()

		app.Log("mail", "Cleaned up rate limit entries (IPs: %d, Senders: %d)",
			ipCount, senderCount)
	}
}

// partFilename is the attachment's name, from Content-Disposition if it has
// one and from the Content-Type name parameter otherwise. Empty when neither
// says — plenty of senders do not.
func partFilename(disposition, contentType string) string {
	for _, h := range []string{disposition, contentType} {
		if h == "" {
			continue
		}
		if _, params, err := mime.ParseMediaType(h); err == nil {
			for _, k := range []string{"filename", "name"} {
				if v := strings.TrimSpace(params[k]); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// isBinaryish reports whether a content type is one we should never inline,
// even if its bytes happen to be valid UTF-8. A small zip can be.
func isBinaryish(contentType string) bool {
	ct := strings.ToLower(contentType)
	for _, prefix := range []string{"image/", "audio/", "video/", "font/"} {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	for _, frag := range []string{"zip", "gzip", "octet-stream", "pdf", "msword",
		"spreadsheet", "presentation", "x-tar", "x-7z", "x-rar"} {
		if strings.Contains(ct, frag) {
			return true
		}
	}
	return false
}

// singlePartBody turns a message that is not multipart into what to show and
// what to keep beside it.
//
// Text is the body. Anything else is described in the body and handed back, and
// the second half is what was missing: this path described the attachment and
// dropped the bytes, while the multipart path returned them. A DMARC report
// that arrives as a single part — Content-Type: application/zip, no text
// alongside it — therefore became a line saying a zip had come, with nothing
// behind it for the message view to render as a table.
//
// Indistinguishable from the outside, which is why it survived: a report that
// renders nothing looks the same whether the renderer is broken or the bytes
// were never stored.
func singlePartBody(bodyBytes []byte, contentType string) (string, *Attachment) {
	if isValidUTF8Text(bodyBytes) {
		return string(bodyBytes), nil
	}
	name := partFilename("", contentType)
	app.Log("mail", "Binary body (%d bytes, %s) described rather than inlined, bytes kept",
		len(bodyBytes), contentType)
	return describeAttachment(name, contentType, len(bodyBytes)),
		&Attachment{Name: name, Type: contentType, Content: append([]byte(nil), bodyBytes...)}
}

// describeAttachment is what the body says when the only thing in the message
// is something we cannot show. One line, readable in a preview, and truthful
// about the fact that the content is not here.
func describeAttachment(name, contentType string, size int) string {
	if name == "" {
		name = "attachment"
	}
	kind, _, err := mime.ParseMediaType(contentType)
	if err != nil || kind == "" {
		kind = strings.TrimSpace(contentType)
	}
	desc := name
	if kind != "" {
		desc += " (" + kind + ")"
	}
	return "[" + desc + ", " + humanSize(size) + " — not shown]"
}

func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}

// InboundMail is a message that arrived for a tagged address.
type InboundMail struct {
	Owner string // account the address belongs to
	Tag   string // the part after the plus: you+<tag>@; empty when Shared

	// Shared marks mail that arrived at agent@<domain> rather than at one of
	// the account's own agents. Tag then names one of this instance's agents
	// rather than one of theirs — two namespaces, see agent/platform.go — and
	// an empty tag is the catch-all.
	Shared bool

	From     string // who wrote in
	To       string // the address they wrote to, which is what answers them
	FromName string
	Subject  string

	// Body is the message as it arrived, HTML preferred, because that is what
	// the inbox renders.
	//
	// Text is the same message as prose. A handler that hands a message to
	// something other than a browser wants this one: the agent was being given
	// `<div dir="auto">What&#39;s happening </div>` as the question, which is
	// what it answered, and the same string became the conversation's name in
	// the record. Markup is not what somebody wrote — it is how their client
	// chose to send it.
	Body string
	Text string

	// Others is everybody else on the message: the people who will read the
	// reply besides the sender, with this instance's own addresses removed.
	//
	// Empty for the ordinary case — one person writing to their agent — and
	// non-empty exactly when the agent has been put into a conversation that
	// already had people in it. That is the whole difference between answering
	// somebody and joining a thread, and everything downstream keys on it. See
	// cc.go.
	Others []string

	// ToAgent is whether the agent's address was in To rather than only Cc.
	// Being in To is somebody speaking to it; being in Cc is somebody adding it
	// to what they were already saying to each other.
	ToAgent bool

	MessageID string // for threading the reply

	// InReplyTo and References are what the sender says this message answers.
	// Passed through rather than consumed here, because whether it continues
	// something is a question about the conversation and mail does not hold
	// one — the handler matches them against turns it has already answered.
	InReplyTo  string
	References string
}

// Handlers register with Inbound, in inbound.go. There used to be a function
// variable here for the agent specifically.
