package mail

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	stdhtml "html"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"
)

var mutex sync.RWMutex

// encryption key derived from environment or default
var encryptionKey []byte

func init() {
	// Get encryption key from environment or generate a default one
	keyStr := os.Getenv("MAIL_ENCRYPTION_KEY")
	if keyStr == "" {
		// Use a default key for development (in production, this should be set via env)
		keyStr = "mu-mail-encryption-key-change-me-in-production"
	}
	// Derive a 32-byte key from the string
	hash := sha256.Sum256([]byte(keyStr))
	encryptionKey = hash[:]
}

// Message represents a mail message
// Body is stored encrypted on the server
type Message struct {
	ID              string    `json:"id"`
	From            string    `json:"from"`
	To              string    `json:"to"`
	Subject         string    `json:"subject"`
	EncryptedBody   string    `json:"encrypted_body"`   // Base64 encoded encrypted body
	Sent            time.Time `json:"sent"`
	Read            bool      `json:"read"`
	DeleteOnRead    bool      `json:"delete_on_read"`   // Auto-delete when marked as read
}

// all messages stored by recipient
var messages = map[string][]*Message{}

// cached html for pages
var html string

var ComposeTemplate = `
<div id="mail">
  <h1>Compose Mail</h1>
  <form action="/mail" method="POST">
    <input type="text" name="to" placeholder="To (username)" required><br>
    <input type="text" name="subject" placeholder="Subject" required><br>
    <textarea name="body" rows="10" placeholder="Message" required style="width: calc(100%% - 60px); padding: 10px; border-radius: 5px; border: 1px solid darkgrey;"></textarea><br>
    <button type="submit">Send</button>
  </form>
  <br>
  <a href="/mail" class="link">Back to Inbox</a>
</div>
`

func init() {
	// Get encryption key from environment or generate a default one
	keyStr := os.Getenv("MAIL_ENCRYPTION_KEY")
	if keyStr == "" {
		// Use a default key for development (in production, this should be set via env)
		keyStr = "mu-mail-encryption-key-change-me-in-production"
	}
	// Derive a 32-byte key from the string
	hash := sha256.Sum256([]byte(keyStr))
	encryptionKey = hash[:]

	// load messages from disk
	b, _ := data.Load("messages.json")
	if b != nil {
		json.Unmarshal(b, &messages)
	}
}

// encryptBody encrypts the message body using AES-GCM
func encryptBody(plaintext string) (string, error) {
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptBody decrypts the message body using AES-GCM
func decryptBody(encrypted string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Load initializes the mail system
func Load() {
	fmt.Println("Mail system loaded")
	
	// Start background task to auto-delete old unread messages
	go autoDeleteOldMessages()
}

// autoDeleteOldMessages runs periodically to delete unread messages older than 24 hours
func autoDeleteOldMessages() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	
	for range ticker.C {
		mutex.Lock()
		
		now := time.Now()
		deletedCount := 0
		
		for username, inbox := range messages {
			filtered := []*Message{}
			for _, msg := range inbox {
				// Delete if unread and older than 24 hours
				if !msg.Read && now.Sub(msg.Sent) > 24*time.Hour {
					deletedCount++
					fmt.Printf("Auto-deleted unread message %s (sent to %s) after 24 hours\n", msg.ID, username)
				} else {
					filtered = append(filtered, msg)
				}
			}
			messages[username] = filtered
		}
		
		if deletedCount > 0 {
			data.SaveJSON("messages.json", messages)
			fmt.Printf("Auto-deleted %d unread messages older than 24 hours\n", deletedCount)
		}
		
		mutex.Unlock()
	}
}

// SendMessage sends a message from one user to another
// The message body is encrypted before storage
func SendMessage(from, to, subject, body string) error {
	// Check if recipient exists before sending
	if !auth.AccountExists(to) {
		return fmt.Errorf("recipient user '%s' does not exist", to)
	}

	// Encrypt the message body
	encryptedBody, err := encryptBody(body)
	if err != nil {
		return fmt.Errorf("failed to encrypt message: %v", err)
	}

	mutex.Lock()
	defer mutex.Unlock()

	// create message
	msg := &Message{
		ID:            fmt.Sprintf("%s-%d", from, time.Now().UnixNano()),
		From:          from,
		To:            to,
		Subject:       subject,
		EncryptedBody: encryptedBody,
		Sent:          time.Now(),
		Read:          false,
		DeleteOnRead:  true, // Enable auto-delete on read
	}

	// add to recipient's inbox
	if messages[to] == nil {
		messages[to] = []*Message{}
	}
	messages[to] = append(messages[to], msg)

	// save to disk
	data.SaveJSON("messages.json", messages)

	// NOTE: We intentionally do NOT index mail messages for security reasons.
	// Mail is private communication between users and should not be searchable
	// in a shared index that could potentially leak private data.
	// Messages are encrypted at rest and auto-deleted when read.

	return nil
}

// GetInbox retrieves all messages for a user and decrypts them
func GetInbox(username string) []*Message {
	mutex.RLock()
	defer mutex.RUnlock()

	inbox := messages[username]
	if inbox == nil {
		return []*Message{}
	}

	// sort by most recent first
	sorted := make([]*Message, len(inbox))
	copy(sorted, inbox)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Sent.After(sorted[j].Sent)
	})

	return sorted
}

// GetDecryptedBody decrypts and returns the body of a message
func GetDecryptedBody(msg *Message) string {
	if msg.EncryptedBody == "" {
		return ""
	}
	body, err := decryptBody(msg.EncryptedBody)
	if err != nil {
		fmt.Printf("Error decrypting message %s: %v\n", msg.ID, err)
		return "[Error decrypting message]"
	}
	return body
}

// MarkAsRead marks a message as read and deletes it if DeleteOnRead is true
func MarkAsRead(username, messageID string) {
	mutex.Lock()
	defer mutex.Unlock()

	inbox := messages[username]
	for i, msg := range inbox {
		if msg.ID == messageID {
			if msg.DeleteOnRead {
				// Store-and-forward: Delete the message from server after reading
				messages[username] = append(inbox[:i], inbox[i+1:]...)
				fmt.Printf("Message %s deleted from server after being read by %s\n", messageID, username)
			} else {
				// Legacy behavior: just mark as read
				msg.Read = true
			}
			data.SaveJSON("messages.json", messages)
			break
		}
	}
}

// DeleteMessage allows a user to delete their own message
// This only works for messages they received, not messages they sent
func DeleteMessage(username, messageID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	inbox := messages[username]
	for i, msg := range inbox {
		if msg.ID == messageID {
			// Remove the message
			messages[username] = append(inbox[:i], inbox[i+1:]...)
			data.SaveJSON("messages.json", messages)
			return nil
		}
	}
	return fmt.Errorf("message not found")
}

// GetUnreadCount returns the number of unread messages
func GetUnreadCount(username string) int {
	mutex.RLock()
	defer mutex.RUnlock()

	inbox := messages[username]
	count := 0
	for _, msg := range inbox {
		if !msg.Read {
			count++
		}
	}
	return count
}

// LatestMail returns HTML for the latest mail status
func LatestMail(username string) string {
	unread := GetUnreadCount(username)
	if unread > 0 {
		return fmt.Sprintf(`<span style="font-weight: bold;">You've got mail! (%d new)</span>`, unread)
	}
	return "No new messages"
}

// Handler handles mail requests
func Handler(w http.ResponseWriter, r *http.Request) {
	// get the session
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}

	username := sess.Account

	// POST - send a new message
	if r.Method == "POST" {
		r.ParseForm()

		// check if it's a compose or mark as read action
		action := r.Form.Get("action")
		
		if action == "read" {
			messageID := r.Form.Get("id")
			MarkAsRead(username, messageID)
			http.Redirect(w, r, "/mail", 302)
			return
		}

		// it's a send message action
		to := r.Form.Get("to")
		subject := r.Form.Get("subject")
		body := r.Form.Get("body")

		if len(to) == 0 || len(subject) == 0 || len(body) == 0 {
			http.Error(w, "Missing required fields", 400)
			return
		}

		err := SendMessage(username, to, subject, body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		// redirect back to inbox
		http.Redirect(w, r, "/mail", 302)
		return
	}

	// GET - show inbox or compose form
	if r.URL.Query().Get("compose") == "true" {
		// Check if this is a reply
		replyTo := r.URL.Query().Get("reply_to")
		subject := r.URL.Query().Get("subject")
		originalBody := r.URL.Query().Get("original")
		
		composeHTML := `<div id="mail">
  <div class="mail-compose">
    <h1>New Message</h1>`
		
		// Show original message if it's a reply
		if originalBody != "" {
			composeHTML += `
    <div class="mail-compose-original">
      <div class="mail-compose-original-label">Original message:</div>
      ` + stdhtml.EscapeString(originalBody) + `
    </div>`
		}
		
		composeHTML += `
    <form action="/mail" method="POST">
      <input type="text" name="to" placeholder="To" value="` + stdhtml.EscapeString(replyTo) + `" required>
      <input type="text" name="subject" placeholder="Subject" value="` + stdhtml.EscapeString(subject) + `" required>
      <textarea name="body" placeholder="Type your message here..." required></textarea>
      <button type="submit">Send</button>
      <a href="/mail" class="mail-compose-back">Cancel</a>
    </form>
  </div>
</div>`
		
		html := app.RenderHTML("Compose Mail", "Send a message", composeHTML)
		w.Write([]byte(html))
		return
	}

	// Check if JSON response is requested via query parameter
	if r.URL.Query().Get("format") == "json" {
		inbox := GetInbox(username)
		
		// Create response with decrypted bodies for client-side search
		type MessageResponse struct {
			ID       string    `json:"id"`
			From     string    `json:"from"`
			To       string    `json:"to"`
			Subject  string    `json:"subject"`
			Body     string    `json:"body"` // Decrypted for client
			Sent     time.Time `json:"sent"`
			Read     bool      `json:"read"`
		}
		
		var response []MessageResponse
		for _, msg := range inbox {
			response = append(response, MessageResponse{
				ID:      msg.ID,
				From:    msg.From,
				To:      msg.To,
				Subject: msg.Subject,
				Body:    GetDecryptedBody(msg),
				Sent:    msg.Sent,
				Read:    msg.Read,
			})
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// show inbox
	inbox := GetInbox(username)
	
	var content string
	content += `<div id="mail">`
	content += `<h1>Inbox</h1>`
	content += `<a href="/mail?compose=true" class="mail-compose-btn">✏️ Compose</a>`
	
	// Add search box for client-side filtering
	content += `<input type="text" id="mailSearch" class="mail-search" placeholder="Search mail" oninput="filterMail()">`
	
	if len(inbox) == 0 {
		content += `<div class="mail-empty">No messages in your inbox.</div>`
	} else {
		content += `<div class="mail-list" id="mailList">`
		for _, msg := range inbox {
			readClass := ""
			if !msg.Read {
				readClass = "unread"
			}
			
			// Decrypt the message body
			decryptedBody := GetDecryptedBody(msg)
			
			// HTML escape all user-provided content to prevent XSS
			fromEscaped := stdhtml.EscapeString(msg.From)
			subjectEscaped := stdhtml.EscapeString(msg.Subject)
			bodyEscaped := stdhtml.EscapeString(decryptedBody)
			
			// Truncate body for preview
			preview := bodyEscaped
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			
			// URL encode the original body for reply
			replyURL := fmt.Sprintf("/mail?compose=true&reply_to=%s&subject=%s&original=%s", 
				fromEscaped, 
				stdhtml.EscapeString("Re: "+msg.Subject),
				stdhtml.EscapeString(decryptedBody))
			
			content += fmt.Sprintf(`
<div class="mail-item %s" data-from="%s" data-subject="%s" data-body="%s">
  <div class="mail-item-content" onclick="window.location='%s'">
    <div class="mail-item-header">
      <span class="mail-item-from">%s</span>
      <span class="mail-item-time">%s</span>
    </div>
    <div class="mail-item-subject">%s</div>
    <div class="mail-item-preview">%s</div>
  </div>
  <form action="/mail" method="POST" style="display: inline;">
    <input type="hidden" name="action" value="read">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="mail-item-delete" title="Delete">🗑️</button>
  </form>
</div>
			`, readClass, fromEscaped, subjectEscaped, bodyEscaped, replyURL, fromEscaped, app.TimeAgo(msg.Sent), subjectEscaped, preview, msg.ID)
		}
		content += `</div>`
	}
	
	// Add JavaScript for client-side search
	content += `
<script>
// Store mail data in sessionStorage for client-side search
function storeMail() {
  fetch('/mail?format=json')
    .then(response => response.json())
    .then(data => {
      sessionStorage.setItem('mailData', JSON.stringify(data));
    });
}

// Filter mail based on search input
function filterMail() {
  const searchTerm = document.getElementById('mailSearch').value.toLowerCase();
  const mailItems = document.querySelectorAll('.mail-item');
  
  mailItems.forEach(item => {
    const from = item.getAttribute('data-from').toLowerCase();
    const subject = item.getAttribute('data-subject').toLowerCase();
    const body = item.getAttribute('data-body').toLowerCase();
    
    if (from.includes(searchTerm) || subject.includes(searchTerm) || body.includes(searchTerm)) {
      item.style.display = '';
    } else {
      item.style.display = 'none';
    }
  });
}

// Store mail on page load
storeMail();
</script>`
	
	content += `</div>`

	html := app.RenderHTML("Mail", "Your inbox", content)
	w.Write([]byte(html))
}
