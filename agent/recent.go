package agent

import (
	"html"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/thread"
)

// RecentConversations is a small, account-scoped preview of chats started here.
func RecentConversations(accountID string) string {
	if accountID == "" {
		return ""
	}
	var b strings.Builder
	for i, t := range chatThreads(accountID, "", false) {
		if i == 3 {
			break
		}
		title := t.Subject
		if title == "" {
			title = "Conversation"
		}
		b.WriteString(`<a class="peek-row" href="/assistant?session=` + url.QueryEscape(t.ID) + `"><span class="peek-main"><span class="peek-title">` + html.EscapeString(title) + `</span>`)
		messages := thread.Messages(accountID, t.ID, 1)
		if len(messages) > 0 {
			text := []rune(messages[len(messages)-1].Text)
			if len(text) > 140 {
				text = append(text[:140], '…')
			}
			b.WriteString(`<span class="peek-snippet">` + html.EscapeString(string(text)) + `</span>`)
		}
		b.WriteString(`</span></a>`)
	}
	if b.Len() == 0 {
		return ""
	}
	return app.PreviewCard("home-recent", "Recent conversations", "/assistant", b.String())
}
