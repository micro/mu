package mail

import (
	"bytes"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"

	"github.com/emersion/go-smtp"
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

// NewSession creates a new SMTP session
// No authentication required - this server only RECEIVES mail
func (bkd *Backend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	// Extract IP address
	remoteAddr := conn.Conn().RemoteAddr().String()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}

	// Check rate limit for this IP
	if !checkIPRateLimit(ip) {
		app.Log("mail", "Rate limit exceeded for IP: %s", ip)
		return nil, &smtp.SMTPError{
			Code:    421,
			Message: "Too many connections from your IP. Please try again later.",
		}
	}

	app.Log("mail", "New SMTP session from IP: %s", ip)
	return &Session{remoteIP: ip}, nil
}

// Session represents an SMTP session for RECEIVING mail
type Session struct {
	from     string
	to       []string
	remoteIP string
}

// Mail is called when the MAIL FROM command is received
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	app.Log("mail", "Mail from: %s (IP: %s)", from, s.remoteIP)

	// Check blocklist first
	if IsBlocked(from, s.remoteIP) {
		app.Log("mail", "Rejected blocked sender: %s (IP: %s)", from, s.remoteIP)
		return &smtp.SMTPError{
			Code:    554,
			Message: "Transaction failed: sender blocked",
		}
	}

	// Check sender rate limit
	if !checkSenderRateLimit(from) {
		app.Log("mail", "Rate limit exceeded for sender: %s", from)
		return &smtp.SMTPError{
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

// Rcpt is called when the RCPT TO command is received
// Validates that the recipient is a local user
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	// Extract username from email address
	toAddr, err := mail.ParseAddress(to)
	if err != nil {
		toAddr = &mail.Address{Address: to}
	}

	// Get username (part before @)
	parts := strings.Split(toAddr.Address, "@")
	if len(parts) == 0 {
		return &smtp.SMTPError{
			Code:    550,
			Message: "Invalid recipient address",
		}
	}

	username := parts[0]

	// Verify the user exists in our system
	_, err = auth.GetAccount(username)
	if err != nil {
		app.Log("mail", "Rejected mail for non-existent user: %s", username)
		return &smtp.SMTPError{
			Code:    550,
			Message: "User not found",
		}
	}

	s.to = append(s.to, to)
	app.Log("mail", "Accepting mail for local user: %s", username)
	return nil
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
		body = string(bodyBytes)
	}

	// Process each recipient
	for _, recipient := range s.to {
		// Parse recipient email
		toAddr, err := mail.ParseAddress(recipient)
		if err != nil {
			toAddr = &mail.Address{Address: recipient}
		}

		// Extract username from email (everything before @)
		toUsername := strings.Split(toAddr.Address, "@")[0]

		// Look up the recipient account
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
		if inReplyTo != "" {
			if origMsg := FindMessageByMessageID(inReplyTo); origMsg != nil {
				replyToID = origMsg.ID
				app.Log("mail", "Threading reply using In-Reply-To: %s", inReplyTo)
			}
		}
		// If In-Reply-To didn't match, try References (last one is usually the direct parent)
		if replyToID == "" && references != "" {
			refs := strings.Fields(references)
			for i := len(refs) - 1; i >= 0; i-- {
				if origMsg := FindMessageByMessageID(refs[i]); origMsg != nil {
					replyToID = origMsg.ID
					app.Log("mail", "Threading reply using References: %s", refs[i])
					break
				}
			}
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
func parseMultipart(body io.Reader, boundary string) string {
	mr := multipart.NewReader(body, boundary)
	var textPlain, textHTML string

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		contentType := part.Header.Get("Content-Type")
		partBody, err := io.ReadAll(part)
		if err != nil {
			continue
		}

		// Prefer text/plain, fallback to text/html
		if strings.Contains(contentType, "text/plain") {
			textPlain = string(partBody)
		} else if strings.Contains(contentType, "text/html") {
			textHTML = string(partBody)
		}
	}

	// Return text/plain if available, otherwise text/html
	if textPlain != "" {
		return strings.TrimSpace(textPlain)
	}
	if textHTML != "" {
		return strings.TrimSpace(textHTML)
	}

	return ""
}

// Reset is called when the RSET command is received
func (s *Session) Reset() {
	s.from = ""
	s.to = []string{}
}

// Logout is called when the connection is closed
func (s *Session) Logout() error {
	return nil
}

// StartSMTPServer starts the SMTP server for RECEIVING mail only
// This is NOT an open relay - it only accepts mail for local users
func StartSMTPServer(addr string) error {
	be := &Backend{}

	s := smtp.NewServer(be)

	s.Addr = addr
	s.Domain = GetConfiguredDomain()
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024 * 10 // 10 MB
	s.MaxRecipients = 50
	s.AllowInsecureAuth = false // No auth needed for receiving

	// Start rate limit cleanup goroutine
	go cleanupRateLimits()

	app.Log("mail", "Starting SMTP server (receive only) on %s", addr)
	app.Log("mail", "Rate limits: %d connections/hour per IP, %d messages/day per sender",
		maxIPConnections, maxSenderMessages)

	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
		return err
	}

	return nil
}

// StartSMTPServerIfEnabled starts the SMTP server if enabled via environment variable
// Returns true if server was started, false if disabled
func StartSMTPServerIfEnabled() bool {
	// Check if SMTP is enabled
	smtpEnabled := os.Getenv("SMTP_ENABLED")
	if smtpEnabled == "" || smtpEnabled == "false" || smtpEnabled == "0" {
		app.Log("mail", "SMTP server disabled (set SMTP_ENABLED=true to enable)")
		return false
	}

	// Get server port from environment
	smtpServerAddr := os.Getenv("SMTP_SERVER_PORT")
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
