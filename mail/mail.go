package mail

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
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

// Inbox organizes messages by thread for a user
type Inbox struct {
	Threads map[string]*Thread // threadID -> Thread
}

// Thread represents a conversation thread
type Thread struct {
	Root      *Message
	Messages  []*Message
	Latest    *Message
	HasUnread bool
}

// inboxes maps userID to their organized inbox
var inboxes map[string]*Inbox

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
	ReplyTo   string    `json:"reply_to"`   // ID of message this is replying to
	ThreadID  string    `json:"thread_id"`  // Root message ID for O(1) thread grouping
	MessageID string    `json:"message_id"` // Email Message-ID header for threading
	CreatedAt time.Time `json:"created_at"`
}

// Load messages from disk
// Load messages from disk and configure SMTP/DKIM
func Load() {
	b, err := data.LoadFile("mail.json")
	if err != nil {
		messages = []*Message{}
		inboxes = make(map[string]*Inbox)
	} else if err := json.Unmarshal(b, &messages); err != nil {
		messages = []*Message{}
		inboxes = make(map[string]*Inbox)
	} else {
		app.Log("mail", "Loaded %d messages", len(messages))
		
		// Fix threading for any messages with broken chains
		fixThreading()
		
		// Build inbox structures organized by thread
		rebuildInboxes()
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

// fixThreading repairs broken threading relationships and computes ThreadID after loading
func fixThreading() {
	fixed := 0
	
	// First pass: fix orphaned messages
	for _, msg := range messages {
		if msg.ReplyTo == "" {
			continue
		}
		
		// Check if the parent exists
		if GetMessageUnlocked(msg.ReplyTo) == nil {
			// Parent doesn't exist - mark as orphaned
			app.Log("mail", "Message %s has missing parent %s - marking as root", msg.ID, msg.ReplyTo)
			msg.ReplyTo = ""
			fixed++
		}
	}
	
	// Second pass: compute ThreadID for all messages
	for _, msg := range messages {
		threadID := computeThreadID(msg)
		if msg.ThreadID != threadID {
			msg.ThreadID = threadID
			fixed++
		}
	}
	
	if fixed > 0 {
		app.Log("mail", "Fixed threading for %d messages", fixed)
		save()
	}
}

// computeThreadID walks up the chain to find the root message ID
func computeThreadID(msg *Message) string {
	if msg.ReplyTo == "" {
		return msg.ID // This is the root
	}
	
	// Walk up the chain
	visited := make(map[string]bool)
	current := msg
	for current.ReplyTo != "" && !visited[current.ID] {
		visited[current.ID] = true
		parent := GetMessageUnlocked(current.ReplyTo)
		if parent == nil {
			break // Parent doesn't exist, current is root
		}
		current = parent
	}
	return current.ID
}

// GetMessageUnlocked finds a message without locking (for internal use when lock is held)
func GetMessageUnlocked(msgID string) *Message {
	for _, msg := range messages {
		if msg.ID == msgID {
			return msg
		}
	}
	return nil
}

// rebuildInboxes reconstructs inbox structures from messages (must hold mutex)
func rebuildInboxes() {
	inboxes = make(map[string]*Inbox)
	
	for _, msg := range messages {
		// Add to sender's inbox (sent messages)
		if msg.FromID != "" {
			if inboxes[msg.FromID] == nil {
				inboxes[msg.FromID] = &Inbox{Threads: make(map[string]*Thread)}
			}
			addMessageToInbox(inboxes[msg.FromID], msg, msg.FromID)
		}
		
		// Add to recipient's inbox
		if msg.ToID != "" && msg.ToID != msg.FromID {
			if inboxes[msg.ToID] == nil {
				inboxes[msg.ToID] = &Inbox{Threads: make(map[string]*Thread)}
			}
			addMessageToInbox(inboxes[msg.ToID], msg, msg.ToID)
		}
	}
}

// addMessageToInbox adds a message to an inbox's thread structure
func addMessageToInbox(inbox *Inbox, msg *Message, userID string) {
	threadID := msg.ThreadID
	if threadID == "" {
		threadID = msg.ID
	}
	
	thread := inbox.Threads[threadID]
	if thread == nil {
		// New thread
		rootMsg := GetMessageUnlocked(threadID)
		if rootMsg == nil {
			rootMsg = msg
		}
		thread = &Thread{
			Root:      rootMsg,
			Messages:  []*Message{msg},
			Latest:    msg,
			HasUnread: !msg.Read && msg.ToID == userID,
		}
		inbox.Threads[threadID] = thread
	} else {
		// Add to existing thread
		thread.Messages = append(thread.Messages, msg)
		if msg.CreatedAt.After(thread.Latest.CreatedAt) {
			thread.Latest = msg
		}
		if !msg.Read && msg.ToID == userID {
			thread.HasUnread = true
		}
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
			// Redirect back to the thread if return_to is specified, otherwise inbox
			returnTo := r.FormValue("return_to")
			if returnTo != "" {
				http.Redirect(w, r, "/mail?id="+returnTo, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/mail", http.StatusSeeOther)
			}
			return
		}

		// Check if this is a delete thread action
		if r.FormValue("action") == "delete_thread" {
			msgID := r.FormValue("msg_id")
			if err := DeleteThread(msgID, acc.ID); err != nil {
				http.Error(w, "Failed to delete thread", http.StatusInternalServerError)
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
			fromEmail := GetEmailForUser(acc.ID, GetConfiguredDomain())
			// Use just the username (acc.ID) as display name, not acc.Name which might contain @
			displayName := acc.ID
			
			// Send external email (connects to localhost SMTP server which relays)
			messageID, err := SendExternalEmail(displayName, fromEmail, to, subject, body, replyTo)
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

		// Redirect back to thread if replying, otherwise to inbox
		// Check if this was a reply (has reply_to parameter or id in URL)
		threadID := r.URL.Query().Get("id")
		if threadID != "" {
			// Return to the thread view
			http.Redirect(w, r, "/mail?id="+threadID, http.StatusSeeOther)
		} else if replyTo != "" {
			// Return to the original message being replied to
			http.Redirect(w, r, "/mail?id="+replyTo, http.StatusSeeOther)
		} else {
			// New message, go to inbox
			http.Redirect(w, r, "/mail", http.StatusSeeOther)
		}
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

		// Prepare reply subject
		replySubject := msg.Subject
		if !strings.HasPrefix(strings.ToLower(msg.Subject), "re:") {
			replySubject = "Re: " + msg.Subject
		}

		// Add block link if sender is external email and user is admin
		blockButton := ""
		if acc.Admin && IsExternalEmail(msg.FromID) {
			blockButton = fmt.Sprintf(`
			<span style="margin: 0 8px;">·</span>
			<a href="#" onclick="if(confirm('Block %s from sending mail?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='block_sender';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='sender_email';input2.value='%s';form.appendChild(input2);var input3=document.createElement('input');input3.type='hidden';input3.name='msg_id';input3.value='%s';form.appendChild(input3);document.body.appendChild(form);form.submit();}return false;" style="color: #666;">Block Sender</a>
		`, msg.FromID, msg.FromID, msg.ID)
		}

	// Build thread view - find all messages in this thread
	var thread []*Message
	mutex.RLock()
	// Find root message by traversing ReplyTo chain
	rootID := msgID
	current := msg
	for current.ReplyTo != "" {
		found := false
		for _, m := range messages {
			if m.ID == current.ReplyTo && (m.FromID == acc.ID || m.ToID == acc.ID) {
				rootID = m.ID
				current = m
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	
	// Collect all messages in thread recursively
	threadIDs := make(map[string]bool)
	threadIDs[rootID] = true
	
	// Keep looking for replies until no more found
	changed := true
	for changed {
		changed = false
		for _, m := range messages {
			if (m.FromID == acc.ID || m.ToID == acc.ID) && !threadIDs[m.ID] {
				if threadIDs[m.ReplyTo] {
					threadIDs[m.ID] = true
					changed = true
				}
			}
		}
	}
	
	// Collect messages
	for _, m := range messages {
		if threadIDs[m.ID] {
			thread = append(thread, m)
		}
	}
	mutex.RUnlock()
	
	// Sort thread by time
	sort.Slice(thread, func(i, j int) bool {
		return thread[i].CreatedAt.Before(thread[j].CreatedAt)
	})

	// Mark all unread messages in this thread as read (if user is recipient)
	for _, m := range thread {
		if m.ToID == acc.ID && !m.Read {
			MarkAsRead(m.ID, acc.ID)
		}
	}

	// Render thread
	var threadHTML strings.Builder
	for _, m := range thread {
		msgBody := m.Body
		if looksLikeBase64(msgBody) {
			if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(msgBody)); err == nil {
				if isValidUTF8Text(decoded) {
					msgBody = string(decoded)
				}
			}
		}
		msgBody = linkifyURLs(msgBody)
		
		isSent := m.FromID == acc.ID
		authorDisplay := m.FromID
		if isSent {
			authorDisplay = "You"
		} else if !IsExternalEmail(m.FromID) {
			// Internal user - add profile link
			authorDisplay = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, m.FromID, m.FromID)
		} else if m.From != m.FromID {
			// External email with display name
			authorDisplay = m.From
		}
		
		threadHTML.WriteString(fmt.Sprintf(`
		<div style="padding: 20px 0; border-bottom: 1px solid #eee;">
			<div style="color: #666; font-size: small; margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
				<span>%s · %s</span>
				<a href="#" onclick="if(confirm('Delete this message?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='_method';input1.value='DELETE';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='id';input2.value='%s';form.appendChild(input2);var input3=document.createElement('input');input3.type='hidden';input3.name='return_to';input3.value='%s';form.appendChild(input3);document.body.appendChild(form);form.submit();}return false;" style="color: #999; font-size: 0.9em;">Delete</a>
			</div>
			<div style="white-space: pre-wrap; line-height: 1.6; word-wrap: break-word; overflow-wrap: break-word;">%s</div>
		</div>`, authorDisplay, app.TimeAgo(m.CreatedAt), m.ID, msgID, msgBody))
	}

	// Determine the other party in the thread
	otherParty := msg.FromID
	if msg.FromID == acc.ID {
		otherParty = msg.ToID
	}
	// Format other party with profile link if internal user
	otherPartyDisplay := otherParty
	if !IsExternalEmail(otherParty) {
		otherPartyDisplay = fmt.Sprintf(`<a href="/@%s" style="color: #666;">%s</a>`, otherParty, otherParty)
	}
	
	messageView := fmt.Sprintf(`
	<div style="color: #666; font-size: small; margin-bottom: 20px;">Thread with: %s</div>
	<hr style="margin: 20px 0; border: none; border-top: 1px solid #eee;">
	%s
	<div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #eee;">
		<form method="POST" action="/mail?id=%s" style="display: flex; flex-direction: column; gap: 15px;">
			<input type="hidden" name="to" value="%s">
			<input type="hidden" name="subject" value="%s">
			<input type="hidden" name="reply_to" value="%s">
			<textarea name="body" placeholder="Write your reply..." required style="padding: 15px; border: 1px solid #ddd; border-radius: 4px; font-family: inherit; font-size: inherit; min-height: 100px; resize: vertical;"></textarea>
			<div style="color: #666; font-size: 14px;">
				<a href="#" onclick="this.closest('form').submit(); return false;" style="color: #666;">Send</a>
				<span style="margin: 0 8px;">·</span>
				<a href="#" onclick="if(confirm('Delete this entire thread?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='delete_thread';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='msg_id';input2.value='%s';form.appendChild(input2);document.body.appendChild(form);form.submit();}return false;" style="color: #dc3545;">Delete Thread</a>
				%s
			</div>
		</form>
		<div style="margin-top: 20px;">
			<a href="/mail" style="color: #666; text-decoration: none;">← Back to mail</a>
		</div>
	</div>
`, otherPartyDisplay, threadHTML.String(), msgID, otherParty, replySubject, rootID, msg.ID, blockButton)
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
		pageTitle := "New Message"
		if replyTo != "" {
			backLink = "/mail?id=" + replyTo
			pageTitle = subject
		}

		composeForm := fmt.Sprintf(`
			<form method="POST" action="/mail" style="display: flex; flex-direction: column; gap: 10px;">
				<input type="hidden" name="reply_to" value="%s">
				<input type="text" name="to" placeholder="To: username or email" value="%s" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
				<input type="text" name="subject" placeholder="Subject" value="%s" required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px;">
				<textarea name="body" rows="10" placeholder="Write your message..." required style="padding: 10px; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical; min-height: 200px;"></textarea>
				<div style="color: #666; font-size: 14px;">
					<a href="#" onclick="this.closest('form').submit(); return false;" style="color: #666;">Send</a>
					<span style="margin: 0 8px;">·</span>
					<a href="%s" style="color: #666;">Cancel</a>
				</div>
			</form>
			<div style="margin-top: 20px;">
				<a href="%s" style="color: #666; text-decoration: none;">← Back</a>
			</div>
		`, replyTo, to, subject, backLink, backLink)

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
	// Get user's inbox - O(1) lookup
	userInbox := inboxes[acc.ID]
	mutex.RUnlock()
	
	if userInbox == nil {
		userInbox = &Inbox{Threads: make(map[string]*Thread)}
	}

	// Render threads from pre-organized inbox
	var items []string
	unreadCount := 0
	if view == "inbox" {
		app.Log("mail", "Rendering inbox with %d threads for user %s", len(userInbox.Threads), acc.Name)

		// Convert threads to slice for sorting
		threads := make([]*Thread, 0, len(userInbox.Threads))
		for _, thread := range userInbox.Threads {
			threads = append(threads, thread)
			
			// Count unread messages
			for _, msg := range thread.Messages {
				if msg.ToID == acc.ID && !msg.Read {
					unreadCount++
				}
			}
		}
		
		// Sort threads by latest message time
		sort.Slice(threads, func(i, j int) bool {
			return threads[i].Latest.CreatedAt.After(threads[j].Latest.CreatedAt)
		})
		
		// Render threads
		for _, thread := range threads {
			if thread.Root.ToID == acc.ID || thread.Latest.ToID == acc.ID {
				// Inbox message - show latest preview, link to root
				items = append(items, renderThreadPreview(thread.Root.ID, thread.Latest, acc.ID, thread.HasUnread))
			}
		}
	} else {
		// Sent view - show threads where user is sender
		threads := make([]*Thread, 0)
		for _, thread := range userInbox.Threads {
			if thread.Root.FromID == acc.ID {
				threads = append(threads, thread)
			}
		}
		
		sort.Slice(threads, func(i, j int) bool {
			return threads[i].Latest.CreatedAt.After(threads[j].Latest.CreatedAt)
		})
		
		for _, thread := range threads {
			// Show latest message in thread, not just root
			items = append(items, renderSentThreadPreview(thread.Root.ID, thread.Latest, acc.ID))
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
			<a href="/mail?compose=true" style="color: #666; text-decoration: none; font-size: 14px;">Write a Message</a>
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

// renderThreadPreview renders a thread preview showing the latest message but linking to root
func renderThreadPreview(rootID string, latestMsg *Message, viewerID string, hasUnread bool) string {
	unreadIndicator := ""
	if hasUnread {
		unreadIndicator = `<span style="color: #007bff; font-weight: bold;">● </span>`
	}

	// Format sender name/email
	fromDisplay := latestMsg.FromID
	if !IsExternalEmail(latestMsg.FromID) {
		fromDisplay = latestMsg.FromID
	} else if latestMsg.From != latestMsg.FromID {
		fromDisplay = latestMsg.From
	}

	// Truncate body for preview
	bodyPreview := latestMsg.Body
	if strings.HasPrefix(bodyPreview, "base64:") || len(bodyPreview) > 500 {
		bodyPreview = "[Message]"
	} else {
		if len(bodyPreview) > 100 {
			bodyPreview = bodyPreview[:100] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		if len(bodyPreview) > 80 {
			bodyPreview = bodyPreview[:80] + "..."
		}
	}

	relativeTime := app.TimeAgo(latestMsg.CreatedAt)
	
	html := fmt.Sprintf(`
		<div style="padding: 15px 0; border-bottom: 1px solid #eee; cursor: pointer;" onclick="window.location.href='/mail?id=%s'">
			<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
				<strong style="font-size: 16px;">%s%s</strong>
				<span style="color: #888; font-size: 12px;">%s</span>
			</div>
			<div style="color: #666; font-size: 14px; margin-bottom: 4px;">%s</div>
			<div style="color: #999; font-size: 13px;">%s</div>
		</div>
	`, rootID, unreadIndicator, fromDisplay, relativeTime, latestMsg.Subject, bodyPreview)

	return html
}

// renderSentThreadPreview renders a sent thread preview showing latest message
func renderSentThreadPreview(rootID string, latestMsg *Message, viewerID string) string {
	// Format recipient name/email (use latest message recipient)
	toDisplay := latestMsg.ToID
	if !IsExternalEmail(latestMsg.ToID) {
		// Internal user
		toDisplay = latestMsg.ToID
	} else if latestMsg.To != latestMsg.ToID {
		// External with name
		toDisplay = latestMsg.To
	}

	// Truncate body for preview
	bodyPreview := latestMsg.Body
	if strings.HasPrefix(bodyPreview, "base64:") || len(bodyPreview) > 500 {
		bodyPreview = "[Message]"
	} else {
		if len(bodyPreview) > 100 {
			bodyPreview = bodyPreview[:100] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		if len(bodyPreview) > 80 {
			bodyPreview = bodyPreview[:80] + "..."
		}
	}

	relativeTime := app.TimeAgo(latestMsg.CreatedAt)
	
	html := fmt.Sprintf(`
		<div style="padding: 15px 0; border-bottom: 1px solid #eee; cursor: pointer;" onclick="window.location.href='/mail?id=%s'">
			<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
				<strong style="font-size: 16px;">%s</strong>
				<span style="color: #888; font-size: 12px;">%s</span>
			</div>
			<div style="color: #666; font-size: 14px; margin-bottom: 4px;">%s</div>
			<div style="color: #999; font-size: 13px;">to %s</div>
		</div>
	`, rootID, latestMsg.Subject, relativeTime, bodyPreview, toDisplay)

	return html
}

// renderInboxMessageWithUnread renders a single inbox message with explicit unread flag
func renderInboxMessageWithUnread(msg *Message, indent int, viewerID string, hasUnread bool) string {
	unreadIndicator := ""
	if hasUnread {
		unreadIndicator = `<span style="color: #007bff; font-weight: bold;">● </span>`
	}

	// Format sender name/email
	fromDisplay := msg.FromID
	if !IsExternalEmail(msg.FromID) {
		// Internal user - just show username
		fromDisplay = msg.FromID
	} else if msg.From != msg.FromID {
		// External with name
		fromDisplay = msg.From
	}

	// Truncate body for preview (first 100 chars) - avoid base64 content
	bodyPreview := msg.Body
	// Skip base64 encoded content in preview
	if strings.HasPrefix(bodyPreview, "base64:") || len(bodyPreview) > 500 {
		bodyPreview = "[Message]"
	} else {
		if len(bodyPreview) > 100 {
			bodyPreview = bodyPreview[:100] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		// Truncate long URLs
		if len(bodyPreview) > 80 {
			bodyPreview = bodyPreview[:80] + "..."
		}
	}

	return fmt.Sprintf(`<div class="message-item" style="padding: 15px 0; border-bottom: 1px solid #eee;">
		<h3 style="margin: 0 0 5px 0; font-size: 16px;"><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s%s</a></h3>
		<div style="margin-bottom: 5px; color: #666; font-size: 14px; word-wrap: break-word; overflow-wrap: break-word;">%s</div>
		<div class="info" style="color: #666; font-size: small;">%s from %s</div>
	</div>`, msg.ID, unreadIndicator, msg.Subject, bodyPreview, app.TimeAgo(msg.CreatedAt), fromDisplay)
}

// renderSentMessage renders a single sent message
func renderSentMessage(msg *Message) string {
	// Format recipient name/email
	toDisplay := msg.ToID
	if !IsExternalEmail(msg.ToID) {
		// Internal user - just show username
		toDisplay = msg.ToID
	} else if msg.To != msg.ToID {
		// External with name
		toDisplay = msg.To
	}

	// Truncate body for preview (first 100 chars) - avoid base64 content
	bodyPreview := msg.Body
	// Skip base64 encoded content in preview
	if strings.HasPrefix(bodyPreview, "base64:") || len(bodyPreview) > 500 {
		bodyPreview = "[Message]"
	} else {
		if len(bodyPreview) > 100 {
			bodyPreview = bodyPreview[:100] + "..."
		}
		bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
		// Truncate long URLs
		if len(bodyPreview) > 80 {
			bodyPreview = bodyPreview[:80] + "..."
		}
	}

	return fmt.Sprintf(`<div class="message-item" style="padding: 15px 0; border-bottom: 1px solid #eee;">
		<h3 style="margin: 0 0 5px 0; font-size: 16px;"><a href="/mail?id=%s" style="text-decoration: none; color: inherit;">%s</a></h3>
		<div style="margin-bottom: 5px; color: #666; font-size: 14px; word-wrap: break-word; overflow-wrap: break-word;">%s</div>
		<div class="info" style="color: #666; font-size: small;">%s to %s</div>
	</div>`, msg.ID, msg.Subject, bodyPreview, app.TimeAgo(msg.CreatedAt), toDisplay)
}

// renderSentMessageInThread renders a sent message as part of a thread (same styling as renderSentMessage)
func renderSentMessageInThread(msg *Message) string {
	return renderSentMessage(msg)
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
	
	// Compute ThreadID
	mutex.Lock()
	if replyTo != "" {
		parent := GetMessageUnlocked(replyTo)
		if parent != nil {
			msg.ThreadID = computeThreadID(parent)
		} else {
			msg.ThreadID = msg.ID // Orphaned reply becomes its own root
		}
	} else {
		msg.ThreadID = msg.ID // Root message
	}
	
	messages = append([]*Message{msg}, messages...)
	rebuildInboxes()
	err := save()
	mutex.Unlock()

	return err
}

// GetUnreadCount returns the number of unread messages for a user
func GetUnreadCount(userID string) int {
	mutex.RLock()
	defer mutex.RUnlock()

	count := 0
	userInbox := inboxes[userID]
	if userInbox != nil {
		for _, thread := range userInbox.Threads {
			for _, msg := range thread.Messages {
				if msg.ToID == userID && !msg.Read {
					count++
				}
			}
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
			rebuildInboxes()
			return save()
		}
	}
	return fmt.Errorf("message not found")
}

// DeleteThread removes all messages in a thread
func DeleteThread(msgID, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	// Find the message
	var msg *Message
	for _, m := range messages {
		if m.ID == msgID && (m.FromID == userID || m.ToID == userID) {
			msg = m
			break
		}
	}

	if msg == nil {
		return fmt.Errorf("message not found")
	}

	// Use ThreadID for O(1) thread identification
	threadID := msg.ThreadID
	if threadID == "" {
		threadID = computeThreadID(msg)
	}

	// Delete all messages in this thread
	var remaining []*Message
	for _, m := range messages {
		if m.ThreadID != threadID {
			remaining = append(remaining, m)
		}
	}

	deleted := len(messages) - len(remaining)
	if deleted == 0 {
		return fmt.Errorf("no messages to delete")
	}

	messages = remaining
	rebuildInboxes()
	app.Log("mail", "Deleted %d messages from thread for user %s", deleted, userID)
	return save()
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
