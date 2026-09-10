package mail

import (
	"context"
	"fmt"
	"mu/internal/service"
)

type MarkReadRequest struct {
	IDs []string `json:"ids" description:"Message IDs to mark read; use all instead to clear the inbox unread count"`
	All bool     `json:"all" description:"Explicitly mark all current inbox mail as read"`
}

type MarkReadResponse struct {
	Updated int    `json:"updated"`
	Text    string `json:"text"`
}

// MarkRead changes unread state without claiming to have reviewed the content.
func (Server) MarkRead(ctx context.Context, req *MarkReadRequest, rsp *MarkReadResponse) error {
	count, err := markRead(service.AccountFrom(ctx), req.IDs, req.All)
	if err != nil {
		return err
	}
	rsp.Updated = count
	rsp.Text = fmt.Sprintf("Marked %d mail messages as read. This only changes their read status; it does not review or triage their contents.", count)
	return nil
}

func markRead(owner string, ids []string, all bool) (int, error) {
	if owner == "" {
		return 0, fmt.Errorf("sign in to mark mail read")
	}
	if all == (len(ids) > 0) {
		return 0, fmt.Errorf("provide message ids or all=true, not both")
	}
	mutex.Lock()
	defer mutex.Unlock()
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var changed []*Message
	for _, m := range messages {
		if m.ToID != owner || m.Spam {
			continue
		}
		if all || wanted[m.ID] {
			delete(wanted, m.ID)
			if !m.Read {
				changed = append(changed, m)
			}
		}
	}
	if len(wanted) > 0 {
		return 0, fmt.Errorf("message not found in your inbox")
	}
	if len(changed) == 0 {
		return 0, nil
	}
	for _, m := range changed {
		m.Read = true
	}
	if err := save(); err != nil {
		for _, m := range changed {
			m.Read = false
		}
		return 0, err
	}
	if inbox := inboxes[owner]; inbox != nil {
		inbox.UnreadCount = 0
		for _, t := range inbox.Threads {
			t.HasUnread = false
			for _, m := range t.Messages {
				if m.ToID == owner && !m.Read {
					t.HasUnread = true
					inbox.UnreadCount++
				}
			}
		}
	}
	return len(changed), nil
}
