package mail

import (
	"bytes"
	"compress/gzip"

	"encoding/base64"
	"encoding/json"
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

	"mu/wallet"
)

var mutex sync.RWMutex

// stored messages
var messages []*Message

// Inbox organizes messages by thread for a user
type Inbox struct {
	Threads     map[string]*Thread // threadID -> Thread
	UnreadCount int                // Cached unread message count
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
	// Loaded
	

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
		
		// Compute email stats
		recomputeStats()
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
				inboxes[msg.FromID] = &Inbox{Threads: make(map[string]*Thread), UnreadCount: 0}
			}
			addMessageToInbox(inboxes[msg.FromID], msg, msg.FromID)
		}

		// Add to recipient's inbox
		if msg.ToID != "" && msg.ToID != msg.FromID {
			if inboxes[msg.ToID] == nil {
				inboxes[msg.ToID] = &Inbox{Threads: make(map[string]*Thread), UnreadCount: 0}
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

	isUnread := !msg.Read && msg.ToID == userID
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
			HasUnread: isUnread,
		}
		inbox.Threads[threadID] = thread
		if isUnread {
			inbox.UnreadCount++
		}
	} else {
		// Add to existing thread
		thread.Messages = append(thread.Messages, msg)
		if msg.CreatedAt.After(thread.Latest.CreatedAt) {
			thread.Latest = msg
		}
		if isUnread {
			thread.HasUnread = true
			inbox.UnreadCount++
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
// Handler handles mail-related requests
//
// Email Flow:
// - Internal messages: stored directly as HTML, displayed in threads
// - External emails: sent as multipart/alternative (plain text + HTML) with threading headers
// - Threading: uses In-Reply-To and References headers, no quoted text bloat
// - Display: emails shown as-is in thread view, full conversation visible
func Handler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	// All users can access mail for internal DMs
	// External email costs credits (checked at send time)

	// Handle POST - send message or delete
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		// Check if this is a delete action
		if r.FormValue("_method") == "DELETE" {
			msgID := r.FormValue("id")
			if err := DeleteMessage(msgID, acc.ID); err != nil {
				app.ServerError(w, r, "Failed to delete message")
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

		// Send message
		// The form can submit in two ways:
		// 1. Compose form: simple "body" field with plain text
		// 2. Reply form: "body_plain" and "body_html" fields for multipart
		to := strings.TrimSpace(r.FormValue("to"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		bodyPlain := strings.TrimSpace(r.FormValue("body_plain"))
		bodyHTML := strings.TrimSpace(r.FormValue("body_html"))

		// Fallback to "body" field for compose form
		if bodyPlain == "" && bodyHTML == "" {
			body := strings.TrimSpace(r.FormValue("body"))
			if body != "" {
				bodyPlain = body
			}
		}
		replyTo := strings.TrimSpace(r.FormValue("reply_to"))

		if to == "" || subject == "" || bodyPlain == "" {
			http.Error(w, "All fields are required", http.StatusBadRequest)
			return
		}

		// Check if recipient is external (has @domain)
		if IsExternalEmail(to) {
			// External email costs credits (unless admin)
			if !acc.Admin {
				canProceed, useFree, cost, err := wallet.CheckQuota(acc.ID, wallet.OpExternalEmail)
				if err != nil || !canProceed {
					http.Error(w, fmt.Sprintf("External email requires %d credits. Top up at /wallet", cost), http.StatusPaymentRequired)
					return
				}
				// Consume quota after successful send (deferred below)
				_ = useFree
				_ = cost
			}

			// External email - send via SMTP with multipart/alternative (plain text + HTML)
			fromEmail := GetEmailForUser(acc.ID, GetConfiguredDomain())
			displayName := acc.Name

			// Convert plain text to HTML only for the external email
			// The HTML version has <br> for newlines and escapes dangerous chars
			htmlBody := convertPlainTextToHTML(bodyPlain)

			// Send multipart email with threading headers
			messageID, err := SendExternalEmail(displayName, fromEmail, to, subject, bodyPlain, htmlBody, replyTo)
			if err != nil {
				http.Error(w, "Failed to send email: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Consume quota for external email (after successful send)
			if !acc.Admin {
				if err := wallet.ConsumeQuota(acc.ID, wallet.OpExternalEmail); err != nil {
					app.Log("mail", "Warning: Failed to consume quota for external email: %v", err)
				}
			}

			// Store plain text in sent messages - render to HTML only at display time
			if err := SendMessage(acc.Name, acc.ID, to, to, subject, bodyPlain, replyTo, messageID); err != nil {
				app.Log("mail", "Warning: Failed to store sent message: %v", err)
			}
		} else {
			// Internal message - store plain text, render at display time
			toAcc, err := auth.GetAccount(to)
			if err != nil {
				http.Error(w, "Recipient not found", http.StatusNotFound)
				return
			}

			app.Log("mail", "Sending internal message from %s to %s with replyTo=%s", acc.Name, toAcc.Name, replyTo)
			if err := SendMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, subject, bodyPlain, replyTo, ""); err != nil {
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

	// Handle view raw source action
	if action == "view_raw" && msgID != "" {
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

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nDate: %s\n\n%s",
			msg.FromID, msg.ToID, msg.Subject, msg.CreatedAt.Format(time.RFC1123), msg.Body)))
		return
	}

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

		// First, check if body contains raw MIME content with headers
		if strings.Contains(displayBody, "Content-Type:") || strings.Contains(displayBody, "content-type:") {
			if extracted := extractMIMEBody(displayBody); extracted != displayBody {
				displayBody = extracted
			}
		}

		trimmed := strings.TrimSpace(displayBody)

		// Check if body contains mixed content: base64 gzip data followed by MIME multipart markers
		// This happens with some DMARC reports from Microsoft that have inline gzip followed by HTML explanation
		if strings.Contains(trimmed, "[multipart/") || strings.Contains(trimmed, "\n--") {
			// Split at the first boundary marker or multipart notation
			var gzipPart string
			// Check for [multipart/ with various whitespace prefixes
			if idx := strings.Index(trimmed, "\n\n[multipart/"); idx > 0 {
				gzipPart = strings.TrimSpace(trimmed[:idx])
			} else if idx := strings.Index(trimmed, "\n[multipart/"); idx > 0 {
				gzipPart = strings.TrimSpace(trimmed[:idx])
			} else if idx := strings.Index(trimmed, "[multipart/"); idx > 0 {
				gzipPart = strings.TrimSpace(trimmed[:idx])
			} else if idx := strings.Index(trimmed, "\n--"); idx > 0 {
				gzipPart = strings.TrimSpace(trimmed[:idx])
			}

			if gzipPart != "" && looksLikeBase64(gzipPart) {
				// Try to decode the gzip part
				if decoded, err := base64.StdEncoding.DecodeString(gzipPart); err == nil {
					if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
						if reader, err := gzip.NewReader(bytes.NewReader(decoded)); err == nil {
							if content, err := io.ReadAll(reader); err == nil {
								reader.Close()
								if isValidUTF8Text(content) {
									// Try to render as DMARC report
									if dmarcHTML := renderDMARCReport(string(content)); dmarcHTML != "" {
										displayBody = dmarcHTML
										isAttachment = true // Skip linkifyURLs for pre-rendered HTML
										app.Log("mail", "Rendered DMARC report from mixed content gzip (%d bytes)", len(content))
									} else {
										displayBody = fmt.Sprintf(`<pre class="code-block">%s</pre>`, html.EscapeString(string(content)))
										app.Log("mail", "Displayed raw XML from mixed content gzip (%d bytes)", len(content))
									}
									// Skip further processing since we handled this specially
									goto skipBodyProcessing
								}
							}
						}
					}
				}
			}
		}

		// Check if body is gzip compressed (DMARC reports are often .xml.gz)
		if len(trimmed) >= 2 && trimmed[0] == 0x1f && trimmed[1] == 0x8b {
			if reader, err := gzip.NewReader(strings.NewReader(trimmed)); err == nil {
				if content, err := io.ReadAll(reader); err == nil {
					reader.Close()
					if isValidUTF8Text(content) {
						// Try to render as DMARC report
						if dmarcHTML := renderDMARCReport(string(content)); dmarcHTML != "" {
							displayBody = dmarcHTML
							isAttachment = true // Skip linkifyURLs for pre-rendered HTML
							app.Log("mail", "Rendered DMARC report from raw gzip (%d bytes)", len(content))
						} else {
							displayBody = string(content)
							app.Log("mail", "Decompressed gzip body for display (%d bytes)", len(content))
						}
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
					displayBody = fmt.Sprintf(`<pre class="code-block">%s</pre>`, html.EscapeString(extracted))
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
								// Try to render as DMARC report
								if dmarcHTML := renderDMARCReport(string(content)); dmarcHTML != "" {
									displayBody = dmarcHTML
									isAttachment = true // Skip linkifyURLs for pre-rendered HTML
									app.Log("mail", "Rendered DMARC report from base64-gzip (%d bytes)", len(content))
								} else {
									displayBody = string(content)
									app.Log("mail", "Decompressed base64-encoded gzip body for display (%d bytes)", len(content))
								}
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
							displayBody = fmt.Sprintf(`<pre class="code-block">%s</pre>`, html.EscapeString(extracted))
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

	skipBodyProcessing:
		// Process email body - renders markdown if detected, otherwise linkifies URLs
		displayBody = renderEmailBody(displayBody, isAttachment)

		// Prepare reply subject (decode MIME encoded subject first)
		decodedSubject := decodeMIMEHeader(msg.Subject)
		replySubject := decodedSubject
		if !strings.HasPrefix(strings.ToLower(decodedSubject), "re:") {
			replySubject = "Re: " + decodedSubject
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

			// First, check if body contains raw MIME content with headers
			if strings.Contains(msgBody, "Content-Type:") || strings.Contains(msgBody, "content-type:") {
				if extracted := extractMIMEBody(msgBody); extracted != msgBody {
					msgBody = extracted
				}
			}

			// Check for gzip or ZIP file
			trimmedBody := strings.TrimSpace(msgBody)

			// Check for mixed content: base64 gzip data followed by MIME multipart markers
			// This happens with some DMARC reports from Microsoft
			if strings.Contains(trimmedBody, "[multipart/") || strings.Contains(trimmedBody, "\n--") {
				var gzipPart string
				if idx := strings.Index(trimmedBody, "\n\n[multipart/"); idx > 0 {
					gzipPart = strings.TrimSpace(trimmedBody[:idx])
				} else if idx := strings.Index(trimmedBody, "\n[multipart/"); idx > 0 {
					gzipPart = strings.TrimSpace(trimmedBody[:idx])
				} else if idx := strings.Index(trimmedBody, "[multipart/"); idx > 0 {
					gzipPart = strings.TrimSpace(trimmedBody[:idx])
				} else if idx := strings.Index(trimmedBody, "\n--"); idx > 0 {
					gzipPart = strings.TrimSpace(trimmedBody[:idx])
				}

				if gzipPart != "" && looksLikeBase64(gzipPart) {
					if decoded, err := base64.StdEncoding.DecodeString(gzipPart); err == nil {
						if len(decoded) >= 2 && decoded[0] == 0x1f && decoded[1] == 0x8b {
							if reader, err := gzip.NewReader(bytes.NewReader(decoded)); err == nil {
								if content, err := io.ReadAll(reader); err == nil {
									reader.Close()
									if isValidUTF8Text(content) {
										if dmarcHTML := renderDMARCReport(string(content)); dmarcHTML != "" {
											msgBody = dmarcHTML
											msgIsAttachment = true
										} else {
											msgBody = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(string(content)))
										}
										goto threadSkipBodyProcessing
									}
								}
							}
						}
					}
				}
			}

			if len(trimmedBody) >= 2 && trimmedBody[0] == 0x1f && trimmedBody[1] == 0x8b {
				// Gzip compressed - decompress and display
				if reader, err := gzip.NewReader(strings.NewReader(trimmedBody)); err == nil {
					if content, err := io.ReadAll(reader); err == nil {
						reader.Close()
						if isValidUTF8Text(content) {
							msgBody = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, string(content))
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
						msgBody = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(extracted))
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
									msgBody = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(string(content)))
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
								msgBody = fmt.Sprintf(`<pre class="code-block-sm">%s</pre>`, html.EscapeString(extracted))
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

		threadSkipBodyProcessing:
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
			<div class="mt-3 border-t pt-3 text-xs">
				<a href="/mail?action=view_raw&id=%s" class="text-muted" target="_blank">View Raw</a>
			</div>
		</div>`, authorDisplay, app.TimeAgo(m.CreatedAt), m.ID, msgID, msgBody, m.ID))
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
			<span class="mx-2">·</span>
			<a href="#" onclick="if(confirm('Block %s from sending mail?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='block_sender';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='sender_email';input2.value='%s';form.appendChild(input2);var input3=document.createElement('input');input3.type='hidden';input3.name='msg_id';input3.value='%s';form.appendChild(input3);document.body.appendChild(form);form.submit();}return false;" class="text-muted">Block Sender</a>
		`, otherParty, otherParty, msg.ID)
		}

		// Get the root ID for reply threading - this is the ID of the latest message being replied to
		latestMsg := thread[len(thread)-1]
		replyToID := latestMsg.ID

		messageView := fmt.Sprintf(`
	<div class="text-muted text-sm mb-5">Thread with: %s</div>
	%s
	<div class="mt-6 border-t pt-5">
		<form method="POST" action="/mail?id=%s" class="d-flex flex-column gap-4" onsubmit="var replyText=document.getElementById('reply-body').innerText.trim().replace(/\n{3,}/g,'\n\n');if(!replyText){alert('Please write a reply');return false;}document.getElementById('reply-body-plain').value=replyText;var replyHTML=replyText.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/\n/g,'<br>');document.getElementById('reply-body-html').value=replyHTML;return true;">
			<input type="hidden" name="to" value="%s">
			<input type="hidden" name="subject" value="%s">
			<input type="hidden" name="reply_to" value="%s">
			<input type="hidden" id="reply-body-plain" name="body_plain" value="">
			<input type="hidden" id="reply-body-html" name="body_html" value="">
			<div id="reply-body" contenteditable="true" class="mail-reply-box" placeholder="Write your reply..."></div>
			<div class="d-flex gap-3 items-center">
				<button type="submit">Send</button>
				<a href="#" onclick="if(confirm('Delete this entire thread?')){var form=document.createElement('form');form.method='POST';form.action='/mail';var input1=document.createElement('input');input1.type='hidden';input1.name='action';input1.value='delete_thread';form.appendChild(input1);var input2=document.createElement('input');input2.type='hidden';input2.name='msg_id';input2.value='%s';form.appendChild(input2);document.body.appendChild(form);form.submit();}return false;" class="text-error text-sm">Delete Thread</a>
				%s
			</div>
		</form>
		<div class="mt-5">
			<a href="/mail" class="text-muted">← Back to mail</a>
		</div>
	</div>
`, otherPartyDisplay, threadHTML.String(), msgID, otherParty, replySubject, replyToID, msg.ID, blockButton)
		w.Write([]byte(app.RenderHTML(decodedSubject, "", messageView)))
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
			<form method="POST" action="/mail" class="mail-form">
				<input type="hidden" name="reply_to" value="%s">
				<input type="text" name="to" placeholder="To: username or email" value="%s" required>
				<input type="text" name="subject" placeholder="Subject" value="%s" required>
				<textarea name="body" rows="10" placeholder="Write your message..." required></textarea>
			<div class="d-flex gap-3 items-center">
				<button type="submit">Send</button>
				<a href="%s" class="text-muted text-sm">Cancel</a>
			</div>
		</form>
		<div class="mt-5">
			<a href="%s" class="text-muted">← Back</a>
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

	// Check if requesting unread count only
	if r.URL.Query().Get("unread") == "count" {
		mutex.RLock()
		count := 0
		if inbox := inboxes[acc.ID]; inbox != nil {
			count = inbox.UnreadCount
		}
		mutex.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"count": count})
		return
	}

	// Check if requesting preview for account page
	if r.URL.Query().Get("preview") == "1" {
		preview := GetRecentThreadsPreview(acc.ID, 3)
		unread := GetUnreadCount(acc.ID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"html":   preview,
			"unread": unread,
		})
		return
	}

	// Get messages for this user
	mutex.RLock()
	// Get user's inbox - O(1) lookup
	userInbox := inboxes[acc.ID]
	mutex.RUnlock()

	if userInbox == nil {
		userInbox = &Inbox{Threads: make(map[string]*Thread), UnreadCount: 0}
	}

	// Render threads from pre-organized inbox
	var items []string
	unreadCount := userInbox.UnreadCount // Use cached count instead of recalculating
	if view == "inbox" {
		app.Log("mail", "Rendering inbox with %d threads for user %s", len(userInbox.Threads), acc.Name)

		// Convert threads to slice for sorting
		threads := make([]*Thread, 0, len(userInbox.Threads))
		for _, thread := range userInbox.Threads {
			threads = append(threads, thread)
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
			content = `<p class="text-muted p-5">No sent messages yet.</p>`
		} else {
			content = `<p class="text-muted p-5">No messages yet.</p>`
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

	// Build tab navigation
	inboxClass := "mail-tab active"
	sentClass := "mail-tab"
	if view == "sent" {
		inboxClass = "mail-tab"
		sentClass = "mail-tab active"
	}
	inboxLabel := "Inbox"
	if unreadCount > 0 {
		inboxLabel = fmt.Sprintf("Inbox (%d)", unreadCount)
	}
	tabs := fmt.Sprintf(`<div class="mail-tabs"><a href="/mail" class="%s">%s</a><a href="/mail?view=sent" class="%s">Sent</a></div>`,
		inboxClass, inboxLabel, sentClass)

	pageHTML := app.Page(app.PageOpts{
		Action:  "/mail?compose=true",
		Label:   "+ Compose",
		Filters: tabs,
		Content: `<div id="mailbox">` + content + `</div>`,
	})

	w.Write([]byte(app.RenderHTML(title, "Your messages", pageHTML)))
}

// renderThreadPreview renders a thread preview showing the latest message but linking to root
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

	// Update stats (outside lock)
	updateStats(msg)

	return err
}

// GetUnreadCount returns the number of unread messages for a user
func GetUnreadCount(userID string) int {
	mutex.RLock()
	defer mutex.RUnlock()

	if inbox := inboxes[userID]; inbox != nil {
		return inbox.UnreadCount
	}
	return 0
}

// GetRecentThreadsPreview returns HTML preview of recent threads for account page
func GetRecentThreadsPreview(userID string, limit int) string {
	mutex.RLock()
	defer mutex.RUnlock()

	inbox := inboxes[userID]
	if inbox == nil || len(inbox.Threads) == 0 {
		return `<p class="text-muted">No messages</p>`
	}

	// Get threads and sort by latest
	threads := make([]*Thread, 0, len(inbox.Threads))
	for _, thread := range inbox.Threads {
		// Only include threads where user received messages
		for _, msg := range thread.Messages {
			if msg.ToID == userID {
				threads = append(threads, thread)
				break
			}
		}
	}

	if len(threads) == 0 {
		return `<p class="text-muted">No messages</p>`
	}

	sort.Slice(threads, func(i, j int) bool {
		return threads[i].Latest.CreatedAt.After(threads[j].Latest.CreatedAt)
	})

	if limit > 0 && len(threads) > limit {
		threads = threads[:limit]
	}

	var b strings.Builder
	for _, thread := range threads {
		msg := thread.Latest
		unreadDot := ""
		if thread.HasUnread {
			unreadDot = `<span class="unread-dot mr-1">●</span>`
		}
		// Strip HTML and truncate body for preview
		body := stripHTMLTags(msg.Body)
		body = strings.TrimSpace(body)
		if len(body) > 50 {
			body = body[:50] + "..."
		}
		if body == "" {
			body = "(no preview)"
		}
		b.WriteString(fmt.Sprintf(`<div class="py-2 border-b">
			%s<strong>%s</strong>
			<span class="text-muted text-sm ml-2">%s</span>
		</div>`, unreadDot, html.EscapeString(msg.From), html.EscapeString(body)))
	}

	return b.String()
}

// MarkAsRead marks a message as read
func MarkAsRead(msgID, userID string) error {
	mutex.Lock()
	defer mutex.Unlock()

	for _, msg := range messages {
		if msg.ID == msgID && msg.ToID == userID {
			if !msg.Read {
				msg.Read = true

				// Update thread's HasUnread status and decrement UnreadCount
				if inbox := inboxes[userID]; inbox != nil {
					if thread := inbox.Threads[msg.ThreadID]; thread != nil {
						// Decrement unread count
						inbox.UnreadCount--
						if inbox.UnreadCount < 0 {
							inbox.UnreadCount = 0
						}

						// Check if any messages in this thread are still unread
						hasUnread := false
						for _, threadMsg := range thread.Messages {
							if !threadMsg.Read && threadMsg.ToID == userID {
								hasUnread = true
								break
							}
						}
						thread.HasUnread = hasUnread
					}
				}
			}

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

// GetAllMessages returns all messages (for admin use)
// IsExternalAddress checks if an address is external (contains @)
func IsExternalAddress(addr string) bool {
	return strings.Contains(addr, "@") && !strings.HasSuffix(addr, "@"+GetConfiguredDomain())
}

// EmailStats holds pre-computed email statistics
type EmailStats struct {
	Total    int            `json:"total"`
	Inbound  int            `json:"inbound"`
	Outbound int            `json:"outbound"`
	Internal int            `json:"internal"`
	Domains  map[string]int `json:"domains"`
}

var (
	emailStats     EmailStats
	emailStatsMux  sync.RWMutex
)

func init() {
	// Will be computed on first Load()
	emailStats.Domains = make(map[string]int)
}

// updateStats updates stats when a message is added
func updateStats(msg *Message) {
	emailStatsMux.Lock()
	defer emailStatsMux.Unlock()
	
	emailStats.Total++
	
	fromExternal := IsExternalAddress(msg.FromID)
	toExternal := IsExternalAddress(msg.ToID)
	
	if fromExternal {
		emailStats.Inbound++
		if parts := strings.Split(msg.FromID, "@"); len(parts) == 2 {
			emailStats.Domains[parts[1]]++
		}
	} else if toExternal {
		emailStats.Outbound++
		if parts := strings.Split(msg.ToID, "@"); len(parts) == 2 {
			emailStats.Domains[parts[1]]++
		}
	} else {
		emailStats.Internal++
	}
}

// recomputeStats rebuilds stats from all messages (called on load)
func recomputeStats() {
	emailStatsMux.Lock()
	defer emailStatsMux.Unlock()
	
	emailStats = EmailStats{
		Domains: make(map[string]int),
	}
	
	for _, msg := range messages {
		emailStats.Total++
		
		fromExternal := IsExternalAddress(msg.FromID)
		toExternal := IsExternalAddress(msg.ToID)
		
		if fromExternal {
			emailStats.Inbound++
			if parts := strings.Split(msg.FromID, "@"); len(parts) == 2 {
				emailStats.Domains[parts[1]]++
			}
		} else if toExternal {
			emailStats.Outbound++
			if parts := strings.Split(msg.ToID, "@"); len(parts) == 2 {
				emailStats.Domains[parts[1]]++
			}
		} else {
			emailStats.Internal++
		}
	}
}

// GetEmailStats returns current email statistics
func GetEmailStats() EmailStats {
	emailStatsMux.RLock()
	defer emailStatsMux.RUnlock()
	
	// Copy to avoid race
	stats := EmailStats{
		Total:    emailStats.Total,
		Inbound:  emailStats.Inbound,
		Outbound: emailStats.Outbound,
		Internal: emailStats.Internal,
		Domains:  make(map[string]int),
	}
	for k, v := range emailStats.Domains {
		stats.Domains[k] = v
	}
	return stats
}

// GetRecentMessages returns the N most recent messages
func GetRecentMessages(limit int) []*Message {
	mutex.RLock()
	defer mutex.RUnlock()
	
	// Messages are stored newest first (prepended)
	if limit > len(messages) {
		limit = len(messages)
	}
	
	result := make([]*Message, limit)
	copy(result, messages[:limit])
	return result
}

