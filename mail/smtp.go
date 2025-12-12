package mail

import (
	"bytes"
	"io"
	"log"
	"net/mail"
	"os"
	"strings"
	"time"

	"mu/app"
	"mu/auth"

	"github.com/emersion/go-smtp"
)

// Backend implements SMTP server backend for RECEIVING mail only
// This is NOT an open relay - it only accepts mail for local users
type Backend struct{}

// NewSession creates a new SMTP session
// No authentication required - this server only RECEIVES mail
func (bkd *Backend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	return &Session{}, nil
}

// Session represents an SMTP session for RECEIVING mail
type Session struct {
	from string
	to   []string
}

// Mail is called when the MAIL FROM command is received
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.from = from
	app.Log("smtp", "Mail from: %s", from)
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
		app.Log("smtp", "Rejected mail for non-existent user: %s", username)
		return &smtp.SMTPError{
			Code:    550,
			Message: "User not found",
		}
	}

	s.to = append(s.to, to)
	app.Log("smtp", "Accepting mail for local user: %s", username)
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
		app.Log("smtp", "Error parsing email: %v", err)
		return err
	}

	// Extract headers
	from := msg.Header.Get("From")
	subject := msg.Header.Get("Subject")

	// Parse sender email
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		fromAddr = &mail.Address{Address: from}
	}

	// Read body
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		app.Log("smtp", "Error reading body: %v", err)
		return err
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
			app.Log("smtp", "Recipient not found: %s", toUsername)
			continue
		}

		// Create and save the message
		// Use email address as sender name for external emails
		senderName := fromAddr.Address
		if fromAddr.Name != "" {
			senderName = fromAddr.Name
		}

		app.Log("smtp", "Saving message from %s to %s: %s", senderName, toAcc.Name, subject)

		if err := SendMessage(
			senderName,
			fromAddr.Address, // Use email as sender ID
			toAcc.Name,
			toAcc.ID,
			subject,
			string(body),
			"",
		); err != nil {
			app.Log("smtp", "Error saving message: %v", err)
			continue
		}
	}

	app.Log("smtp", "Email processed successfully")
	return nil
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
	s.Domain = "localhost"
	s.ReadTimeout = 10 * time.Second
	s.WriteTimeout = 10 * time.Second
	s.MaxMessageBytes = 1024 * 1024 * 10 // 10 MB
	s.MaxRecipients = 50
	s.AllowInsecureAuth = false // No auth needed for receiving

	app.Log("smtp", "Starting SMTP server (receive only) on %s", addr)

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
		app.Log("smtp", "SMTP server disabled (set SMTP_ENABLED=true to enable)")
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
			app.Log("smtp", "SMTP server error: %v", err)
		}
	}()

	return true
}
