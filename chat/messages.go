package chat

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/app"
	"mu/auth"
)

// handleMessagesMode handles direct messaging UI and logic
func handleMessagesMode(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}

	// Handle POST - send message
	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			app.BadRequest(w, r, "Failed to parse form")
			return
		}

		to := strings.TrimSpace(r.FormValue("to"))
		body := strings.TrimSpace(r.FormValue("body"))
		replyTo := strings.TrimSpace(r.FormValue("reply_to"))

		if to == "" || body == "" {
			http.Error(w, "Recipient and message are required", http.StatusBadRequest)
			return
		}

		// Check if recipient is @micro (AI assistant)
		if to == "micro" || to == "@micro" {
			// This is handled by the AI chat - redirect
			http.Redirect(w, r, "/chat", http.StatusSeeOther)
			return
		}

		// Look up recipient
		toAcc, err := auth.GetAccountByName(to)
		if err != nil {
			http.Error(w, "Recipient not found", http.StatusNotFound)
			return
		}

		// Send message
		if err := SendChatMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, body, replyTo); err != nil {
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
			return
		}

		// Redirect to thread if replying, otherwise to inbox
		threadID := r.URL.Query().Get("id")
		if threadID != "" {
			http.Redirect(w, r, "/chat?mode=messages&id="+threadID, http.StatusSeeOther)
		} else if replyTo != "" {
			chatMessagesMutex.RLock()
			parentMsg := getChatMessageUnlocked(replyTo)
			chatMessagesMutex.RUnlock()
			if parentMsg != nil {
				http.Redirect(w, r, "/chat?mode=messages&id="+parentMsg.ThreadID, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/chat?mode=messages", http.StatusSeeOther)
			}
		} else {
			http.Redirect(w, r, "/chat?mode=messages", http.StatusSeeOther)
		}
		return
	}

	// Handle GET - show inbox or thread
	msgID := r.URL.Query().Get("id")
	compose := r.URL.Query().Get("compose")

	if msgID != "" {
		// Show thread
		renderChatThread(w, r, msgID, acc)
		return
	}

	if compose != "" {
		// Show compose form
		renderChatCompose(w, r, acc)
		return
	}

	// Show inbox
	renderChatInbox(w, r, acc)
}

// renderChatInbox renders the chat inbox with conversations
func renderChatInbox(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	inbox := GetChatInbox(acc.ID)

	// Get all threads and sort by latest message time
	type threadInfo struct {
		thread *ChatThread
		id     string
	}
	var threads []threadInfo
	for id, thread := range inbox.Threads {
		threads = append(threads, threadInfo{thread: thread, id: id})
	}

	sort.Slice(threads, func(i, j int) bool {
		return threads[i].thread.Latest.CreatedAt.After(threads[j].thread.Latest.CreatedAt)
	})

	// Render thread previews
	var items []string
	for _, t := range threads {
		thread := t.thread
		root := thread.Root
		latest := thread.Latest

		// Determine who we're chatting with
		var otherUser string
		if root.FromID == acc.ID {
			otherUser = root.To
		} else {
			otherUser = root.From
		}

		// Build preview
		unreadMarker := ""
		if thread.HasUnread {
			unreadMarker = `<span class="unread-marker" style="color: #007bff; margin-left: 5px;">●</span>`
		}

		preview := latest.Body
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		preview = html.EscapeString(preview)

		timeAgo := app.TimeAgo(latest.CreatedAt)

		item := fmt.Sprintf(`
			<div class="thread-preview" onclick="window.location.href='/chat?mode=messages&id=%s'" style="padding: 15px; border-bottom: 1px solid #eee; cursor: pointer; background: #fff;">
				<div class="mail-thread-item">
					<strong class="mail-thread-subject">%s%s</strong>
				</div>
				<div class="mail-thread-row" style="display: flex; justify-content: space-between; margin-top: 5px;">
					<div class="mail-thread-preview" style="flex: 1; color: #666;">%s</div>
					<span class="mail-thread-time" style="color: #999; margin-left: 10px;">%s</span>
				</div>
			</div>
		`, root.ID, html.EscapeString(otherUser), unreadMarker, preview, timeAgo)
		items = append(items, item)
	}

	content := `
		<div style="margin-bottom: 20px;">
			<a href="/chat?mode=messages&compose=true" class="button" style="padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; display: inline-block;">New Chat</a>
			<a href="/chat" class="button" style="padding: 10px 20px; background: #6c757d; color: white; text-decoration: none; border-radius: 4px; display: inline-block; margin-left: 10px;">AI Chat</a>
		</div>
	`

	if len(items) == 0 {
		content += `<div style="background: #f9f9f9; padding: 40px; text-align: center; border-radius: 4px; color: #666;">
			<p>No conversations yet.</p>
			<p><a href="/chat?mode=messages&compose=true" style="color: #007bff;">Start a new chat</a> or try chatting with <a href="/chat?mode=messages&compose=true&to=micro" style="color: #007bff;">@micro</a> (AI assistant)</p>
		</div>`
	} else {
		content += `<div style="background: #fff; border: 1px solid #ddd; border-radius: 4px;">` + strings.Join(items, "\n") + `</div>`
	}

	htmlContent := app.RenderHTMLForRequest("Chat Messages", "Direct messages", content, r)
	w.Write([]byte(htmlContent))
}

// renderChatCompose renders the new message compose form
func renderChatCompose(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	to := r.URL.Query().Get("to")

	content := fmt.Sprintf(`
		<div style="margin-bottom: 20px;">
			<a href="/chat?mode=messages" class="button" style="padding: 10px 20px; background: #6c757d; color: white; text-decoration: none; border-radius: 4px; display: inline-block;">← Back to Inbox</a>
		</div>
		<div style="max-width: 600px; background: #fff; padding: 20px; border: 1px solid #ddd; border-radius: 4px;">
			<h2 style="margin-top: 0;">New Chat</h2>
			<form method="POST" action="/chat?mode=messages">
				<div style="margin-bottom: 15px;">
					<label for="to" style="display: block; margin-bottom: 5px; font-weight: bold;">To:</label>
					<input type="text" id="to" name="to" value="%s" placeholder="Username (or 'micro' for AI)" required 
						style="width: 100%%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; box-sizing: border-box;">
					<small style="color: #666; display: block; margin-top: 5px;">Tip: Type 'micro' to chat with the AI assistant</small>
				</div>
				<div style="margin-bottom: 15px;">
					<label for="body" style="display: block; margin-bottom: 5px; font-weight: bold;">Message:</label>
					<textarea id="body" name="body" rows="10" required 
						style="width: 100%%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; box-sizing: border-box; font-family: inherit;"></textarea>
				</div>
				<button type="submit" style="padding: 12px 24px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; font-weight: bold;">Send Message</button>
			</form>
		</div>
	`, html.EscapeString(to))

	htmlContent := app.RenderHTMLForRequest("New Chat", "Compose message", content, r)
	w.Write([]byte(htmlContent))
}

// renderChatThread renders a conversation thread
func renderChatThread(w http.ResponseWriter, r *http.Request, threadID string, acc *auth.Account) {
	chatMessagesMutex.RLock()

	// Get all messages in thread
	var threadMessages []*ChatMessage
	for _, msg := range chatMessages {
		if msg.ThreadID == threadID && (msg.FromID == acc.ID || msg.ToID == acc.ID) {
			threadMessages = append(threadMessages, msg)
		}
	}

	if len(threadMessages) == 0 {
		chatMessagesMutex.RUnlock()
		http.Error(w, "Thread not found", http.StatusNotFound)
		return
	}

	// Sort by created time
	sort.Slice(threadMessages, func(i, j int) bool {
		return threadMessages[i].CreatedAt.Before(threadMessages[j].CreatedAt)
	})

	// Determine who we're chatting with
	firstMsg := threadMessages[0]
	var otherUser string
	var otherUserID string
	if firstMsg.FromID == acc.ID {
		otherUser = firstMsg.To
		otherUserID = firstMsg.ToID
	} else {
		otherUser = firstMsg.From
		otherUserID = firstMsg.FromID
	}

	chatMessagesMutex.RUnlock()

	// Mark all unread messages as read
	for _, msg := range threadMessages {
		if msg.ToID == acc.ID && !msg.Read {
			MarkChatMessageAsRead(msg.ID, acc.ID)
		}
	}

	// Render messages
	var messageHTML []string
	for _, msg := range threadMessages {
		isSent := msg.FromID == acc.ID

		sender := msg.From
		if isSent {
			sender = "You"
		}

		body := html.EscapeString(msg.Body)
		body = strings.ReplaceAll(body, "\n", "<br>")

		timeStr := msg.CreatedAt.Format("Jan 2, 3:04 PM")

		borderColor := "#28a745"
		if isSent {
			borderColor = "#007bff"
		}

		msgHTML := fmt.Sprintf(`
			<div class="chat-message" style="margin-bottom: 15px; padding: 12px; border-left: 3px solid %s; background: #f9f9f9; border-radius: 4px;">
				<div style="font-weight: bold; margin-bottom: 5px; color: %s;">%s</div>
				<div style="margin-bottom: 5px; line-height: 1.5;">%s</div>
				<div style="font-size: 12px; color: #999;">%s</div>
			</div>
		`, borderColor, borderColor, sender, body, timeStr)
		messageHTML = append(messageHTML, msgHTML)
	}

	content := fmt.Sprintf(`
		<div style="margin-bottom: 20px;">
			<a href="/chat?mode=messages" class="button" style="padding: 10px 20px; background: #6c757d; color: white; text-decoration: none; border-radius: 4px; display: inline-block;">← Back to Inbox</a>
			<h2 style="display: inline-block; margin-left: 20px;">Chat with %s</h2>
		</div>
		<div style="max-width: 800px; margin-bottom: 20px; background: #fff; padding: 20px; border: 1px solid #ddd; border-radius: 4px;">
			%s
		</div>
		<div style="max-width: 800px; background: #fff; padding: 20px; border: 1px solid #ddd; border-radius: 4px;">
			<form method="POST" action="/chat?mode=messages&id=%s">
				<input type="hidden" name="to" value="%s">
				<input type="hidden" name="reply_to" value="%s">
				<div style="margin-bottom: 15px;">
					<label for="body" style="display: block; margin-bottom: 5px; font-weight: bold;">Reply:</label>
					<textarea id="body" name="body" rows="4" required 
						style="width: 100%%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; font-size: 14px; box-sizing: border-box; font-family: inherit;"></textarea>
				</div>
				<button type="submit" style="padding: 12px 24px; background: #007bff; color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; font-weight: bold;">Send Reply</button>
			</form>
		</div>
	`, html.EscapeString(otherUser), strings.Join(messageHTML, "\n"), threadID, otherUserID, threadMessages[len(threadMessages)-1].ID)

	htmlContent := app.RenderHTMLForRequest("Chat Thread", "Conversation with "+otherUser, content, r)
	w.Write([]byte(htmlContent))
}
