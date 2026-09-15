package mail

import (
	"mu/internal/app"
	"mu/internal/auth"
	"net/http"
	"sort"
)

// clientPage supplies the mailbox UI without exposing transport credentials or
// raw attachment bytes. Every selected message is checked against its reader.
func clientPage(w http.ResponseWriter, r *http.Request, acc *auth.Account) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, view := r.URL.Query().Get("id"), r.URL.Query().Get("view")
	if view == "outbox" {
		outboxPage(w, r, acc.ID)
		return
	}
	mutex.RLock()
	selected := []*Message{}
	if id != "" {
		var found *Message
		for _, m := range messages {
			if m.ID == id && (m.ToID == acc.ID || sentBy(m, acc.ID)) {
				found = m
				break
			}
		}
		if found == nil {
			mutex.RUnlock()
			app.RespondError(w, 404, "Message not found")
			return
		}
		threadID := found.ThreadID
		if threadID == "" {
			threadID = computeThreadID(found)
		}
		if box := inboxes[acc.ID]; box != nil && box.Threads[threadID] != nil {
			selected = append(selected, box.Threads[threadID].Messages...)
		} else {
			selected = append(selected, found)
		}
	} else if view == "filtered" {
		for _, m := range messages {
			if m.ToID == acc.ID && m.Spam {
				selected = append(selected, m)
			}
		}
	} else if box := inboxes[acc.ID]; box != nil {
		for _, t := range box.Threads {
			include := view != "sent" && t.Latest.ToID == acc.ID
			if view == "sent" {
				for _, m := range t.Messages {
					if sentBy(m, acc.ID) {
						include = true
						break
					}
				}
			}
			if include {
				selected = append(selected, t.Latest)
			}
		}
	}
	// Copy under the lock: marking read and concurrent delivery may update records.
	copies := make([]Message, 0, len(selected))
	for _, m := range selected {
		if m.ToID == acc.ID || sentBy(m, acc.ID) {
			copies = append(copies, *m)
		}
	}
	mutex.RUnlock()
	sort.Slice(copies, func(i, j int) bool {
		if id != "" {
			return copies[i].CreatedAt.Before(copies[j].CreatedAt)
		}
		return copies[i].CreatedAt.After(copies[j].CreatedAt)
	})
	items := []map[string]any{}
	for i := range copies {
		m := &copies[i]
		item := map[string]any{"id": m.ID, "from": m.From, "to": m.To, "subject": decodeMIMEHeader(m.Subject), "created": m.CreatedAt, "snippet": record(m, false).Snippet, "read": m.Read, "reply_to": m.ID, "recipient": m.FromID, "attachment_name": m.AttachmentName, "spam_reasons": m.SpamReasons}
		if id != "" {
			item["html"] = Rendered(m)
			if m.ToID == acc.ID && !m.Read {
				MarkAsRead(m.ID, acc.ID)
			}
		}
		items = append(items, item)
	}
	app.RespondJSON(w, map[string]any{"items": items, "address": AliasFor(acc.Name, "")})
}
