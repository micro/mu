package mail

import (
	"context"
	"fmt"
	"mu/internal/service"
)

// Source resolves only the caller's own stored correspondence.
func (Server) Source(ctx context.Context, req *service.SourceRequest, rsp *service.SourceResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read mail")
	}
	mutex.RLock()
	original := byID[req.ID]
	if original == nil || original.Spam || (original.ToID != owner && !sentBy(original, owner)) {
		mutex.RUnlock()
		return nil
	}
	copied := *original
	m := &copied
	conv := m.MessageID
	if root := byID[m.ThreadID]; root != nil && root.MessageID != "" {
		conv = root.MessageID
	}
	mutex.RUnlock()
	text := stripHTMLTags(m.Body)
	if text == "" && m.AttachmentName != "" {
		text = "(attached: " + m.AttachmentName + ")"
	}
	if conv == "" {
		conv = m.ThreadID
	}
	rsp.Item = &service.SourceMessage{ID: m.ID, Ref: m.MessageID, Conversation: conv, Subject: m.Subject, Text: text, HTML: Rendered(m), From: m.FromID, To: deliveredTo(owner, m.Tag), At: m.CreatedAt}
	return nil
}

func (Server) Sources(ctx context.Context, req *service.SourcesRequest, rsp *service.SourcesResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read mail")
	}
	mutex.RLock()
	defer mutex.RUnlock()
	var ids []string
	for _, m := range messages {
		if m.ToID == owner && !m.Spam {
			ids = append(ids, m.ID)
		}
	}
	start, end := service.PageRange(len(ids), req.Offset, req.Limit)
	rsp.IDs = ids[start:end]
	if end < len(ids) {
		rsp.Next = end
	}
	return nil
}
