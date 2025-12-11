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

// Save messages to disk (caller must hold mutex)
func save() error {
	b, err := json.Marshal(messages)
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

	// Handle POST - send message or delete
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		// Check if this is a delete action
		if r.FormValue("_method") == "DELETE" {
			msgID := r.FormValue("id")
			if err := DeleteMessage(msgID, acc.ID); err != nil {
				http.Error(w, "Failed to delete message", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/mail", http.StatusSeeOther)
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

	// Check if viewing a specific message
	msgID := r.URL.Query().Get("id")
	if msgID != "" {
		mutex.RLock()
		var msg *Message
		for _, m := range messages {
			if m.ID == msgID && (m.ToID == acc.ID || m.FromID == acc.ID) {
				msg = m
				break
			}
		}
		mutex.RUnlock()

		if msg == nil {
			http.Error(w, "Message not found", http.StatusNotFound)
			return
		}

		// Mark as read if recipient is viewing
		if msg.ToID == acc.ID && !msg.Read {
			MarkAsRead(msgID, acc.ID)
		}

		// Display the message
		replyLink := fmt.Sprintf(`/mail?compose=true&to=%s&subject=%s`, msg.FromID, url.QueryEscape("Re: "+msg.Subject))
		
		messageView := fmt.Sprintf(`
			<div style="margin-bottom: 20px;">
				<a href="/mail"><button>← Back to Inbox</button></a>
			</div>
			<div style="border: 1px solid #eee; padding: 20px; border-radius: 5px;">
				<h2 style="margin-top: 0;">%s</h2>
				<div style="color: #666; margin-bottom: 20px;">
					<strong>From:</strong> <a href="/@%s">%s</a><br>
					<strong>Date:</strong> %s
				</div>
				<hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
				<div style="white-space: pre-wrap; margin: 20px 0;">%s</div>
				<hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
				<div style="display: flex; gap: 10px;">
					<a href="%s"><button>Reply</button></a>
					<form method="POST" action="/mail" style="display: inline;">
						<input type="hidden" name="_method" value="DELETE">
						<input type="hidden" name="id" value="%s">
						<button type="submit" onclick="return confirm('Delete this message?')" style="background-color: #dc3545; color: white; padding: 8px 16px; border: none; border-radius: 4px; cursor: pointer;">Delete</button>
					</form>
				</div>
			</div>
		`, msg.Subject, msg.FromID, msg.From, app.TimeAgo(msg.CreatedAt), msg.Body, replyLink, msg.ID)

		w.Write([]byte(app.RenderHTML(msg.Subject, "", messageView)))
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

	// Check which view to show (inbox or sent)
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "inbox"
	}

	// Get messages for this user
	mutex.RLock()
	var mailbox []*Message
	unreadCount := 0
	
	if view == "sent" {
		// Show sent messages
		for _, msg := range messages {
			if msg.FromID == acc.ID {
				mailbox = append(mailbox, msg)
			}
		}
	} else {
		// Show inbox (received messages)
		for _, msg := range messages {
			if msg.ToID == acc.ID {
				mailbox = append(mailbox, msg)
				if !msg.Read {
					unreadCount++
				}
			}
		}
	}
	mutex.RUnlock()

	// Sort by newest first
	sort.Slice(mailbox, func(i, j int) bool {
		return mailbox[i].CreatedAt.After(mailbox[j].CreatedAt)
	})

	// Render messages
	var items []string
	for _, msg := range mailbox {
		if view == "sent" {
			// Sent view - show recipient
			item := fmt.Sprintf(`<div class="message-item" style="padding: 15px; border-bottom: 1px solid #eee;">
				<div style="margin-bottom: 5px;">
					<strong><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></strong>
				</div>
				<div style="color: #666; font-size: 14px; margin-bottom: 5px;">To: <a href="/@%s" style="color: #666;">%s</a></div>
				<div style="color: #999; font-size: 12px;">%s</div>
			</div>`, msg.ID, msg.Subject, msg.ToID, msg.To, app.TimeAgo(msg.CreatedAt))
			items = append(items, item)
		} else {
			// Inbox view - show sender with unread indicator
			unreadIndicator := ""
			if !msg.Read {
				unreadIndicator = `<span style="color: #0066cc; font-weight: bold;">●</span> `
			}

			replyLink := fmt.Sprintf(`/mail?compose=true&to=%s&subject=%s`,
				msg.FromID, url.QueryEscape(fmt.Sprintf("Re: %s", msg.Subject)))

			item := fmt.Sprintf(`<div class="message-item" style="padding: 15px; border-bottom: 1px solid #eee;">
				<div style="margin-bottom: 5px;">
					%s<strong><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></strong>
				</div>
				<div style="color: #666; font-size: 14px; margin-bottom: 5px;">From: <a href="/@%s" style="color: #666;">%s</a> · <a href="%s" style="color: #666;">Reply</a></div>
				<div style="color: #999; font-size: 12px;">%s</div>
			</div>`, unreadIndicator, msg.ID, msg.Subject, msg.FromID, msg.From, replyLink, app.TimeAgo(msg.CreatedAt))

			items = append(items, item)
		}
	}

	content := ""
	if len(items) == 0 {
		if view == "sent" {
			content = `<p style="color: #666; padding: 20px;">No sent messages yet.</p>`
		} else {
			content = `<p style="color: #666; padding: 20px;">No messages yet.</p>`
		}
	} else {
		content = strings.Join(items, "")
	}

	title := "Mail"
	if view == "sent" {
		title = "Sent Mail"
	} else if unreadCount > 0 {
		title = fmt.Sprintf("Mail (%d new)", unreadCount)
	}

	// Tab navigation
	inboxStyle := "padding: 10px 20px; text-decoration: none; color: #333; border-bottom: 2px solid #333;"
	sentStyle := "padding: 10px 20px; text-decoration: none; color: #666; border-bottom: 2px solid transparent;"
	if view == "sent" {
		inboxStyle = "padding: 10px 20px; text-decoration: none; color: #666; border-bottom: 2px solid transparent;"
		sentStyle = "padding: 10px 20px; text-decoration: none; color: #333; border-bottom: 2px solid #333;"
	}

	html := fmt.Sprintf(`
		<div style="margin-bottom: 20px;">
			<a href="/mail?compose=true"><button>Compose</button></a>
		</div>
		<div style="border-bottom: 1px solid #eee; margin-bottom: 20px;">
			<a href="/mail" style="%s">Inbox%s</a>
			<a href="/mail?view=sent" style="%s">Sent</a>
		</div>
		<div id="mailbox">%s</div>
	`, inboxStyle, func() string {
		if unreadCount > 0 {
			return fmt.Sprintf(" (%d)", unreadCount)
		}
		return ""
	}(), sentStyle, content)

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
	err := save()
	mutex.Unlock()

	return err
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

// DeleteMessage removes a message
func DeleteMessage(msgID, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for i, msg := range messages {
		// Allow deletion if user is sender or recipient
		if msg.ID == msgID && (msg.FromID == userID || msg.ToID == userID) {
			messages = append(messages[:i], messages[i+1:]...)
			return save()
		}
	}
	return fmt.Errorf("message not found")
}
