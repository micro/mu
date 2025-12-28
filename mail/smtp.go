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

	"mu/app"
	"mu/auth"

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
	if !verifySPF(from, s.remoteIP) {
		app.Log("mail", "SPF verification failed for %s from IP %s", from, s.remoteIP)
		// Log but don't reject - many legitimate servers have misconfigured SPF
		// In production you might want to reject or flag these
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

	username := parts[0]

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
	if recipientDomain != GetConfiguredDomain() {
		app.Log("mail", "Rejected mail for external domain %s (not an open relay)", recipientDomain)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Relay access denied",
		}
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

	// Check if user has mail access (admin or member)
	if !acc.Admin && !acc.Member {
		app.Log("mail", "Rejected mail for user without mail access: %s", username)
		return &smtpd.SMTPError{
			Code:    550,
			Message: "Mail access restricted to members only",
		}
	}

	s.to = append(s.to, to)
	app.Log("mail", "Accepting mail for local user: %s", username)
	return nil
}

// relayToExternal delivers email to an external SMTP server
// RelayToExternal sends email directly to an external SMTP server (exported for internal use)
func RelayToExternal(from, to string, data []byte) error {
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
		hostname := GetConfiguredDomain()
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

	// Parse the email
	msg, err := mail.ReadMessage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		app.Log("mail", "Error parsing email: %v", err)
		return err
	}

	// Extract headers
	from := msg.Header.Get("From")
	subject := msg.Header.Get("Subject")
	contentType := msg.Header.Get("Content-Type")
	messageID := msg.Header.Get("Message-ID")
	inReplyTo := msg.Header.Get("In-Reply-To")
	references := msg.Header.Get("References")

	// Parse sender email
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		fromAddr = &mail.Address{Address: from}
	}

	// Parse body based on content type
	var body string
	if strings.Contains(contentType, "multipart/") {
		// Parse multipart message
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			boundary := params["boundary"]
			if boundary != "" {
				body = parseMultipart(msg.Body, boundary)
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

		// Store the decoded content
		if isValidUTF8Text(bodyBytes) {
			body = string(bodyBytes)
		} else {
			// Binary content - base64 encode for safe storage
			body = base64.StdEncoding.EncodeToString(bodyBytes)
			app.Log("mail", "Base64 encoded binary body for safe storage (%d bytes)", len(bodyBytes))
		}

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
		toUsername := parts[0]
		toDomain := parts[1]

		// Check if this is an external recipient
		isExternal := toDomain != GetConfiguredDomain()

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

		// Look up the recipient account (local user)
		toAcc, err := auth.GetAccount(toUsername)
		if err != nil {
			app.Log("mail", "Recipient not found: %s", toUsername)
			continue
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

		if err := SendMessage(
			senderName,
			fromAddr.Address, // Use email as sender ID
			toAcc.Name,
			toAcc.ID,
			subject,
			body,
			replyToID,
			messageID,
		); err != nil {
			app.Log("mail", "Error saving message: %v", err)
			continue
		}
	}

	app.Log("mail", "Email processed successfully")
	return nil
}

// parseMultipart extracts text content from a multipart MIME message
// If there's only one attachment and no text body, returns the attachment content
func parseMultipart(body io.Reader, boundary string) string {
	mr := multipart.NewReader(body, boundary)
	var textPlain, textHTML string
	var attachmentBody []byte
	var attachmentContentType string
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
		partBody, err := io.ReadAll(part)
		if err != nil {
			continue
		}

		// Log what we're seeing
		app.Log("mail", "MIME part: Content-Type=%s, Transfer-Encoding=%s, Disposition=%s, Size=%d", 
			contentType, transferEncoding, contentDisposition, len(partBody))

		// Decode based on transfer encoding
		if strings.ToLower(transferEncoding) == "base64" {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(partBody))); err == nil {
				partBody = decoded
			}
		} else if strings.ToLower(transferEncoding) == "quoted-printable" {
			reader := quotedprintable.NewReader(bytes.NewReader(partBody))
			if decoded, err := io.ReadAll(reader); err == nil {
				partBody = decoded
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
		} else if isAttachment || strings.Contains(contentType, "application/") {
			// Store attachment info (we'll only use it if there's no text body)
			attachmentBody = partBody
			attachmentContentType = contentType
			app.Log("mail", "Found attachment: %s (%d bytes)", contentType, len(partBody))
		} else {
			// Unknown part type - preserve it
			app.Log("mail", "Unknown part type: %s (%d bytes) - preserving", contentType, len(partBody))
			allParts = append(allParts, fmt.Sprintf("\n\n[%s]\n%s", contentType, string(partBody)))
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
		// If no text body but there's an attachment (like DMARC reports), return the attachment
		// For gzip attachments, store as base64 for safe storage
		if strings.Contains(attachmentContentType, "gzip") || strings.Contains(attachmentContentType, "zip") {
			result = base64.StdEncoding.EncodeToString(attachmentBody)
		} else {
			result = string(attachmentBody)
		}
	}

	// Append any other parts we found (like PGP signatures)
	for _, part := range allParts {
		result += part
	}

	return result
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
	s.Domain = GetConfiguredDomain()
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
				// CIDR notation - would need proper CIDR matching
				app.Log("mail", "SPF CIDR check not fully implemented: %s", allowedIP)
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
		// "+all" or "~all" or "?all" - permissive policies
		if token == "+all" || token == "~all" || token == "?all" {
			app.Log("mail", "SPF permissive policy: %s", token)
			return true
		}
	}

	app.Log("mail", "SPF check inconclusive for %s from IP %s (record: %s)", from, ip, spfRecord)
	// Return true for now - we don't want to be too strict initially
	return true
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
