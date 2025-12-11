package mail

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"mu/app"
	"mu/auth"
	"mu/data"
)

var mutex sync.RWMutex

// stored messages
var messages []*Message

type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`       // Sender username
	FromID    string    `json:"from_id"`    // Sender account ID
	To        string    `json:"to"`         // Recipient username
	ToID      string    `json:"to_id"`      // Recipient account ID
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Read      bool      `json:"read"`
	ReplyTo   string    `json:"reply_to"`   // ID of message this is replying to
	CreatedAt time.Time `json:"created_at"`
}

// Load messages from disk
func Load() {
	b, err := data.LoadFile("mail.json")
	if err != nil {
		messages = []*Message{}
		return
	}

	if err := json.Unmarshal(b, &messages); err != nil {
		messages = []*Message{}
		return
	}

	app.Log("mail", "Loaded %d messages", len(messages))
}

// Save messages to disk
func save() error {
	mutex.RLock()
	b, err := json.Marshal(messages)
	mutex.RUnlock()

	if err != nil {
		return err
	}

	return data.SaveFile("mail.json", string(b))
}

// Handler for /mail (inbox)
func Handler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", http.StatusUnauthorized)
		return
	}

	// Handle POST - send message
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		to := strings.TrimSpace(r.FormValue("to"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))

		if to == "" || subject == "" || body == "" {
			http.Error(w, "All fields are required", http.StatusBadRequest)
			return
		}

		// Get recipient account
		toAcc, err := auth.GetAccount(to)
		if err != nil {
			http.Error(w, "Recipient not found", http.StatusNotFound)
			return
		}

		// Send the message
		if err := SendMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, subject, body, ""); err != nil {
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
			return
		}

		// Redirect to inbox
		http.Redirect(w, r, "/mail", http.StatusSeeOther)
		return
	}

	// Check if compose mode
	if r.URL.Query().Get("compose") == "true" {
		to := r.URL.Query().Get("to")
		subject := r.URL.Query().Get("subject")
		
		composeForm := fmt.Sprintf(`
			<div style="margin-bottom: 20px;">
				<a href="/mail"><button>← Back to Inbox</button></a>
			</div>
			<form method="POST" action="/mail" style="display: flex; flex-direction: column; gap: 15px; max-width: 600px;">
				<div>
					<label for="to" style="display: block; margin-bottom: 5px; font-weight: bold;">To:</label>
					<input type="text" id="to" name="to" value="%s" required style="width: 100%%; padding: 8px; border: 1px solid #ccc; border-radius: 4px;">
				</div>
				<div>
					<label for="subject" style="display: block; margin-bottom: 5px; font-weight: bold;">Subject:</label>
					<input type="text" id="subject" name="subject" value="%s" required style="width: 100%%; padding: 8px; border: 1px solid #ccc; border-radius: 4px;">
				</div>
				<div>
					<label for="body" style="display: block; margin-bottom: 5px; font-weight: bold;">Message:</label>
					<textarea id="body" name="body" rows="10" required style="width: 100%%; padding: 8px; border: 1px solid #ccc; border-radius: 4px; resize: vertical;"></textarea>
				</div>
				<div>
					<button type="submit" style="padding: 10px 20px; background-color: #333; color: white; border: none; border-radius: 4px; cursor: pointer;">Send Message</button>
				</div>
			</form>
		`, to, subject)
		
		w.Write([]byte(app.RenderHTML("Compose Message", "", composeForm)))
		return
	}

	// Get messages for this user
	mutex.RLock()
	var inbox []*Message
	unreadCount := 0
	for _, msg := range messages {
		if msg.ToID == acc.ID {
			inbox = append(inbox, msg)
			if !msg.Read {
				unreadCount++
			}
		}
	}
	mutex.RUnlock()

	// Sort by newest first
	sort.Slice(inbox, func(i, j int) bool {
		return inbox[i].CreatedAt.After(inbox[j].CreatedAt)
	})

	// Render inbox
	var items []string
	for _, msg := range inbox {
		unreadIndicator := ""
		if !msg.Read {
			unreadIndicator = `<span style="color: #0066cc; font-weight: bold;">●</span> `
		}

		replyLink := fmt.Sprintf(`/mail?compose=true&to=%s&reply_to=%s&subject=%s`,
			msg.FromID, msg.ID, url.QueryEscape(fmt.Sprintf("Re: %s", msg.Subject)))

		item := fmt.Sprintf(`<div class="message-item" style="padding: 15px; border-bottom: 1px solid #eee;">
			<div style="margin-bottom: 5px;">
				%s<strong><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></strong>
			</div>
			<div style="color: #666; font-size: 14px; margin-bottom: 5px;">From: <a href="/@%s" style="color: #666;">%s</a> · <a href="%s" style="color: #666;">Reply</a></div>
			<div style="color: #999; font-size: 12px;">%s</div>
		</div>`, unreadIndicator, msg.ID, msg.Subject, msg.FromID, msg.From, replyLink, app.TimeAgo(msg.CreatedAt))

		items = append(items, item)
	}

	content := ""
	if len(items) == 0 {
		content = `<p style="color: #666; padding: 20px;">No messages yet.</p>`
	} else {
		content = strings.Join(items, "")
	}

	title := "Mail"
	if unreadCount > 0 {
		title = fmt.Sprintf("Mail (%d new)", unreadCount)
	}

	html := fmt.Sprintf(`
		<div style="margin-bottom: 20px;">
			<a href="/mail?compose=true"><button>Compose</button></a>
		</div>
		<div id="inbox">%s</div>
	`, content)

	w.Write([]byte(app.RenderHTML(title, "Your messages", html)))
}

// SendMessage creates and saves a new message
func SendMessage(from, fromID, to, toID, subject, body, replyTo string) error {
	msg := &Message{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		From:      from,
		FromID:    fromID,
		To:        to,
		ToID:      toID,
		Subject:   subject,
		Body:      body,
		Read:      false,
		ReplyTo:   replyTo,
		CreatedAt: time.Now(),
	}

	mutex.Lock()
	messages = append([]*Message{msg}, messages...)
	mutex.Unlock()

	return save()
}

// GetUnreadCount returns the number of unread messages for a user
func GetUnreadCount(userID string) int {
	mutex.RLock()
	defer mutex.RUnlock()

	count := 0
	for _, msg := range messages {
		if msg.ToID == userID && !msg.Read {
			count++
		}
	}
	return count
}

// MarkAsRead marks a message as read
func MarkAsRead(msgID, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for _, msg := range messages {
		if msg.ID == msgID && msg.ToID == userID {
			msg.Read = true
			return save()
		}
	}
	return fmt.Errorf("message not found")
}
