package sms

import (
	"html"
	"net/url"
	"strings"

	"mu/internal/app"
	"mu/internal/contacts"
	"mu/internal/userdb"
)

// conversationList shows one latest-message preview per number and channel.
// IDs come from stored messages, so URLs contain neither numbers nor text.
func conversationList(who string, history []Message) string {
	if len(history) == 0 {
		return `<p class="text-muted">No conversations yet.</p>`
	}
	names := map[string]string{}
	for _, c := range contacts.List(who) {
		if n := e164(c.Phone); n != "" && c.Name != "" {
			names[n] = c.Name
		}
	}
	seen := map[string]bool{}
	var b strings.Builder
	b.WriteString(`<nav class="sms-conversations" aria-label="Conversations">`)
	for _, m := range history {
		key := m.Number + ":" + m.Channel
		if seen[key] {
			continue
		}
		seen[key] = true
		name := names[m.Number]
		if name == "" {
			name = m.Number
		}
		preview := m.Text
		if m.Direction == "out" {
			preview = "You: " + preview
		}
		b.WriteString(`<a class="sms-conversation" href="/sms?id=` + url.QueryEscape(m.ID) + `"><div class="sms-conversation-head"><strong>` + html.EscapeString(name) + `</strong>` + app.Pill(Channel(m.Channel).Label()) + `<span class="text-muted text-sm">` + html.EscapeString(app.TimeAgo(m.At)) + `</span></div>`)
		if name != m.Number {
			b.WriteString(`<span class="text-muted text-sm">` + html.EscapeString(m.Number) + `</span>`)
		}
		b.WriteString(`<span class="sms-preview">` + html.EscapeString(preview) + `</span></a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

const conversationsCSS = `<style>
.sms-conversations{display:flex;flex-direction:column;min-width:0}
.sms-conversation{display:flex;flex-direction:column;gap:var(--space-control);padding:var(--space-field) 0;border-bottom:1px solid var(--border-color,#ddd);color:inherit;text-decoration:none;min-width:0}
.sms-conversation:hover{background:var(--hover-background,#f5f5f5);text-decoration:none}
.sms-conversation-head{display:flex;align-items:center;gap:var(--space-control);flex-wrap:wrap;min-width:0}
.sms-conversation-head strong{overflow-wrap:anywhere}
.sms-preview{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--text-secondary,#555)}
</style>`

func recentConversations(who string) ([]Message, error) {
	recs, err := userdb.LatestBy(ns, who, msgs, []string{"number", "channel"}, "at", 200)
	if err != nil {
		return nil, err
	}
	return messagesFrom(recs), nil
}
