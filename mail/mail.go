package mail

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
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

	// Try to load DKIM config if keys exist (optional)
	dkimDomain := os.Getenv("MAIL_DOMAIN")
	if dkimDomain == "" {
		dkimDomain = "localhost"
	}
	dkimSelector := os.Getenv("MAIL_SELECTOR")
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
		// Compute ThreadID if missing
		threadID = computeThreadID(msg)
		if threadID == "" {
			threadID = msg.ID
		}
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

	// Restrict mail to admins and members only
	if !acc.Admin && !acc.Member {
		http.Error(w, "Mail access restricted to members only", http.StatusForbidden)
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
			// Use the account name as display name
			displayName := acc.Name

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
	action := r.URL.Query().Get("action")

	// Handle download attachment action
	if action == "download_attachment" && msgID != "" {
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

		trimmed := strings.TrimSpace(msg.Body)

		// Check if it's gzip (should not be downloaded, just displayed)
		if len(trimmed) >= 2 && trimmed[0] == 0x1f && trimmed[1] == 0x8b {
			http.Error(w, "This content should be displayed inline, not downloaded", http.StatusBadRequest)
			return
		}

		// Check if it's raw binary data (ZIP file)
		if len(trimmed) >= 2 && trimmed[0] == 'P' && trimmed[1] == 'K' {
			filename := "attachment.zip"
			if strings.Contains(strings.ToLower(msg.FromID), "dmarc") {
				filename = "dmarc-report.zip"
			}

			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(trimmed)))
			w.Write([]byte(trimmed))
			return
		}

		// Try base64 decode
		if looksLikeBase64(msg.Body) {
			if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
				// Check if it's gzip (should be displayed, not downloaded)
				if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
					http.Error(w, "This content should be displayed inline, not downloaded", http.StatusBadRequest)
					return
				}

				// Determine filename and content type
				filename := "attachment.bin"
				contentType := "application/octet-stream"

				if len(decoded) >= 2 && decoded[0] == 'P' && decoded[1] == 'K' {
					filename = "attachment.zip"
					contentType = "application/zip"
					if strings.Contains(strings.ToLower(msg.FromID), "dmarc") {
						filename = "dmarc-report.zip"
					}
				}

				w.Header().Set("Content-Type", contentType)
				w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(decoded)))
				w.Write(decoded)
				return
			}
		}

		http.Error(w, "Attachment not found or invalid", http.StatusBadRequest)
		return
	}

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
		isAttachment := false
		attachmentName := ""

		trimmed := strings.TrimSpace(displayBody)

		// Check if body is gzip compressed (DMARC reports are often .xml.gz)
		if len(trimmed) >= 2 && trimmed[0] == 0x1f && trimmed[1] == 0x8b {
			if reader, err := gzip.NewReader(strings.NewReader(trimmed)); err == nil {
				if content, err := io.ReadAll(reader); err == nil {
					reader.Close()
					if isValidUTF8Text(content) {
						displayBody = string(content)
						app.Log("mail", "Decompressed gzip body for display (%d bytes)", len(content))
					} else {
						app.Log("mail", "Gzip content is not valid text")
					}
				} else {
					app.Log("mail", "Failed to read gzip: %v", err)
				}
			} else {
				app.Log("mail", "Failed to create gzip reader: %v", err)
			}
		} else if len(trimmed) >= 2 && trimmed[0] == 'P' && trimmed[1] == 'K' {
			// ZIP file - try to extract contents for display
			if extracted := extractZipContents([]byte(trimmed), msg.FromID); extracted != "" {
				// Try to render as DMARC report
				if dmarcHTML := renderDMARCReport(extracted); dmarcHTML != "" {
					displayBody = dmarcHTML
					app.Log("mail", "Rendered DMARC report from raw ZIP (%d bytes)", len(trimmed))
				} else {
					displayBody = fmt.Sprintf(`<pre style="background: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; font-size: 12px; line-height: 1.5;">%s</pre>`, html.EscapeString(extracted))
					app.Log("mail", "Extracted and displayed raw ZIP contents (%d bytes)", len(trimmed))
				}
			} else {
				// Extraction failed - show download link
				isAttachment = true
				attachmentName = "attachment.zip"
				if strings.Contains(strings.ToLower(msg.FromID), "dmarc") {
					attachmentName = "dmarc-report.zip"
				}
				displayBody = fmt.Sprintf(`<p>📎 <a href="/mail?action=download_attachment&msg_id=%s" download="%s">%s</a></p>`, msg.ID, attachmentName, attachmentName)
				app.Log("mail", "Could not extract raw ZIP, showing download link: %s (%d bytes)", attachmentName, len(trimmed))
			}
		} else if looksLikeBase64(displayBody) {
			// Try base64 decode
			if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil {
				// Log first few bytes for debugging
				if len(decoded) >= 4 {
					app.Log("mail", "Decoded body first bytes: %02x %02x %02x %02x", decoded[0], decoded[1], decoded[2], decoded[3])
				}
				// Check if decoded data is gzip compressed
				if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
					if reader, err := gzip.NewReader(bytes.NewReader(decoded)); err == nil {
						if content, err := io.ReadAll(reader); err == nil {
							reader.Close()
							if isValidUTF8Text(content) {
								displayBody = string(content)
								app.Log("mail", "Decompressed base64-encoded gzip body for display (%d bytes)", len(content))
							}
						}
					}
				} else if len(decoded) >= 2 && decoded[0] == 'P' && decoded[1] == 'K' {
					// ZIP file - try to extract contents for display
					if extracted := extractZipContents(decoded, msg.FromID); extracted != "" {
						// Try to render as DMARC report
						if dmarcHTML := renderDMARCReport(extracted); dmarcHTML != "" {
							displayBody = dmarcHTML
							isAttachment = true // Skip linkifyURLs for pre-rendered HTML
							app.Log("mail", "SET displayBody to DMARC HTML (%d bytes)", len(dmarcHTML))
						} else {
							displayBody = fmt.Sprintf(`<pre style="background: #f5f5f5; padding: 15px; border-radius: 5px; overflow-x: auto; font-size: 12px; line-height: 1.5;">%s</pre>`, html.EscapeString(extracted))
							app.Log("mail", "SET displayBody to raw XML in pre tags (%d bytes)", len(extracted))
						}
					} else {
						// Extraction failed - show download link
						isAttachment = true
						attachmentName = "attachment.zip"
						if strings.Contains(strings.ToLower(msg.FromID), "dmarc") {
							attachmentName = "dmarc-report.zip"
						}
						displayBody = fmt.Sprintf(`<p>📎 <a href="/mail?action=download_attachment&msg_id=%s" download="%s">%s</a></p>`, msg.ID, attachmentName, attachmentName)
						app.Log("mail", "Could not extract ZIP, showing download link: %s (%d bytes)", attachmentName, len(decoded))
					}
				} else if isValidUTF8Text(decoded) {
					displayBody = string(decoded)
					app.Log("mail", "Decoded base64 body for display")
				}
			}
		}

		// Process email body - renders markdown if detected, otherwise linkifies URLs
		displayBody = renderEmailBody(displayBody, isAttachment)

		// Prepare reply subject
		replySubject := msg.Subject
		if !strings.HasPrefix(strings.ToLower(msg.Subject), "re:") {
			replySubject = "Re: " + msg.Subject
		}

		// Build thread view - use pre-built inbox structure for efficiency
		var thread []*Message
		mutex.RLock()
		
		// Get thread ID from the message
		threadID := msg.ThreadID
		if threadID == "" {
			threadID = computeThreadID(msg)
			if threadID == "" {
				threadID = msg.ID
			}
		}
		
		// Look up thread from inbox structure
		inbox := inboxes[acc.ID]
		if inbox != nil && inbox.Threads[threadID] != nil {
			// Use pre-built thread
			thread = append([]*Message{}, inbox.Threads[threadID].Messages...)
		} else {
			// Fallback: build thread manually (shouldn't normally happen)
			thread = []*Message{msg}
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
			msgIsAttachment := false

			// Check for gzip or ZIP file
			trimmedBody := strings.TrimSpace(msgBody)
			if len(trimmedBody) >= 2 && trimmedBody[0] == 0x1f && trimmedBody[1] == 0x8b {
				// Gzip compressed - decompress and display
				if reader, err := gzip.NewReader(strings.NewReader(trimmedBody)); err == nil {
					if content, err := io.ReadAll(reader); err == nil {
						reader.Close()
						if isValidUTF8Text(content) {
							msgBody = fmt.Sprintf(`<pre style="background: #f5f5f5; padding: 10px; border-radius: 3px; overflow-x: auto; font-size: 11px; line-height: 1.4;">%s</pre>`, string(content))
						}
					}
				}
			} else if len(trimmedBody) >= 2 && trimmedBody[0] == 'P' && trimmedBody[1] == 'K' {
				// ZIP file - try to extract
				if extracted := extractZipContents([]byte(trimmedBody), m.FromID); extracted != "" {
					// Try to render as DMARC report
					if dmarcHTML := renderDMARCReport(extracted); dmarcHTML != "" {
						msgBody = dmarcHTML
						msgIsAttachment = true // Skip linkifyURLs for pre-rendered HTML
					} else {
						msgBody = fmt.Sprintf(`<pre style="background: #f5f5f5; padding: 10px; border-radius: 3px; overflow-x: auto; font-size: 11px; line-height: 1.4;">%s</pre>`, html.EscapeString(extracted))
					}
				} else {
					// Extraction failed - show download link
					msgIsAttachment = true
					attachName := "attachment.zip"
					if strings.Contains(strings.ToLower(m.FromID), "dmarc") {
						attachName = "dmarc-report.zip"
					}
					msgBody = fmt.Sprintf(`📎 <a href="/mail?action=download_attachment&msg_id=%s" download="%s">%s</a>`, m.ID, attachName, attachName)
				}
			} else if looksLikeBase64(msgBody) {
				if decoded, err := base64.StdEncoding.DecodeString(trimmedBody); err == nil {
					// Check if decoded data is gzip
					if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
						if reader, err := gzip.NewReader(bytes.NewReader(decoded)); err == nil {
							if content, err := io.ReadAll(reader); err == nil {
								reader.Close()
								if isValidUTF8Text(content) {
									msgBody = fmt.Sprintf(`<pre style="background: #f5f5f5; padding: 10px; border-radius: 3px; overflow-x: auto; font-size: 11px; line-height: 1.4;">%s</pre>`, html.EscapeString(string(content)))
								}
							}
						}
					} else if len(decoded) >= 2 && decoded[0] == 'P' && decoded[1] == 'K' {
						// ZIP file - try to extract
						if extracted := extractZipContents(decoded, m.FromID); extracted != "" {
							// Try to render as DMARC report
							if dmarcHTML := renderDMARCReport(extracted); dmarcHTML != "" {
								msgBody = dmarcHTML
								msgIsAttachment = true // Skip linkifyURLs for pre-rendered HTML
							} else {
								msgBody = fmt.Sprintf(`<pre style="background: #f5f5f5; padding: 10px; border-radius: 3px; overflow-x: auto; font-size: 11px; line-height: 1.4;">%s</pre>`, html.EscapeString(extracted))
							}
						} else {
							// Extraction failed - show download link
							msgIsAttachment = true
							attachName := "attachment.zip"
							if strings.Contains(strings.ToLower(m.FromID), "dmarc") {
								attachName = "dmarc-report.zip"
							}
							msgBody = fmt.Sprintf(`📎 <a href="/mail?action=download_attachment&msg_id=%s" download="%s">%s</a>`, m.ID, attachName, attachName)
						}
					} else if isValidUTF8Text(decoded) {
						msgBody = string(decoded)
					}
				}
			}

			// Process email body - renders markdown if detected, otherwise linkifies URLs
			msgBody = renderEmailBody(msgBody, msgIsAttachment)

			isSent := m.FromID == acc.ID
			authorDisplay := m.FromID
			if isSent {
				authorDisplay = "You"
			} else if !IsExternalEmail(m.FromID) {
				// Internal user - add profile link
				authorDisplay = fmt.Sprintf(`<a href="/@%s" class="mail-link">%s</a>`, m.FromID, m.FromID)
			} else if m.From != m.FromID {
				// External email with display name
				authorDisplay = m.From
			}

			// Card-style layout for messages
			threadHTML.WriteString(fmt.Sprintf(`
		<div class="thread-message">
			<div class="thread-message-header">
				<div class="thread-message-header-text">
					<span class="thread-message-author">%s</span> <span class="thread-message-time">· %s</span>
				</div>
				<a href="#" onclick="if(confirm('Delete this message?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='_method';input1.value='DELETE';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='id';input2.value='%s';form.appendChild(input2);var input3=document.createElement('input');input3.type='hidden';input3.name='return_to';input3.value='%s';form.appendChild(input3);document.body.appendChild(form);form.submit();}return false;" class="thread-message-delete">×</a>
			</div>
			<div class="thread-message-body">%s</div>
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
			otherPartyDisplay = fmt.Sprintf(`<a href="/@%s" class="mail-link-muted">%s</a>`, otherParty, otherParty)
		}

		// Add block link if other party is external email and user is admin
		blockButton := ""
		if acc.Admin && IsExternalEmail(otherParty) {
			blockButton = fmt.Sprintf(`
			<span style="margin: 0 8px;">·</span>
			<a href="#" onclick="if(confirm('Block %s from sending mail?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='block_sender';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='sender_email';input2.value='%s';form.appendChild(input2);var input3=document.createElement('input');input3.type='hidden';input3.name='msg_id';input3.value='%s';form.appendChild(input3);document.body.appendChild(form);form.submit();}return false;" style="color: #666;">Block Sender</a>
		`, otherParty, otherParty, msg.ID)
		}

		// Build quoted text from the latest message for the reply form
		latestMsg := thread[len(thread)-1]
		quotedBody := formatQuotedText(latestMsg, otherParty)
		// HTML escape the quoted body for display in the collapsed section
		quotedBodyHTML := html.EscapeString(quotedBody)
		quotedBodyHTML = strings.ReplaceAll(quotedBodyHTML, "\n", "<br>")
		// Also escape for use in hidden input value attribute
		quotedBodyValue := html.EscapeString(quotedBody)

		// Get the root ID for reply threading - this is the ID of the latest message being replied to
		replyToID := latestMsg.ID

		messageView := fmt.Sprintf(`
	<div style="color: #666; font-size: small; margin-bottom: 20px;">Thread with: %s</div>
	%s
	<div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #e0e0e0;">
		<form method="POST" action="/mail?id=%s" style="display: flex; flex-direction: column; gap: 15px;" onsubmit="var replyText=document.getElementById('reply-body').innerText.trim();if(!replyText){alert('Please write a reply');return false;}var quotedText=document.getElementById('quoted-text-raw').value;document.getElementById('reply-body-hidden').value=replyText+(quotedText?'\n\n'+quotedText:'');return true;">
			<input type="hidden" name="to" value="%s">
			<input type="hidden" name="subject" value="%s">
			<input type="hidden" name="reply_to" value="%s">
			<input type="hidden" id="reply-body-hidden" name="body" value="">
			<input type="hidden" id="quoted-text-raw" value="%s">
			<div id="reply-body" contenteditable="true" style="padding: 15px; border: 1px solid #ddd; border-radius: 4px; font-family: 'Nunito Sans', serif; font-size: inherit; min-height: 100px; outline: none; background: white;" placeholder="Write your reply..."></div>
			<div style="margin:3px 0 0 0">
				<a href="#" onclick="var el=document.getElementById('quoted-text-content');el.style.display=el.style.display==='none'?'block':'none';this.innerHTML=el.style.display==='none'?'<span style=\'color:#888\'>▸</span> Show quoted text':'<span style=\'color:#888\'>▾</span> Hide quoted text';return false;" style="color:#0066cc;text-decoration:none;font-size:13px"><span style="color:#888">▸</span> Show quoted text</a>
			</div>
			<div id="quoted-text-content" style="display:none;border-left:2px solid #ccc;padding-left:10px;margin:5px 0 0 5px;color:#666;font-size:13px;">%s</div>
			<div style="display: flex; gap: 10px; align-items: center;">
				<button type="submit" style="padding: 8px 16px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Send</button>
				<a href="#" onclick="if(confirm('Delete this entire thread?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='delete_thread';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='msg_id';input2.value='%s';form.appendChild(input2);document.body.appendChild(form);form.submit();}return false;" style="color: #dc3545; font-size: 14px;">Delete Thread</a>
				%s
			</div>
		</form>
		<div style="margin-top: 20px;">
			<a href="/mail" style="color: #666; text-decoration: none;">← Back to mail</a>
		</div>
	</div>
`, otherPartyDisplay, threadHTML.String(), msgID, otherParty, replySubject, replyToID, quotedBodyValue, quotedBodyHTML, msg.ID, blockButton)
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
				<textarea name="body" rows="10" placeholder="Write your message..." required style="padding: 10px; font-family: 'Nunito Sans', serif; font-size: 14px; border: 1px solid #ccc; border-radius: 5px; resize: vertical; min-height: 200px;"></textarea>
			<div style="display: flex; gap: 10px; align-items: center;">
				<button type="submit" style="padding: 8px 16px; font-size: 14px; background-color: #333; color: white; border: none; border-radius: 5px; cursor: pointer;">Send</button>
				<a href="%s" style="color: #666; font-size: 14px;">Cancel</a>
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
			// Show threads where user is either sender or recipient of any message
			userInThread := false
			for _, msg := range thread.Messages {
				if msg.ToID == acc.ID {
					userInThread = true
					break
				}
			}
			if userInThread {
				// Inbox message - show latest preview, link to root
				items = append(items, renderThreadPreview(thread.Root.ID, thread.Latest, acc.ID, thread.HasUnread))
			}
		}
	} else {
		// Sent view - show threads where user has sent at least one message
		threads := make([]*Thread, 0)
		for _, thread := range userInbox.Threads {
			// Check if user has sent any message in this thread
			hasSent := false
			for _, msg := range thread.Messages {
				if msg.FromID == acc.ID {
					hasSent = true
					break
				}
			}
			if hasSent {
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
		// Strip HTML tags for preview to prevent layout issues
		bodyPreview = stripHTMLTags(bodyPreview)
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
		<div class="thread-preview" onclick="window.location.href='/mail?id=%s'">
			<a href="#" class="delete-btn" onclick="event.stopPropagation(); if(confirm('Delete this conversation?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='delete_thread';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='msg_id';input2.value='%s';form.appendChild(input2);document.body.appendChild(form);form.submit();}return false;" title="Delete conversation">×</a>
			<div style="margin-bottom: 4px;">
				<strong style="font-size: 16px;">%s%s</strong>
			</div>
			<div style="color: #666; font-size: 14px; margin-bottom: 4px;">%s</div>
			<div style="display: flex; justify-content: space-between; align-items: center;">
				<div style="color: #999; font-size: 13px; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">%s</div>
				<span style="color: #888; font-size: 12px; margin-left: 10px; flex-shrink: 0;">%s</span>
			</div>
		</div>
	`, rootID, rootID, unreadIndicator, fromDisplay, latestMsg.Subject, bodyPreview, relativeTime)

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
		// Strip HTML tags for preview to prevent layout issues
		bodyPreview = stripHTMLTags(bodyPreview)
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
		<div class="thread-preview" onclick="window.location.href='/mail?id=%s'">
			<a href="#" class="delete-btn" onclick="event.stopPropagation(); if(confirm('Delete this conversation?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='delete_thread';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='msg_id';input2.value='%s';form.appendChild(input2);document.body.appendChild(form);form.submit();}return false;" title="Delete conversation">×</a>
			<div style="margin-bottom: 4px;">
				<strong style="font-size: 16px;">%s</strong>
			</div>
			<div style="color: #666; font-size: 14px; margin-bottom: 4px;">to %s</div>
			<div style="display: flex; justify-content: space-between; align-items: center;">
				<div style="color: #999; font-size: 13px; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">%s</div>
				<span style="color: #888; font-size: 12px; margin-left: 10px; flex-shrink: 0;">%s</span>
			</div>
		</div>
	`, rootID, rootID, latestMsg.Subject, toDisplay, bodyPreview, relativeTime)

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
		// Strip HTML tags for preview to prevent layout issues
		bodyPreview = stripHTMLTags(bodyPreview)
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
		// Strip HTML tags for preview to prevent layout issues
		bodyPreview = stripHTMLTags(bodyPreview)
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

	// Restrict mail to admins and members only
	if !acc.Admin && !acc.Member {
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

// looksLikeMarkdown checks if text contains markdown formatting
func looksLikeMarkdown(text string) bool {
	// Check for definitive markdown patterns (require full syntax)
	definitivePatterns := []string{
		"**",  // bold (needs two asterisks)
		"__",  // bold (needs two underscores)
		"```", // code block
		"- ",  // unordered list
		"* ",  // unordered list (at start)
	}

	for _, pattern := range definitivePatterns {
		if strings.Contains(text, pattern) {
			return true
		}
	}

	// Check for markdown links [text](url) - need both parts
	if strings.Contains(text, "[") && strings.Contains(text, "](") {
		return true
	}

	// Check for headers (# at start of line)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			return true
		}
	}

	return false
}

// formatQuotedText formats a message as quoted text for replies (Gmail-style)
func formatQuotedText(msg *Message, senderName string) string {
	// Build citation line like "On [date], [sender] wrote:"
	dateStr := msg.CreatedAt.Format("Mon, 2 Jan 2006, 15:04")
	citation := fmt.Sprintf("\n\nOn %s, %s wrote:\n", dateStr, senderName)
	
	// Get the raw body and strip any HTML to plain text for quoting
	body := msg.Body
	body = stripHTMLTags(body)
	body = strings.TrimSpace(body)
	
	// Add > prefix to each line with proper space after >
	lines := strings.Split(body, "\n")
	var quotedLines []string
	for _, line := range lines {
		// Trim any existing leading > characters to avoid double-quoting
		line = strings.TrimSpace(line)
		for strings.HasPrefix(line, ">") {
			line = strings.TrimPrefix(line, ">")
			line = strings.TrimSpace(line)
		}
		// Add proper quote prefix with space
		if line == "" {
			quotedLines = append(quotedLines, ">")
		} else {
			quotedLines = append(quotedLines, "> "+line)
		}
	}
	
	return citation + strings.Join(quotedLines, "\n")
}

// stripHTMLTags removes HTML tags from a string, leaving only text content
// This is used for email previews to prevent HTML from breaking the layout
func stripHTMLTags(s string) string {
	// First, convert block-level HTML elements to newlines to preserve structure
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = strings.ReplaceAll(s, "</blockquote>", "\n")
	s = strings.ReplaceAll(s, "</li>", "\n")
	s = strings.ReplaceAll(s, "</tr>", "\n")
	s = strings.ReplaceAll(s, "</h1>", "\n")
	s = strings.ReplaceAll(s, "</h2>", "\n")
	s = strings.ReplaceAll(s, "</h3>", "\n")
	
	// Simple tag stripper - removes anything between < and >
	var result strings.Builder
	inTag := false
	
	for _, char := range s {
		if char == '<' {
			inTag = true
			continue
		}
		if char == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(char)
		}
	}
	
	// Decode common HTML entities
	text := result.String()
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	
	return text
}

// renderEmailBody processes email body - renders markdown if detected, otherwise linkifies URLs
func renderEmailBody(body string, isAttachment bool) string {
	if isAttachment {
		return body
	}

	// Check if body looks like markdown
	if looksLikeMarkdown(body) {
		// Render markdown to HTML
		rendered := app.RenderString(body)

		// Clean up excessive whitespace while preserving HTML structure
		// Collapse multiple newlines into single newlines
		for strings.Contains(rendered, "\n\n\n") {
			rendered = strings.ReplaceAll(rendered, "\n\n\n", "\n\n")
		}

		// Remove newlines that are just between tags (no content)
		// This preserves formatting inside tags but removes empty space between them
		rendered = strings.ReplaceAll(rendered, ">\n<", "><")
		rendered = strings.ReplaceAll(rendered, ">\n\n<", "><")

		return makeQuotedTextCollapsible(rendered)
	}

	// Otherwise just linkify URLs and handle quoted text
	linked := linkifyURLs(body)
	return makeQuotedTextCollapsible(linked)
}

// makeQuotedTextCollapsible wraps quoted text in a collapsible section like Gmail
func makeQuotedTextCollapsible(body string) string {
	// Quick check - if no quotes present, return as-is
	hasBlockquote := strings.Contains(body, "<blockquote")
	hasTextQuote := strings.Contains(body, ">") && (strings.Contains(body, "&gt;") || strings.Contains(body, "\n>"))
	
	if !hasBlockquote && !hasTextQuote {
		return body
	}
	
	// Check for HTML blockquote tags
	if hasBlockquote {
		// Generate a unique ID for this quoted section
		quoteID := fmt.Sprintf("quote-%d", time.Now().UnixNano())
		
		// Find the blockquote and wrap it
		idx := strings.Index(body, "<blockquote")
		if idx >= 0 {
			before := body[:idx]
			after := body[idx:]
			
			// Look backwards from the blockquote to find a citation line
			// Common patterns: "On [date], [name] wrote:" or lines ending with "wrote:"
			citationStart := idx
			beforeLines := strings.Split(before, "<br>")
			if len(beforeLines) == 0 {
				beforeLines = strings.Split(before, "\n")
			}
			
			// Check last few lines before the blockquote
			for i := len(beforeLines) - 1; i >= 0 && i >= len(beforeLines)-3; i-- {
				line := beforeLines[i]
				lowerLine := strings.ToLower(strings.TrimSpace(line))
				
				// Check if it's a citation line
				if (strings.Contains(lowerLine, "wrote:") || strings.Contains(lowerLine, "writes:")) && 
				   (strings.HasPrefix(lowerLine, "on ") || strings.Contains(lowerLine, ",")) {
					// Found citation line - include it in the collapsible section
					citationStart = strings.LastIndex(body[:idx], line)
					if citationStart >= 0 {
						before = body[:citationStart]
						after = body[citationStart:]
						break
					}
				}
			}
			
			// Add collapsible wrapper with minimal whitespace
			collapsed := before + fmt.Sprintf(`<div style="margin:3px 0 0 0"><a href="#" onclick="var el=document.getElementById('%s');el.style.display=el.style.display==='none'?'block':'none';this.innerHTML=el.style.display==='none'?'<span style=\'color:#888\'>▸</span> Show quoted text':'<span style=\'color:#888\'>▾</span> Hide quoted text';return false;" style="color:#0066cc;text-decoration:none;font-size:13px"><span style="color:#888">▸</span> Show quoted text</a></div><div id="%s" style="display:none;border-left:2px solid #ccc;padding-left:10px;margin:5px 0 0 5px;color:#666">%s</div>`, quoteID, quoteID, after)
			
			return collapsed
		}
	}
	
	// Check for plain text quoted lines (starting with >)
	lines := strings.Split(body, "\n")
	var result strings.Builder
	var quotedLines []string
	inQuote := false
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isQuoted := strings.HasPrefix(trimmed, "&gt;") || strings.HasPrefix(trimmed, ">")
		
		// Check if this is an email citation line (e.g., "On [date], [name] wrote:")
		isCitationLine := false
		if !isQuoted && !inQuote {
			lowerLine := strings.ToLower(trimmed)
			// Common patterns: "On [date]..." or "From:" or "[date]..." ending with "wrote:"
			if (strings.HasPrefix(lowerLine, "on ") && strings.Contains(lowerLine, "wrote:")) ||
			   (strings.Contains(lowerLine, ",") && strings.Contains(lowerLine, "wrote:")) ||
			   (strings.HasPrefix(lowerLine, "from:") && strings.Contains(lowerLine, "@")) {
				// Look ahead to see if next line is quoted
				if i+1 < len(lines) {
					nextTrimmed := strings.TrimSpace(lines[i+1])
					if strings.HasPrefix(nextTrimmed, "&gt;") || strings.HasPrefix(nextTrimmed, ">") {
						isCitationLine = true
						isQuoted = true // Treat it as start of quote
					}
				}
			}
		}
		
		if isQuoted {
			if !inQuote {
				// Start of quoted section
				inQuote = true
			}
			if isCitationLine {
				// Include the citation line as-is
				quotedLines = append(quotedLines, line)
			} else {
				// Remove the > prefix for cleaner display
				cleaned := strings.TrimPrefix(trimmed, "&gt;")
				cleaned = strings.TrimPrefix(cleaned, ">")
				cleaned = strings.TrimSpace(cleaned)
				quotedLines = append(quotedLines, cleaned)
			}
		} else {
			if inQuote {
				// End of quoted section - output the collapsible quote
				if len(quotedLines) > 0 {
					quoteID := fmt.Sprintf("quote-%d-%d", time.Now().UnixNano(), i)
					result.WriteString(fmt.Sprintf(`<div style="margin:3px 0 0 0"><a href="#" onclick="var el=document.getElementById('%s');el.style.display=el.style.display==='none'?'block':'none';this.innerHTML=el.style.display==='none'?'<span style=\'color:#888\'>▸</span> Show quoted text':'<span style=\'color:#888\'>▾</span> Hide quoted text';return false;" style="color:#0066cc;text-decoration:none;font-size:13px"><span style="color:#888">▸</span> Show quoted text</a></div><div id="%s" style="display:none;border-left:2px solid #ccc;padding-left:10px;margin:5px 0 0 5px;color:#666">%s</div>`, quoteID, quoteID, strings.Join(quotedLines, "<br>")))
					quotedLines = nil
				}
				inQuote = false
			}
			// Output non-quoted line
			if i > 0 {
				result.WriteString("<br>")
			}
			result.WriteString(line)
		}
	}
	
	// Handle any remaining quoted lines at the end
	if inQuote && len(quotedLines) > 0 {
		quoteID := fmt.Sprintf("quote-%d-end", time.Now().UnixNano())
		result.WriteString(fmt.Sprintf(`<div style="margin:3px 0 0 0"><a href="#" onclick="var el=document.getElementById('%s');el.style.display=el.style.display==='none'?'block':'none';this.innerHTML=el.style.display==='none'?'<span style=\'color:#888\'>▸</span> Show quoted text':'<span style=\'color:#888\'>▾</span> Hide quoted text';return false;" style="color:#0066cc;text-decoration:none;font-size:13px"><span style="color:#888">▸</span> Show quoted text</a></div><div id="%s" style="display:none;border-left:2px solid #ccc;padding-left:10px;margin:5px 0 0 5px;color:#666">%s</div>`, quoteID, quoteID, strings.Join(quotedLines, "<br>")))
	}
	
	resultStr := result.String()
	// Only return the processed version if we found quotes
	if inQuote || strings.Contains(resultStr, "quote-") {
		return resultStr
	}
	
	return body
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

// extractZipContents extracts all files from a ZIP archive and returns their contents as a string
// Only extracts if sender is a trusted DMARC reporter
func extractZipContents(data []byte, senderEmail string) string {
	// Only auto-extract from trusted DMARC report senders
	trustedSenders := []string{
		"@google.com",
		"@yahoo.com",
		"@outlook.com",
		"@microsoft.com",
		"@amazon.com",
		"@apple.com",
	}

	// Check if sender contains "dmarc" OR is from a trusted domain
	isTrusted := strings.Contains(strings.ToLower(senderEmail), "dmarc")
	if !isTrusted {
		senderLower := strings.ToLower(senderEmail)
		for _, domain := range trustedSenders {
			if strings.HasSuffix(senderLower, domain) {
				isTrusted = true
				break
			}
		}
	}

	if !isTrusted {
		app.Log("mail", "Not extracting ZIP - sender not trusted: %s", senderEmail)
		return "" // Don't auto-extract from unknown senders
	}

	// Size limit: 10MB
	if len(data) > 10*1024*1024 {
		app.Log("mail", "ZIP too large: %d bytes", len(data))
		return ""
	}

	// Log first few bytes for debugging
	if len(data) >= 4 {
		app.Log("mail", "ZIP signature: %02x %02x %02x %02x", data[0], data[1], data[2], data[3])
	}

	// Check if it's actually gzip (DMARC reports are often .xml.gz)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		app.Log("mail", "Detected gzip format, attempting to decompress")
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			app.Log("mail", "Failed to create gzip reader: %v", err)
			return ""
		}
		defer reader.Close()

		content, err := io.ReadAll(reader)
		if err != nil {
			app.Log("mail", "Failed to read gzip: %v", err)
			return ""
		}

		if isValidUTF8Text(content) {
			app.Log("mail", "Successfully decompressed gzip file (%d bytes)", len(content))
			return string(content)
		}
		app.Log("mail", "Gzip content is not valid text")
		return ""
	}

	reader := bytes.NewReader(data)
	zipReader, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		app.Log("mail", "Failed to read ZIP: %v", err)
		return ""
	}

	// Limit number of files
	if len(zipReader.File) > 10 {
		app.Log("mail", "ZIP has too many files: %d", len(zipReader.File))
		return ""
	}

	app.Log("mail", "Extracting ZIP from %s: %d files", senderEmail, len(zipReader.File))

	var result strings.Builder
	filesExtracted := 0
	var singleFileContent string // Store content if it's a single file

	for i, file := range zipReader.File {
		// Limit individual file size: 5MB
		if file.UncompressedSize64 > 5*1024*1024 {
			app.Log("mail", "Skipping large file: %s (%d bytes)", file.Name, file.UncompressedSize64)
			continue
		}

		rc, err := file.Open()
		if err != nil {
			if i > 0 {
				result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
			}
			result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
			result.WriteString(strings.Repeat("-", 80) + "\n\n")
			result.WriteString(fmt.Sprintf("Error opening file: %v\n", err))
			app.Log("mail", "Failed to open file %s: %v", file.Name, err)
			continue
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			if i > 0 {
				result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
			}
			result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
			result.WriteString(strings.Repeat("-", 80) + "\n\n")
			result.WriteString(fmt.Sprintf("Error reading file: %v\n", err))
			app.Log("mail", "Failed to read file %s: %v", file.Name, err)
			continue
		}

		// Only display text content (XML, TXT, etc) - never execute or render HTML
		if isValidUTF8Text(content) {
			filesExtracted++

			// If single file, store raw content without headers
			if len(zipReader.File) == 1 {
				singleFileContent = string(content)
				app.Log("mail", "Extracted single text file: %s (%d bytes)", file.Name, len(content))
			} else {
				// Multiple files - add headers
				if i > 0 {
					result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
				}
				result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
				result.WriteString(strings.Repeat("-", 80) + "\n\n")
				result.WriteString(string(content))
				app.Log("mail", "Extracted text file: %s (%d bytes)", file.Name, len(content))
			}
		} else {
			if i > 0 {
				result.WriteString("\n\n" + strings.Repeat("=", 80) + "\n\n")
			}
			result.WriteString(fmt.Sprintf("File: %s (%d bytes)\n", file.Name, file.UncompressedSize64))
			result.WriteString(strings.Repeat("-", 80) + "\n\n")
			result.WriteString(fmt.Sprintf("[Binary file, %d bytes - not displayed]\n", len(content)))
			app.Log("mail", "Skipped binary file: %s", file.Name)
		}
	}

	if filesExtracted == 0 {
		app.Log("mail", "No text files extracted from ZIP")
		return ""
	}

	app.Log("mail", "Successfully extracted %d files from ZIP", filesExtracted)

	// For single file ZIPs (like DMARC reports), return raw content
	if len(zipReader.File) == 1 && singleFileContent != "" {
		return singleFileContent
	}

	if result.Len() == 0 {
		return ""
	}

	return result.String()
}

// DMARC XML structures
type DMARCReport struct {
	XMLName         xml.Name        `xml:"feedback"`
	ReportMetadata  ReportMetadata  `xml:"report_metadata"`
	PolicyPublished PolicyPublished `xml:"policy_published"`
	Records         []Record        `xml:"record"`
}

type ReportMetadata struct {
	OrgName   string    `xml:"org_name"`
	Email     string    `xml:"email"`
	ReportID  string    `xml:"report_id"`
	DateRange DateRange `xml:"date_range"`
}

type DateRange struct {
	Begin int64 `xml:"begin"`
	End   int64 `xml:"end"`
}

type PolicyPublished struct {
	Domain string `xml:"domain"`
	ADKIM  string `xml:"adkim"`
	ASPF   string `xml:"aspf"`
	P      string `xml:"p"`
	SP     string `xml:"sp"`
	Pct    int    `xml:"pct"`
}

type Record struct {
	Row         Row         `xml:"row"`
	Identifiers Identifiers `xml:"identifiers"`
	AuthResults AuthResults `xml:"auth_results"`
}

type Row struct {
	SourceIP        string          `xml:"source_ip"`
	Count           int             `xml:"count"`
	PolicyEvaluated PolicyEvaluated `xml:"policy_evaluated"`
}

type PolicyEvaluated struct {
	Disposition string `xml:"disposition"`
	DKIM        string `xml:"dkim"`
	SPF         string `xml:"spf"`
}

type Identifiers struct {
	HeaderFrom string `xml:"header_from"`
}

type AuthResults struct {
	DKIM []DKIMResult `xml:"dkim"`
	SPF  []SPFResult  `xml:"spf"`
}

type DKIMResult struct {
	Domain   string `xml:"domain"`
	Result   string `xml:"result"`
	Selector string `xml:"selector"`
}

type SPFResult struct {
	Domain string `xml:"domain"`
	Result string `xml:"result"`
}

// renderDMARCReport parses DMARC XML and renders it as HTML tables
func renderDMARCReport(xmlData string) string {
	app.Log("mail", "renderDMARCReport called with %d bytes, first 200 chars: %s", len(xmlData), xmlData[:min(200, len(xmlData))])

	var report DMARCReport
	if err := xml.Unmarshal([]byte(xmlData), &report); err != nil {
		// Not a DMARC report or invalid XML - return empty to fall back to raw display
		app.Log("mail", "Failed to parse as DMARC report: %v", err)
		return ""
	}

	app.Log("mail", "Successfully parsed DMARC report from %s", report.ReportMetadata.OrgName)

	var html strings.Builder

	// Report metadata
	html.WriteString(`<div style="margin-bottom: 20px;">`)
	html.WriteString(fmt.Sprintf(`<h4 style="margin: 0 0 10px 0;">DMARC Report from %s</h4>`, report.ReportMetadata.OrgName))
	html.WriteString(`<table style="border-collapse: collapse; width: 100%; font-size: 13px;">`)
	html.WriteString(fmt.Sprintf(`<tr><td style="padding: 4px 8px; background: #f5f5f5;"><strong>Report ID:</strong></td><td style="padding: 4px 8px;">%s</td></tr>`, report.ReportMetadata.ReportID))
	html.WriteString(fmt.Sprintf(`<tr><td style="padding: 4px 8px; background: #f5f5f5;"><strong>Domain:</strong></td><td style="padding: 4px 8px;">%s</td></tr>`, report.PolicyPublished.Domain))
	html.WriteString(fmt.Sprintf(`<tr><td style="padding: 4px 8px; background: #f5f5f5;"><strong>Policy:</strong></td><td style="padding: 4px 8px;">%s</td></tr>`, report.PolicyPublished.P))
	html.WriteString(`</table></div>`)

	// Records table
	if len(report.Records) > 0 {
		html.WriteString(`<h4 style="margin: 0 0 10px 0;">Email Results</h4>`)
		html.WriteString(`<table style="border-collapse: collapse; width: 100%; font-size: 12px; border: 1px solid #ddd;">`)
		html.WriteString(`<thead><tr style="background: #f5f5f5;">`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">Source IP</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">Count</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">DKIM</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">SPF</th>`)
		html.WriteString(`<th style="padding: 8px; text-align: left; border: 1px solid #ddd;">Disposition</th>`)
		html.WriteString(`</tr></thead><tbody>`)

		for _, record := range report.Records {
			dkimResult := "none"
			if len(record.AuthResults.DKIM) > 0 {
				dkimResult = record.AuthResults.DKIM[0].Result
			}
			spfResult := "none"
			if len(record.AuthResults.SPF) > 0 {
				spfResult = record.AuthResults.SPF[0].Result
			}

			// Color code results
			dkimColor := "#d4edda" // green
			if dkimResult != "pass" {
				dkimColor = "#f8d7da" // red
			}
			spfColor := "#d4edda"
			if spfResult != "pass" {
				spfColor = "#f8d7da"
			}

			html.WriteString(`<tr>`)
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd;">%s</td>`, record.Row.SourceIP))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd;">%d</td>`, record.Row.Count))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd; background: %s;">%s</td>`, dkimColor, dkimResult))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd; background: %s;">%s</td>`, spfColor, spfResult))
			html.WriteString(fmt.Sprintf(`<td style="padding: 8px; border: 1px solid #ddd;">%s</td>`, record.Row.PolicyEvaluated.Disposition))
			html.WriteString(`</tr>`)
		}

		html.WriteString(`</tbody></table>`)
	}

	result := html.String()
	app.Log("mail", "renderDMARCReport returning %d bytes of HTML", len(result))
	return result
}
