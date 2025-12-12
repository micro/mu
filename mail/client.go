package mail

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mu/app"

	"github.com/emersion/go-msgauth/dkim"
)

// SMTPConfig holds the SMTP client configuration
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

// DKIMConfig holds DKIM signing configuration
type DKIMConfig struct {
	Domain     string
	Selector   string
	PrivateKey *rsa.PrivateKey
}

// Global SMTP config - can be configured via environment variables
var smtpConfig = &SMTPConfig{
	Host: "localhost",
	Port: "25",
}

// ConfigureSMTP updates SMTP client configuration from environment variables
func ConfigureSMTP() {
	if host := os.Getenv("SMTP_HOST"); host != "" {
		smtpConfig.Host = host
	}
	if port := os.Getenv("SMTP_PORT"); port != "" {
		smtpConfig.Port = port
	}
	if user := os.Getenv("SMTP_USERNAME"); user != "" {
		smtpConfig.Username = user
	}
	if pass := os.Getenv("SMTP_PASSWORD"); pass != "" {
		smtpConfig.Password = pass
	}
	app.Log("mail", "SMTP client configured: %s:%s", smtpConfig.Host, smtpConfig.Port)
}

// Global DKIM config - optional, auto-loaded if keys exist
var dkimConfig *DKIMConfig

// LoadDKIMConfig loads DKIM configuration from files in ~/.mu/keys/
// Keys should be named dkim.key (private) and dkim.pub (public, optional)
// Domain defaults to "localhost" if not specified
func LoadDKIMConfig(domain, selector string) error {
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
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read DKIM key: %v", err)
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

// SendExternalEmail sends an email to an external address via SMTP with optional DKIM signing
// Returns the generated Message-ID for threading purposes
func SendExternalEmail(from, fromEmail, to, subject, body, replyToMsgID string) (string, error) {
	// Generate unique Message-ID
	messageID := fmt.Sprintf("<%d@%s>", time.Now().UnixNano(), GetConfiguredDomain())

	// Construct the email message
	var msgBuf bytes.Buffer
	msgBuf.WriteString(fmt.Sprintf("From: %s <%s>\r\n", from, fromEmail))
	msgBuf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msgBuf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msgBuf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msgBuf.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))

	// Add In-Reply-To and References if this is a reply
	if replyToMsgID != "" {
		if origMsg := FindMessageByMessageID(replyToMsgID); origMsg != nil {
			if origMsg.MessageID != "" {
				msgBuf.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", origMsg.MessageID))
				msgBuf.WriteString(fmt.Sprintf("References: %s\r\n", origMsg.MessageID))
			}
		} else if origMsg := GetMessage(replyToMsgID); origMsg != nil && origMsg.MessageID != "" {
			// Try looking up by our internal ID
			msgBuf.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", origMsg.MessageID))
			msgBuf.WriteString(fmt.Sprintf("References: %s\r\n", origMsg.MessageID))
		}
	}

	msgBuf.WriteString("MIME-Version: 1.0\r\n")
	msgBuf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msgBuf.WriteString("\r\n")
	msgBuf.WriteString(body)
	msgBuf.WriteString("\r\n")

	message := msgBuf.Bytes()

	// Sign with DKIM if configured
	if dkimConfig != nil {
		options := &dkim.SignOptions{
			Domain:   dkimConfig.Domain,
			Selector: dkimConfig.Selector,
			Signer:   dkimConfig.PrivateKey,
		}

		var signedBuf bytes.Buffer
		if err := dkim.Sign(&signedBuf, bytes.NewReader(message), options); err != nil {
			app.Log("dkim", "Warning: DKIM signing failed: %v", err)
			// Continue without DKIM signature
		} else {
			message = signedBuf.Bytes()
			app.Log("dkim", "Email signed with DKIM")
		}
	}

	// Connect to the SMTP server
	addr := smtpConfig.Host + ":" + smtpConfig.Port

	app.Log("mail", "Sending email from %s to %s via %s", fromEmail, to, addr)

	// Send the email
	// For localhost or internal relay, we don't need authentication
	err := smtp.SendMail(addr, nil, fromEmail, []string{to}, message)
	if err != nil {
		app.Log("mail", "Error sending email: %v", err)
		return "", fmt.Errorf("failed to send email: %v", err)
	}

	app.Log("mail", "Email sent successfully to %s (server accepted)", to)
	return messageID, nil
}

// IsExternalEmail checks if an email address is external (contains @domain)
func IsExternalEmail(email string) bool {
	return strings.Contains(email, "@")
}

// GetEmailForUser generates an email address for a local user
func GetEmailForUser(username, domain string) string {
	if domain == "" {
		domain = "localhost"
	}
	return username + "@" + domain
}

// GetConfiguredDomain returns the configured mail domain
// Checks MAIL_DOMAIN first, falls back to DKIM_DOMAIN, then "localhost"
func GetConfiguredDomain() string {
	domain := os.Getenv("MAIL_DOMAIN")
	if domain == "" {
		// Fallback to DKIM_DOMAIN for backward compatibility
		domain = os.Getenv("DKIM_DOMAIN")
	}
	if domain == "" {
		domain = "localhost"
	}
	return domain
}
