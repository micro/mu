package mail

import (
	"fmt"
	stdhtml "html"
	"net/url"
	"strings"

	"mu/internal/app"
)

// renderThreadPreview renders a thread preview showing the latest message but linking to root
func renderThreadPreview(rootID string, latestMsg *Message, viewerID string, hasUnread bool) string {
	unreadIndicator := ""
	rowClass := "thread-preview"
	if hasUnread {
		rowClass += " mail-unread"
		unreadIndicator = `<span class="unread-dot" aria-label="Unread"></span>`
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

	return previewRow(rootID, rowClass, unreadIndicator+stdhtml.EscapeString(fromDisplay), latestMsg.Subject, bodyPreview, relativeTime)
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

	return previewRow(rootID, "thread-preview", "to "+stdhtml.EscapeString(toDisplay), latestMsg.Subject, bodyPreview, relativeTime)
}

func previewRow(id, classes, sender, subject, preview, when string) string {
	return fmt.Sprintf(`<div class="list-row compact-row %s"><a class="list-link grow page-stack compact-stack" href="/mail?id=%s"><span class="mail-thread-subject truncate">%s</span><span class="text-sm truncate">%s</span><span class="text-sm text-muted truncate">%s</span></a><form class="form-action" method="POST" action="/mail" onsubmit="return confirm('Delete this conversation?')"><input type="hidden" name="action" value="delete_thread"><input type="hidden" name="msg_id" value="%s"><button type="submit" aria-label="Delete conversation">×</button></form><span class="metadata-time">%s</span></div>`, classes, url.QueryEscape(id), sender, stdhtml.EscapeString(decodeMIMEHeader(subject)), stdhtml.EscapeString(preview), stdhtml.EscapeString(id), when)
}
