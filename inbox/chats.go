package inbox

import (
	"context"
	"html"
	"net/url"
	"sort"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/group"
	"mu/internal/service"
	"mu/internal/thread"
	"mu/service/chat"
)

// Chat is a navigation projection, not a second message store.
type Chat struct {
	ID, Title, Context, URL, Preview string
	Updated                          time.Time
	Shared, Unread                   bool
}

// Chats combines personal web conversations and currently accessible group rooms.
func Chats(owner string) []Chat {
	var out []Chat
	for _, t := range thread.List(owner, 0) {
		if t.Client != thread.WebClient {
			continue
		}
		title := t.Subject
		if title == "" {
			title = "Chat with Micro"
		}
		out = append(out, Chat{ID: t.ID, Title: title, Context: "Personal", URL: "/?session=" + url.QueryEscape(t.ID), Updated: t.Updated, Unread: thread.Unread(t)})
	}
	gs, _, err := group.List(owner)
	if err == nil {
		for _, g := range gs {
			rooms, err := group.Conversations(g.ID, owner)
			if err != nil {
				continue
			}
			for _, c := range rooms {
				var messages chat.MessagesResponse
				if err := (chat.Server{}).Messages(service.WithAccount(context.Background(), owner), &chat.MessagesRequest{Room: c.ID}, &messages); err != nil {
					continue
				}
				preview, at := "", c.Created
				if len(messages.Messages) > 0 {
					last := messages.Messages[len(messages.Messages)-1]
					preview, at = plainPreview(last.Content), last.Timestamp
				}
				if at.IsZero() {
					at = c.Created
				}
				out = append(out, Chat{ID: c.ID, Title: c.Name, Context: g.Name, URL: "/chat?id=" + url.QueryEscape(c.ID), Preview: preview, Updated: at, Shared: true, Unread: g.Activity[c.ID].After(g.Seen[owner+"/"+c.ID])})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Updated.Equal(out[j].Updated) {
			return out[i].ID < out[j].ID
		}
		return out[i].Updated.After(out[j].Updated)
	})
	return out
}

func ChatRow(c Chat) string {
	unread := ""
	if c.Unread {
		unread = `<span class="unread-dot" aria-label="Unread"></span>`
	}
	return `<a class="conversation-row" href="` + html.EscapeString(c.URL) + `"><span class="conversation-title">` + unread + html.EscapeString(c.Title) + `</span><time datetime="` + c.Updated.Format(time.RFC3339) + `">` + html.EscapeString(app.TimeAgo(c.Updated)) + `</time><span class="conversation-context">` + html.EscapeString(c.Context) + `</span><span class="conversation-preview">` + html.EscapeString(c.Preview) + `</span></a>`
}

func invitations(owner, csrf string) string {
	_, pending, err := group.List(owner)
	if err != nil || len(pending) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="record-card"><h2>Group invitations</h2>`)
	for _, p := range pending {
		b.WriteString(`<div class="collection-item"><span><strong>` + html.EscapeString(p.Name) + `</strong><span class="metadata-row">Invited by @` + html.EscapeString(p.InvitedBy) + `</span></span><form class="form-actions" method="post" action="/groups">` + app.CSRFField(csrf) + `<input type="hidden" name="id" value="` + html.EscapeString(p.ID) + `"><button name="action" value="accept">Join chat</button><button name="action" value="decline">Decline</button></form></div>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}
