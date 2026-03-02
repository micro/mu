package chat

import (
	"fmt"
	"html"
	"net/http"
	"sort"
	"strings"

	"mu/ai"
	"mu/app"
	"mu/auth"
	"mu/data"
	"mu/wallet"
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
			// Check wallet quota for AI DM
			canProceed, _, cost, _ := wallet.CheckQuota(acc.ID, wallet.OpChatQuery)
			if !canProceed {
				content := wallet.QuotaExceededPage(wallet.OpChatQuery, cost)
				htmlContent := app.RenderHTMLForRequest("Quota Exceeded", "Daily limit reached", content, r)
				w.Write([]byte(htmlContent))
				return
			}

			// Store user's message to micro
			threadID, err := SendChatMessage(acc.Name, acc.ID, "micro", "micro", body, replyTo)
			if err != nil {
				http.Error(w, "Failed to send message", http.StatusInternalServerError)
				return
			}

			// Find the message ID for the reply_to reference
			chatMessagesMutex.RLock()
			var userMsgID string
			for i := len(chatMessages) - 1; i >= 0; i-- {
				m := chatMessages[i]
				if m.FromID == acc.ID && m.ToID == "micro" && m.ThreadID == threadID {
					userMsgID = m.ID
					break
				}
			}
			chatMessagesMutex.RUnlock()

			// Generate AI response synchronously
			ragEntries := data.Search(body, 5)
			var ragContext []string
			for _, entry := range ragEntries {
				contextStr := fmt.Sprintf("%s: %s", entry.Title, entry.Content)
				if len(contextStr) > 600 {
					contextStr = contextStr[:600] + "..."
				}
				ragContext = append(ragContext, contextStr)
			}

			aiResp, err := askLLM(&ai.Prompt{
				Rag:      ragContext,
				Question: body,
			})

			if err == nil && aiResp != "" {
				wallet.ConsumeQuota(acc.ID, wallet.OpChatQuery)
				if _, storeErr := SendChatMessage("micro", "micro", acc.Name, acc.ID, aiResp, userMsgID); storeErr != nil {
					app.Log("chat", "Failed to store micro DM reply for %s: %v", acc.Name, storeErr)
				}
			}

			// Redirect to the DM thread
			http.Redirect(w, r, "/chat?mode=messages&id="+threadID, http.StatusSeeOther)
			return
		}

		// Look up recipient
		toAcc, err := auth.GetAccountByName(to)
		if err != nil {
			http.Error(w, "Recipient not found", http.StatusNotFound)
			return
		}

		// Send message
		threadID, err := SendChatMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, body, replyTo)
		if err != nil {
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
			return
		}

		// Redirect to thread
		urlThreadID := r.URL.Query().Get("id")
		if urlThreadID != "" {
			http.Redirect(w, r, "/chat?mode=messages&id="+urlThreadID, http.StatusSeeOther)
		} else if replyTo != "" {
			chatMessagesMutex.RLock()
			parentMsg := getChatMessageUnlocked(replyTo)
			chatMessagesMutex.RUnlock()
			if parentMsg != nil {
				http.Redirect(w, r, "/chat?mode=messages&id="+parentMsg.ThreadID, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/chat?mode=messages&id="+threadID, http.StatusSeeOther)
			}
		} else {
			// New message - redirect to the new thread
			http.Redirect(w, r, "/chat?mode=messages&id="+threadID, http.StatusSeeOther)
		}
		return
	}

	// Handle GET - show inbox or thread
	msgID := r.URL.Query().Get("id")
	compose := r.URL.Query().Get("compose")

	if msgID != "" {
		// Show thread
		renderDMThread(w, r, msgID, acc)
		return
	}

	if compose != "" {
		// Show compose form
		renderDMCompose(w, r, acc)
		return
	}

	// Show inbox
	renderDMInbox(w, r, acc)
}

// renderDMInbox renders the DM inbox with conversations
func renderDMInbox(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
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

		unreadDot := ""
		if thread.HasUnread {
			unreadDot = `<span class="unread-dot">● </span>`
		}

		preview := latest.Body
		if len(preview) > 80 {
			preview = preview[:80] + "..."
		}
		preview = html.EscapeString(preview)

		timeAgo := app.TimeAgo(latest.CreatedAt)

		item := fmt.Sprintf(`
			<div class="thread-preview card" onclick="window.location.href='/chat?mode=messages&id=%s'">
				<div class="mail-thread-item">
					<strong class="mail-thread-subject">%s%s</strong>
				</div>
				<div class="mail-thread-row">
					<div class="mail-thread-preview">%s</div>
					<span class="mail-thread-time">%s</span>
				</div>
			</div>
		`, root.ID, unreadDot, html.EscapeString(otherUser), preview, timeAgo)
		items = append(items, item)
	}

	var content string
	if len(items) == 0 {
		content = app.Page(app.PageOpts{
			Action:  "/chat?mode=messages&compose=true",
			Label:   "+ New DM",
			Content: `<p class="empty">No conversations yet. <a href="/chat?mode=messages&compose=true">Start a new DM</a> or try messaging <a href="/chat?mode=messages&compose=true&to=micro">@micro</a> (AI assistant)</p>`,
		})
	} else {
		content = app.Page(app.PageOpts{
			Action:  "/chat?mode=messages&compose=true",
			Label:   "+ New DM",
			Content: `<div class="card-list">` + strings.Join(items, "\n") + `</div>`,
		})
	}

	htmlContent := app.RenderHTMLForRequest("DMs", "Direct messages", content, r)
	w.Write([]byte(htmlContent))
}

// renderDMCompose renders the new DM compose form
func renderDMCompose(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	to := r.URL.Query().Get("to")

	content := fmt.Sprintf(`
		<div class="page-action">
			<a href="/chat?mode=messages" class="btn">← Back</a>
		</div>
		<div class="thread-message">
			<form method="POST" action="/chat?mode=messages">
				<div style="margin-bottom:12px;">
					<label for="to" style="display:block;margin-bottom:4px;font-weight:600;">To</label>
					<input type="text" id="to" name="to" value="%s" placeholder="Username (or 'micro' for AI)" required>
					<small class="card-meta">Tip: type 'micro' to chat with the AI assistant</small>
				</div>
				<div style="margin-bottom:12px;">
					<label for="body" style="display:block;margin-bottom:4px;font-weight:600;">Message</label>
					<textarea id="body" name="body" rows="6" required style="width:100%%;padding:10px;border:1px solid #e0e0e0;border-radius:6px;font-size:14px;box-sizing:border-box;font-family:inherit;resize:vertical;"></textarea>
				</div>
				<button type="submit">Send</button>
			</form>
		</div>
	`, html.EscapeString(to))

	htmlContent := app.RenderHTMLForRequest("New DM", "Compose direct message", content, r)
	w.Write([]byte(htmlContent))
}

// renderDMThread renders a DM conversation thread
func renderDMThread(w http.ResponseWriter, r *http.Request, threadID string, acc *auth.Account) {
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
	if firstMsg.FromID == acc.ID {
		otherUser = firstMsg.To
	} else {
		otherUser = firstMsg.From
	}

	chatMessagesMutex.RUnlock()

	// Mark all unread messages as read
	for _, msg := range threadMessages {
		if msg.ToID == acc.ID && !msg.Read {
			MarkChatMessageAsRead(msg.ID, acc.ID)
		}
	}

	// Render messages using the same card style as the rest of the app
	var messageHTML []string
	for _, msg := range threadMessages {
		isSent := msg.FromID == acc.ID

		sender := html.EscapeString(msg.From)
		if isSent {
			sender = "You"
		}

		body := html.EscapeString(msg.Body)
		body = strings.ReplaceAll(body, "\n", "<br>")

		timeStr := msg.CreatedAt.Format("Jan 2, 3:04 PM")

		alignStyle := ""
		if isSent {
			alignStyle = " style=\"margin-left:auto;\""
		}

		msgHTML := fmt.Sprintf(`
			<div class="thread-message"%s>
				<div class="thread-message-header">
					<div class="thread-message-header-text">
						<span class="thread-message-author">%s</span>
					</div>
					<span class="thread-message-time">%s</span>
				</div>
				<div class="thread-message-body">%s</div>
			</div>
		`, alignStyle, sender, timeStr, body)
		messageHTML = append(messageHTML, msgHTML)
	}

	lastMsgID := threadMessages[len(threadMessages)-1].ID

	content := fmt.Sprintf(`
		<div class="page-action">
			<a href="/chat?mode=messages" class="btn">← DMs</a>
		</div>
		<div style="margin-bottom:16px;">%s</div>
		<div class="thread-message">
			<form method="POST" action="/chat?mode=messages&id=%s">
				<input type="hidden" name="to" value="%s">
				<input type="hidden" name="reply_to" value="%s">
				<div style="margin-bottom:10px;">
					<textarea id="body" name="body" rows="3" required placeholder="Reply to %s..." style="width:100%%;padding:10px;border:1px solid #e0e0e0;border-radius:6px;font-size:14px;box-sizing:border-box;font-family:inherit;resize:vertical;"></textarea>
				</div>
				<button type="submit">Send</button>
			</form>
		</div>
	`, strings.Join(messageHTML, "\n"), threadID, html.EscapeString(otherUser), lastMsgID, html.EscapeString(otherUser))

	htmlContent := app.RenderHTMLForRequest("DM: "+otherUser, "Conversation with "+otherUser, content, r)
	w.Write([]byte(htmlContent))
}

