package mail

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
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

// Blocklist for blocking abusive senders
type Blocklist struct {
	Emails []string `json:"emails"` // Blocked email addresses
	IPs    []string `json:"ips"`    // Blocked IP addresses
}

var (
	blocklistMutex sync.RWMutex
	blocklist      = &Blocklist{
		Emails: []string{},
		IPs:    []string{},
	}
)

type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`    // Sender username
	FromID    string    `json:"from_id"` // Sender account ID
	To        string    `json:"to"`      // Recipient username
	ToID      string    `json:"to_id"`   // Recipient account ID
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	Read      bool      `json:"read"`
	ReplyTo   string    `json:"reply_to"` // ID of message this is replying to
	MessageID string    `json:"message_id"` // Email Message-ID header for threading
	CreatedAt time.Time `json:"created_at"`
}

// Load messages from disk
// Load messages from disk and configure SMTP/DKIM
func Load() {
	b, err := data.LoadFile("mail.json")
	if err != nil {
		messages = []*Message{}
	} else if err := json.Unmarshal(b, &messages); err != nil {
		messages = []*Message{}
	} else {
		app.Log("mail", "Loaded %d messages", len(messages))
	}

	// Load blocklist
	loadBlocklist()

	// Configure SMTP client from environment variables
	ConfigureSMTP()

	// Try to load DKIM config if keys exist (optional)
	dkimDomain := os.Getenv("DKIM_DOMAIN")
	if dkimDomain == "" {
		dkimDomain = "localhost"
	}
	dkimSelector := os.Getenv("DKIM_SELECTOR")
	if dkimSelector == "" {
		dkimSelector = "default"
	}

	if err := LoadDKIMConfig(dkimDomain, dkimSelector); err != nil {
		app.Log("mail", "DKIM signing disabled: %v", err)
	}
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

		// Check if this is a block sender action (admin only)
		if r.FormValue("action") == "block_sender" {
			senderEmail := r.FormValue("sender_email")
			if senderEmail != "" && acc.Admin {
				if err := BlockEmail(senderEmail); err != nil {
					app.Log("mail", "Failed to block sender %s: %v", senderEmail, err)
				}
			}
			// Redirect back to mail view
			msgID := r.FormValue("msg_id")
			if msgID != "" {
				http.Redirect(w, r, "/mail?id="+msgID, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/mail", http.StatusSeeOther)
			}
			return
		}

		to := strings.TrimSpace(r.FormValue("to"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		body := strings.TrimSpace(r.FormValue("body"))
		replyTo := strings.TrimSpace(r.FormValue("reply_to"))

		if to == "" || subject == "" || body == "" {
			http.Error(w, "All fields are required", http.StatusBadRequest)
			return
		}

		// Check if recipient is external (has @domain)
		if IsExternalEmail(to) {
			// Send external email via SMTP
			fromEmail := GetEmailForUser(acc.Name, GetConfiguredDomain())
			messageID, err := SendExternalEmail(acc.Name, fromEmail, to, subject, body, replyTo)
			if err != nil {
				http.Error(w, "Failed to send email: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Store a copy in sent messages for the sender
			if err := SendMessage(acc.Name, acc.ID, to, to, subject, body, replyTo, messageID); err != nil {
				app.Log("mail", "Warning: Failed to store sent message: %v", err)
			}
		} else {
			// Internal recipient - look up user account
			toAcc, err := auth.GetAccount(to)
			if err != nil {
				http.Error(w, "Recipient not found", http.StatusNotFound)
				return
			}

			// Send the internal message
			app.Log("mail", "Sending internal message from %s to %s with replyTo=%s", acc.Name, toAcc.Name, replyTo)
			if err := SendMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, subject, body, replyTo, ""); err != nil {
				http.Error(w, "Failed to send message", http.StatusInternalServerError)
				return
			}
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

		// Decode body if it looks base64 encoded
		displayBody := msg.Body
		if looksLikeBase64(displayBody) {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(displayBody)); err == nil {
				if isValidUTF8Text(decoded) {
					displayBody = string(decoded)
					app.Log("mail", "Decoded base64 body for display")
				}
			}
		}
		
		// Convert URLs to clickable links
		displayBody = linkifyURLs(displayBody)

		// Display the message
		replySubject := msg.Subject
		if !strings.HasPrefix(strings.ToLower(msg.Subject), "re:") {
			replySubject = "Re: " + msg.Subject
		}
		replyLink := fmt.Sprintf(`/mail?compose=true&to=%s&subject=%s&reply_to=%s`, msg.FromID, url.QueryEscape(replySubject), msgID)

		// Add block button if sender is external email and user is admin
		blockButton := ""
		if acc.Admin && IsExternalEmail(msg.FromID) {
			blockButton = fmt.Sprintf(`
				<form method="POST" action="/mail" style="display: inline;">
					<input type="hidden" name="action" value="block_sender">
					<input type="hidden" name="sender_email" value="%s">
					<input type="hidden" name="msg_id" value="%s">
					<button type="submit" onclick="return confirm('Block %s from sending mail?')" style="background-color: #6c757d; color: white; padding: 8px 16px; border: none; border-radius: 4px; cursor: pointer;">Block Sender</button>
				</form>`, msg.FromID, msg.ID, msg.FromID)
		}

		// Format From field - only link to user profile if it's an internal user
		fromDisplayFull := msg.From
		if IsExternalEmail(msg.FromID) {
			// External email - show name and email address
			if msg.From != msg.FromID {
				fromDisplayFull = fmt.Sprintf(`%s &lt;%s&gt;`, msg.From, msg.FromID)
			} else {
				fromDisplayFull = msg.FromID
			}
		} else {
		fromDisplayFull = fmt.Sprintf(`<a href="/@%s">%s</a>`, msg.FromID, msg.FromID)
	}

	messageView := fmt.Sprintf(`
		<div style="margin-bottom: 20px;">
			<a href="/mail"><button>← Back to Inbox</button></a>
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
					%s
				</div>
			</div>
		`, msg.Subject, fromDisplayFull, app.TimeAgo(msg.CreatedAt), displayBody, replyLink, msg.ID, blockButton)

		w.Write([]byte(app.RenderHTML(msg.Subject, "", messageView)))
		return
	}

	// Check if compose mode
	if r.URL.Query().Get("compose") == "true" {
		to := r.URL.Query().Get("to")
		subject := r.URL.Query().Get("subject")
		replyTo := r.URL.Query().Get("reply_to")

		// Determine back link and page title
		backLink := "/mail"
		pageTitle := "Compose Message"
		if replyTo != "" {
			backLink = "/mail?id=" + replyTo
			pageTitle = subject
		}

		composeForm := fmt.Sprintf(`
			<div style="margin-bottom: 20px;">
				<a href="%s"><button>← Back</button></a>
			</div>
			<form method="POST" action="/mail" style="display: flex; flex-direction: column; gap: 15px; max-width: 600px;">
				<input type="hidden" name="reply_to" value="%s">
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
		`, backLink, replyTo, to, subject)

		w.Write([]byte(app.RenderHTML(pageTitle, "", composeForm)))
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

	// Group messages into threads for inbox view
	var items []string
	if view == "inbox" {
		app.Log("mail", "Rendering inbox with %d messages for user %s", len(mailbox), acc.Name)
		
		// Build a complete thread context including sent messages
		// This allows us to thread replies to messages we sent
		mutex.RLock()
		threadContext := make(map[string]*Message)
		for _, msg := range messages {
			if msg.FromID == acc.ID || msg.ToID == acc.ID {
				threadContext[msg.ID] = msg
			}
		}
		mutex.RUnlock()
		
		// Create a map of inbox messages for quick lookup
		msgMap := make(map[string]*Message)
		for _, msg := range mailbox {
			msgMap[msg.ID] = msg
			if msg.ReplyTo != "" {
				app.Log("mail", "Inbox message %s (from %s, subject: %s) has replyTo=%s", msg.ID, msg.From, msg.Subject, msg.ReplyTo)
				// Check if parent exists
				if parent, exists := threadContext[msg.ReplyTo]; exists {
					app.Log("mail", "  -> Parent found: %s (from %s, in sent: %v)", parent.ID, parent.From, parent.FromID == acc.ID)
				} else {
					app.Log("mail", "  -> Parent NOT found in thread context")
				}
			}
		}

		// Track which messages have been rendered (to avoid duplicates)
		rendered := make(map[string]bool)

		// Build thread groups: find root messages and their replies
		type thread struct {
			root    *Message
			replies []*Message
		}
		threads := []thread{}
		
		// First pass: identify which inbox messages are replies and which are roots
		for _, msg := range mailbox {
			if msg.ReplyTo != "" {
				// This is a reply - check if parent is in inbox
				if _, inInbox := msgMap[msg.ReplyTo]; inInbox {
					// Parent is in inbox, will be handled when we process the parent
					continue
				}
				// Parent is not in inbox (probably a sent message)
				// Treat this as a root and try to build thread from parent
				if parent, exists := threadContext[msg.ReplyTo]; exists {
					// We have the parent from thread context (sent message)
					// Create a thread starting from the parent
					if !rendered[parent.ID] {
						t := thread{root: parent}
						// Find all inbox messages that are replies to this parent
						for _, candidate := range mailbox {
							if candidate.ReplyTo == parent.ID {
								t.replies = append(t.replies, candidate)
							}
						}
						sort.Slice(t.replies, func(i, j int) bool {
							return t.replies[i].CreatedAt.Before(t.replies[j].CreatedAt)
						})
						threads = append(threads, t)
						rendered[parent.ID] = true
						for _, r := range t.replies {
							rendered[r.ID] = true
						}
					}
				} else {
					// Parent doesn't exist, treat as orphan
					if !rendered[msg.ID] {
						threads = append(threads, thread{root: msg})
						rendered[msg.ID] = true
					}
				}
			} else {
				// This is a root message (no replyTo)
				if !rendered[msg.ID] {
					t := thread{root: msg}
					// Find all replies in inbox
					for _, candidate := range mailbox {
						if candidate.ReplyTo == msg.ID {
							t.replies = append(t.replies, candidate)
						}
					}
					sort.Slice(t.replies, func(i, j int) bool {
						return t.replies[i].CreatedAt.Before(t.replies[j].CreatedAt)
					})
					threads = append(threads, t)
					rendered[msg.ID] = true
					for _, r := range t.replies {
							rendered[r.ID] = true
					}
				}
			}
		}
		
		// Render all threads
		for _, t := range threads {
			// Render root (could be from inbox or sent)
			if t.root.ToID == acc.ID {
				// Root is in our inbox
				items = append(items, renderInboxMessage(t.root, 0, acc.ID))
			} else if t.root.FromID == acc.ID {
				// Root is from our sent messages - show as "You sent"
				items = append(items, renderSentMessageInThread(t.root))
			}
			// Render replies
			for _, reply := range t.replies {
				items = append(items, renderInboxMessage(reply, 1, acc.ID))
			}
		}
		
		// Render any unrendered messages as orphans
		for _, msg := range mailbox {
			if !rendered[msg.ID] {
				app.Log("mail", "Rendering orphaned inbox message: %s (replyTo=%s)", msg.ID, msg.ReplyTo)
				items = append(items, renderInboxMessage(msg, 0, acc.ID))
			}
		}
	} else {
		// Sent view - simple list
		for _, msg := range mailbox {
			items = append(items, renderSentMessage(msg))
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

// renderInboxMessage renders a single inbox message with optional indentation
func renderInboxMessage(msg *Message, indent int, viewerID string) string {
	unreadIndicator := ""
	if !msg.Read {
		unreadIndicator = `<span style="color: #999; font-size: 12px;">●</span> `
	}

	replyLink := fmt.Sprintf(`/mail?compose=true&to=%s&subject=%s&reply_to=%s`,
		msg.FromID, url.QueryEscape(fmt.Sprintf("Re: %s", msg.Subject)), msg.ID)

	// Add indentation for replies
	marginLeft := ""
	replyIndicator := ""
	if indent > 0 {
		marginLeft = fmt.Sprintf(" margin-left: %dpx;", indent*30)
		replyIndicator = `<span style="color: #999; margin-right: 5px;">↳</span>`
	}

	// Format From field - only link to user profile if it's an internal user (no @)
	fromDisplay := msg.From
	if IsExternalEmail(msg.FromID) {
		// External email - show name and email address
		if msg.From != msg.FromID {
			fromDisplay = fmt.Sprintf(`<span style="color: #666;">%s &lt;%s&gt;</span>`, msg.From, msg.FromID)
		} else {
			fromDisplay = fmt.Sprintf(`<span style="color: #666;">%s</span>`, msg.FromID)
		}
	} else {
		// Internal user - link to profile
		fromDisplay = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, msg.FromID, msg.FromID)
	}

	return fmt.Sprintf(`<div class="message-item" style="padding: 15px; border-bottom: 1px solid #eee;%s">
		<div style="margin-bottom: 5px;">
			%s%s<strong><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></strong>
		</div>
		<div style="color: #666; font-size: 14px; margin-bottom: 5px;">From: %s · <a href="%s" style="color: #666;">Reply</a></div>
		<div style="color: #999; font-size: 12px;">%s</div>
	</div>`, marginLeft, replyIndicator, unreadIndicator, msg.ID, msg.Subject, fromDisplay, replyLink, app.TimeAgo(msg.CreatedAt))
}

// renderSentMessage renders a single sent message
func renderSentMessage(msg *Message) string {
	// Format To field - only link to user profile if it's an internal user
	toDisplay := msg.To
	if IsExternalEmail(msg.ToID) {
		// External email - show name and email address
		if msg.To != msg.ToID {
			toDisplay = fmt.Sprintf(`<span style="color: #666;">%s &lt;%s&gt;</span>`, msg.To, msg.ToID)
		} else {
			toDisplay = fmt.Sprintf(`<span style="color: #666;">%s</span>`, msg.ToID)
		}
	} else {
		toDisplay = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, msg.ToID, msg.To)
	}
	
	return fmt.Sprintf(`<div class="message-item" style="padding: 15px; border-bottom: 1px solid #eee;">
		<div style="margin-bottom: 5px;">
			<strong><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></strong>
		</div>
		<div style="color: #666; font-size: 14px; margin-bottom: 5px;">To: %s</div>
		<div style="color: #999; font-size: 12px;">%s</div>
	</div>`, msg.ID, msg.Subject, toDisplay, app.TimeAgo(msg.CreatedAt))
}

// renderSentMessageInThread renders a sent message as part of a thread in inbox view
func renderSentMessageInThread(msg *Message) string {
	// Format To field - only link to user profile if it's an internal user
	toDisplay := msg.To
	if IsExternalEmail(msg.ToID) {
		// External email - show name and email address
		if msg.To != msg.ToID {
			toDisplay = fmt.Sprintf(`<span style="color: #666;">%s &lt;%s&gt;</span>`, msg.To, msg.ToID)
		} else {
			toDisplay = fmt.Sprintf(`<span style="color: #666;">%s</span>`, msg.ToID)
		}
	} else {
		toDisplay = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, msg.ToID, msg.To)
	}
	
	return fmt.Sprintf(`<div class="message-item" style="padding: 15px; border-bottom: 1px solid #eee; background-color: #f9f9f9;">
		<div style="margin-bottom: 5px;">
			<strong><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></strong>
		</div>
		<div style="color: #666; font-size: 14px; margin-bottom: 5px;">You sent to: %s</div>
		<div style="color: #999; font-size: 12px;">%s</div>
	</div>`, msg.ID, msg.Subject, toDisplay, app.TimeAgo(msg.CreatedAt))
}

// SendMessage creates and saves a new message
func SendMessage(from, fromID, to, toID, subject, body, replyTo, messageID string) error {
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
		MessageID: messageID,
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

// FindMessageByMessageID finds a message by its email Message-ID header
func FindMessageByMessageID(messageID string) *Message {
	mutex.RLock()
	defer mutex.RUnlock()

	if messageID == "" {
		return nil
	}

	for _, msg := range messages {
		if msg.MessageID == messageID {
			return msg
		}
	}
	return nil
}

// GetMessage finds a message by its internal ID
func GetMessage(msgID string) *Message {
	mutex.RLock()
	defer mutex.RUnlock()

	for _, msg := range messages {
		if msg.ID == msgID {
			return msg
		}
	}
	return nil
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

// UnreadCountHandler returns unread message count as JSON
func UnreadCountHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"count": 0})
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"count": 0})
		return
	}

	count := GetUnreadCount(acc.ID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"count": count})
}

// ============================================
// BLOCKLIST MANAGEMENT
// ============================================

// loadBlocklist loads the blocklist from disk
func loadBlocklist() {
	b, err := data.LoadFile("blocklist.json")
	if err != nil {
		app.Log("mail", "No blocklist file found, starting with empty blocklist")
		return
	}

	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	if err := json.Unmarshal(b, blocklist); err != nil {
		app.Log("mail", "Error loading blocklist: %v", err)
		return
	}

	app.Log("mail", "Loaded blocklist: %d emails, %d IPs", len(blocklist.Emails), len(blocklist.IPs))
}

// saveBlocklist saves the blocklist to disk
func saveBlocklist() error {
	blocklistMutex.RLock()
	defer blocklistMutex.RUnlock()

	b, err := json.MarshalIndent(blocklist, "", "  ")
	if err != nil {
		return err
	}

	return data.SaveFile("blocklist.json", string(b))
}

// IsBlocked checks if an email or IP is blocked
func IsBlocked(email, ip string) bool {
	blocklistMutex.RLock()
	defer blocklistMutex.RUnlock()

	email = strings.ToLower(strings.TrimSpace(email))
	ip = strings.TrimSpace(ip)

	// Check email
	for _, blocked := range blocklist.Emails {
		if strings.ToLower(blocked) == email {
			return true
		}
		// Support wildcard domain blocking (e.g., "*@spam.com")
		if strings.HasPrefix(blocked, "*@") {
			domain := strings.TrimPrefix(blocked, "*@")
			if strings.HasSuffix(email, "@"+domain) {
				return true
			}
		}
	}

	// Check IP
	for _, blocked := range blocklist.IPs {
		if blocked == ip {
			return true
		}
	}

	return false
}

// BlockEmail adds an email to the blocklist
func BlockEmail(email string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	// Check if already blocked
	for _, blocked := range blocklist.Emails {
		if strings.ToLower(blocked) == email {
			return fmt.Errorf("email already blocked")
		}
	}

	blocklist.Emails = append(blocklist.Emails, email)
	app.Log("mail", "Blocked email: %s", email)

	return saveBlocklist()
}

// BlockIP adds an IP to the blocklist
func BlockIP(ip string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("IP cannot be empty")
	}

	// Check if already blocked
	for _, blocked := range blocklist.IPs {
		if blocked == ip {
			return fmt.Errorf("IP already blocked")
		}
	}

	blocklist.IPs = append(blocklist.IPs, ip)
	app.Log("mail", "Blocked IP: %s", ip)

	return saveBlocklist()
}

// UnblockEmail removes an email from the blocklist
func UnblockEmail(email string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	email = strings.ToLower(strings.TrimSpace(email))

	for i, blocked := range blocklist.Emails {
		if strings.ToLower(blocked) == email {
			blocklist.Emails = append(blocklist.Emails[:i], blocklist.Emails[i+1:]...)
			app.Log("mail", "Unblocked email: %s", email)
			return saveBlocklist()
		}
	}

	return fmt.Errorf("email not found in blocklist")
}

// UnblockIP removes an IP from the blocklist
func UnblockIP(ip string) error {
	blocklistMutex.Lock()
	defer blocklistMutex.Unlock()

	ip = strings.TrimSpace(ip)

	for i, blocked := range blocklist.IPs {
		if blocked == ip {
			blocklist.IPs = append(blocklist.IPs[:i], blocklist.IPs[i+1:]...)
			app.Log("mail", "Unblocked IP: %s", ip)
			return saveBlocklist()
		}
	}

	return fmt.Errorf("IP not found in blocklist")
}

// GetBlocklist returns a copy of the current blocklist
func GetBlocklist() *Blocklist {
	blocklistMutex.RLock()
	defer blocklistMutex.RUnlock()

	return &Blocklist{
		Emails: append([]string{}, blocklist.Emails...),
		IPs:    append([]string{}, blocklist.IPs...),
	}
}

// looksLikeBase64 checks if a string appears to be base64 encoded
func looksLikeBase64(s string) bool {
	s = strings.TrimSpace(s)
	
	// Must be reasonable length (not empty, not too short)
	if len(s) < 20 {
		return false
	}
	
	// Base64 strings should be mostly base64 characters (a-zA-Z0-9+/=)
	validChars := 0
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || 
		   (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' || c == '\n' || c == '\r' {
			validChars++
		}
	}
	
	// If more than 90% of characters are valid base64 chars, likely base64
	return float64(validChars)/float64(len(s)) > 0.9
}

// isValidUTF8Text checks if decoded bytes are valid UTF-8 text
func isValidUTF8Text(data []byte) bool {
	// Check if it's valid UTF-8
	if !strings.HasPrefix(string(data), "\xff\xfe") && !strings.HasPrefix(string(data), "\xfe\xff") {
		text := string(data)
		// Should contain mostly printable characters
		printable := 0
		for _, r := range text {
			if r >= 32 || r == '\t' || r == '\n' || r == '\r' {
				printable++
			}
		}
		// If more than 90% printable, consider it valid text
		if len(text) > 0 && float64(printable)/float64(len(text)) > 0.9 {
			return true
		}
	}
	return false
}

// linkifyURLs converts URLs in text to clickable HTML links
func linkifyURLs(text string) string {
	result := ""
	lastIndex := 0
	
	for i := 0; i < len(text); i++ {
		// Check for http:// or https://
		if strings.HasPrefix(text[i:], "http://") || strings.HasPrefix(text[i:], "https://") || strings.HasPrefix(text[i:], "www.") {
			// Add text before the URL
			result += text[lastIndex:i]
			
			// Find the end of the URL
			end := i
			for end < len(text) && !isURLTerminator(text[end]) {
				end++
			}
			
			url := text[i:end]
			// Add http:// prefix for www. URLs
			href := url
			if strings.HasPrefix(url, "www.") {
				href = "http://" + url
			}
			
			// Create clickable link
			result += fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" style="color: #0066cc; text-decoration: underline;">%s</a>`, href, url)
			
			lastIndex = end
			i = end - 1 // -1 because loop will increment
		}
	}
	
	// Add remaining text
	result += text[lastIndex:]
	return result
}

// isURLTerminator checks if a character ends a URL
func isURLTerminator(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '<' || c == '>' || 
	       c == '"' || c == '\'' || c == ')' || c == ']' || c == '}' || c == ',' || c == ';'
}
