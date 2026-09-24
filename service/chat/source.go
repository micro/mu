package chat

import (
	"context"
	"fmt"
	"mu/internal/service"
)

func (Server) Source(ctx context.Context, req *service.SourceRequest, rsp *service.SourceResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read messages")
	}
	saidMu.RLock()
	defer saidMu.RUnlock()
	for _, m := range said[owner] {
		if m.ID == req.ID {
			rsp.Item = &service.SourceMessage{ID: m.ID, Ref: m.ID, Conversation: m.Conv, Text: m.Text, From: m.From, To: m.To, At: m.At, Facts: m.Facts}
			if sender := accountFor(m.From); m.From == owner || (sender != nil && sender.ID == owner) {
				rsp.Item.Direction = "out"
			}
			return nil
		}
	}
	return nil
}

func (Server) Sources(ctx context.Context, req *service.SourcesRequest, rsp *service.SourcesResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read messages")
	}
	saidMu.RLock()
	defer saidMu.RUnlock()
	all := said[owner]
	start, end := service.PageRange(len(all), req.Offset, req.Limit)
	for _, m := range all[start:end] {
		rsp.IDs = append(rsp.IDs, m.ID)
	}
	if end < len(all) {
		rsp.Next = end
	}
	return nil
}
