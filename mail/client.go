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

	"mu/app"

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

// SendExternalEmail sends an email to an external address via direct relay
// Sends multipart/alternative with both plain text and HTML versions (like Gmail)
// Returns the generated Message-ID for threading purposes
func SendExternalEmail(displayName, from, to, subject, bodyPlain, bodyHTML string, replyToMsgID string) (string, error) {
	// Extract username from email for Message-ID
	username := from
	if strings.Contains(from, "@") {
		username = strings.Split(from, "@")[0]
	}

	// Generate unique Message-ID for threading
	messageID := fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), username, GetConfiguredDomain())

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
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))

	if replyToMsgID != "" {
		msg.WriteString(fmt.Sprintf("In-Reply-To: %s\r\n", replyToMsgID))
		msg.WriteString(fmt.Sprintf("References: %s\r\n", replyToMsgID))
	}

	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	msg.WriteString("\r\n")

	// Plain text part
	msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	msg.WriteString("Content-Transfer-Encoding: 7bit\r\n")
	msg.WriteString("\r\n")
	bodyPlain = strings.ReplaceAll(bodyPlain, "\r\n", "\n")
	bodyPlain = strings.ReplaceAll(bodyPlain, "\n", "\r\n")
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

	message := msg.Bytes()

	// Apply DKIM signing if configured
	if dkimConfig != nil {
		options := &dkim.SignOptions{
			Domain:   dkimConfig.Domain,
			Selector: dkimConfig.Selector,
			Signer:   dkimConfig.PrivateKey,
		}

		var signedBuf bytes.Buffer
		if err := dkim.Sign(&signedBuf, bytes.NewReader(message), options); err != nil {
			app.Log("dkim", "WARNING: DKIM signing failed: %v", err)
		} else {
			message = signedBuf.Bytes()
			app.Log("dkim", "✓ Email signed with DKIM successfully")
		}
	}

	app.Log("mail", "=== Direct Relay (Internal) ===")
	app.Log("mail", "From: %s <%s>", displayName, from)
	app.Log("mail", "To: %s", to)
	app.Log("mail", "Subject: %s", subject)
	app.Log("mail", "Message-ID: %s", messageID)
	app.Log("mail", "Message headers preview: %s", string(message[:min(500, len(message))]))

	// Call relay function directly (no SMTP needed!)
	if err := RelayToExternal(from, to, message); err != nil {
		app.Log("mail", "✗ Failed to relay email: %v", err)
		return "", fmt.Errorf("failed to relay email: %v", err)
	}

	app.Log("mail", "✓ Email relayed successfully")
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
func GetConfiguredDomain() string {
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
