package app

import "html"

// WorkState is the user-facing label shared by work details and conversation activity.
func WorkState(status string) string {
	switch status {
	case "todo":
		return "Queued"
	case "doing":
		return "Running"
	case "blocked":
		return "Needs input"
	case "failed":
		return "Failed"
	case "done":
		return "Done"
	case "canceled":
		return "Stopped"
	}
	return status
}

func WorkBadge(status string) string {
	if status == "" {
		return ""
	}
	return `<span class="status-badge status-` + html.EscapeString(status) + `">` + html.EscapeString(WorkState(status)) + `</span>`
}

// ConversationActivity is filled by the authenticated agent projection. It holds
// no message content and never replaces the composer or transcript.
func ConversationActivity(source string) string {
	return `<div data-conversation-activity="` + html.EscapeString(source) + `" aria-live="polite"></div>`
}
