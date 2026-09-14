package app

import (
	"html"
	"net/url"
	"strings"
	"time"

	"mu/internal/thread"
)

// ConversationList renders the same owned history in the rail and the switcher.
func ConversationList(account, current string) string {
	var b strings.Builder
	b.WriteString("<div class=\"chat-sess-list\" aria-label=\"Conversations\">")
	shown := 0
	for _, t := range thread.List(account, 0) {
		if thread.Arrived(t) {
			continue
		}
		title := t.Subject
		if title == "" {
			title = "Untitled conversation"
		}
		active := ""
		if t.ID == current {
			active = " active"
		}
		b.WriteString("<a class=\"chat-sess" + active + "\" href=\"/?session=" + url.QueryEscape(t.ID) + "\"><span class=\"chat-sess-title\">" + html.EscapeString(title) + "</span><time class=\"chat-sess-when\" datetime=\"" + t.Updated.Format(time.RFC3339) + "\">" + t.Updated.Format("2 Jan 2006") + "</time></a>")
		shown++
		if shown == 30 {
			break
		}
	}
	if shown == 0 {
		b.WriteString("<p class=\"chat-sess-empty\">No conversations yet.</p>")
	}
	b.WriteString("</div>")
	return b.String()
}
