package sms

import (
	"context"
	"errors"
	"fmt"
	"mu/internal/service"
	"mu/internal/userdb"
)

func (Server) Source(ctx context.Context, req *service.SourceRequest, rsp *service.SourceResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read messages")
	}
	r, err := userdb.Get(ns, owner, msgs, req.ID)
	if errors.Is(err, userdb.ErrNotFound) || errors.Is(err, userdb.ErrForbidden) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.Owner != owner {
		return nil
	}
	list := messagesFrom([]userdb.Record{*r})
	if len(list) != 1 {
		return nil
	}
	m := list[0]
	rsp.Item = &service.SourceMessage{ID: m.ID, Ref: m.ID, Conversation: m.Number, Text: m.Text, From: m.Number, At: m.At, Channel: m.Channel, Direction: m.Direction, Facts: map[string]interface{}{"verified_owner": r.Data["verified_owner"]}}
	return nil
}

func (Server) Sources(ctx context.Context, req *service.SourcesRequest, rsp *service.SourcesResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read messages")
	}
	// Source IDs are a projection input; filter ownership before pagination.
	records, more, err := userdb.Page(ns, owner, msgs, "mine", req.Offset, req.Limit)
	if err != nil {
		return err
	}
	for _, r := range records {
		rsp.IDs = append(rsp.IDs, r.ID)
	}
	if more {
		rsp.Next = req.Offset + len(records)
	}
	return nil
}
