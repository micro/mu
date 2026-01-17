package mail

import (
	"fmt"
	"strings"

	"mu/app"
)

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
	`, rootID, rootID, unreadIndicator, fromDisplay, decodeMIMEHeader(latestMsg.Subject), bodyPreview, relativeTime)

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
	`, rootID, rootID, decodeMIMEHeader(latestMsg.Subject), toDisplay, bodyPreview, relativeTime)

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
	</div>`, msg.ID, unreadIndicator, decodeMIMEHeader(msg.Subject), bodyPreview, app.TimeAgo(msg.CreatedAt), fromDisplay)
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
	</div>`, msg.ID, decodeMIMEHeader(msg.Subject), bodyPreview, app.TimeAgo(msg.CreatedAt), toDisplay)
}

// renderSentMessageInThread renders a sent message as part of a thread (same styling as renderSentMessage)
func renderSentMessageInThread(msg *Message) string {
	return renderSentMessage(msg)
}
