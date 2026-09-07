package mail

import (
	"context"
	"fmt"
	"mu/internal/service"
	"sort"
	"strings"
	"time"
)

// MessageView is the message content a correspondent may read. Transport headers,
// sender IPs, spam diagnostics and attachment bytes never enter tool responses.
type MessageView struct {
	ID             string    `json:"id"`
	From           string    `json:"from"`
	To             string    `json:"to"`
	Subject        string    `json:"subject"`
	Body           string    `json:"body,omitempty"`
	Snippet        string    `json:"snippet"`
	Tag            string    `json:"tag,omitempty"`
	Created        time.Time `json:"created"`
	AttachmentName string    `json:"attachment_name,omitempty"`
	AttachmentType string    `json:"attachment_type,omitempty"`
}

func record(m *Message, full bool) MessageView {
	r := MessageView{ID: m.ID, From: m.From, To: m.To, Subject: m.Subject, Snippet: clip(stripHTMLTags(m.Body), 140), Tag: m.Tag, Created: m.CreatedAt, AttachmentName: m.AttachmentName, AttachmentType: m.AttachmentType}
	if full {
		r.Body = m.Body
	}
	return r
}

type ReadRequest struct {
	Offset int    `json:"offset" description:"Body character offset"`
	Limit  int    `json:"limit" description:"Body characters per page; default and maximum 2000"`
	ID     string `json:"id" description:"Message id from mail_inbox or mail_search"`
}
type ReadResponse struct {
	Total      int          `json:"total"`
	NextOffset int          `json:"next_offset,omitempty"`
	Item       *MessageView `json:"item"`
	Text       string       `json:"text"`
}

// Read returns a complete message to its authenticated correspondent.
func (Server) Read(ctx context.Context, req *ReadRequest, rsp *ReadResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read mail")
	}
	if strings.TrimSpace(req.ID) == "" {
		var page SearchResponse
		if err := messagePage(owner, "", "", 0, 10, false, &page); err != nil {
			return err
		}
		rsp.Text = page.Text
		return nil
	}
	mutex.RLock()
	defer mutex.RUnlock()
	for _, m := range messages {
		if m.ID != strings.TrimSpace(req.ID) || (m.ToID != owner && !sentBy(m, owner)) {
			continue
		}
		r := record(m, true)
		body := []rune(r.Body)
		limit := req.Limit
		if limit <= 0 || limit > 2000 {
			limit = 2000
		}
		start := min(max(req.Offset, 0), len(body))
		end := min(start+limit, len(body))
		rsp.Total = len(body)
		if end < len(body) {
			rsp.NextOffset = end
		}
		r.Body = string(body[start:end])
		rsp.Item = &r
		rsp.Text = fmt.Sprintf("%s: body characters %d–%d of %d", r.Subject, start, end, len(body))
		return nil
	}
	return fmt.Errorf("message not found")
}

// messagePage filters before paging and copies records while the store is locked.
func messagePage(owner, query, tag string, offset, limit int, search bool, rsp *SearchResponse) error {
	if owner == "" {
		return fmt.Errorf("sign in to read mail")
	}
	query, tag = strings.ToLower(strings.TrimSpace(query)), strings.ToLower(strings.TrimSpace(tag))
	mutex.RLock()
	count := len(messages)
	mutex.RUnlock()
	var indexed []*Message
	if query != "" {
		indexed = Search(owner, query, count+1)
	}
	mutex.RLock()
	candidates := messages
	if query != "" {
		candidates = indexed
	}
	var found []*Message
	for _, m := range candidates {
		if m.Spam {
			continue
		}
		if search {
			if m.ToID != owner && !sentBy(m, owner) {
				continue
			}
		} else if m.ToID != owner || selfAddressed(m) {
			continue
		}
		if tag != "" && !strings.EqualFold(m.Tag, tag) {
			continue
		}
		cp := *m
		found = append(found, &cp)
	}
	mutex.RUnlock()
	if query == "" {
		sort.Slice(found, func(i, j int) bool {
			if found[i].CreatedAt.Equal(found[j].CreatedAt) {
				return found[i].ID < found[j].ID
			}
			return found[i].CreatedAt.After(found[j].CreatedAt)
		})
	}
	if limit <= 0 {
		limit = 10
	}
	start, end := service.PageRange(len(found), offset, limit)
	rsp.Total, rsp.Offset = len(found), start
	rsp.Items = make([]MessageView, 0, end-start)
	for _, m := range found[start:end] {
		rsp.Items = append(rsp.Items, record(m, false))
	}
	if end < len(found) {
		rsp.NextOffset = &end
	}
	rsp.Text = renderInbox(found[start:end])
	if query != "" {
		rsp.Text = fmt.Sprintf("Mail matching %q:\n", query) + renderMessages(found[start:end])
	}
	return nil
}
