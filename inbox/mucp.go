package inbox

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/api"
	"mu/internal/mucp"
	"mu/internal/thread"
	pb "mu/proto/assistant/v1"
	"net/http"
)

func MUCPOperations() []mucp.Operation {
	return []mucp.Operation{
		{Service: "assistant", Endpoint: "Inbox.List", Capability: "inbox", Request: func() proto.Message { return &pb.ListInboxRequest{} }, Handle: inboxCall},
		{Service: "assistant", Endpoint: "Inbox.Read", Capability: "inbox", Request: func() proto.Message { return &pb.ReadInboxRequest{} }, Handle: inboxCall},
		{Service: "assistant", Endpoint: "Inbox.MarkRead", Capability: "inbox", Writes: true, Request: func() proto.Message { return &pb.MarkInboxReadRequest{} }, Handle: inboxCall},
	}
}
func inboxItem(t thread.Thread) *pb.InboxItem {
	return &pb.InboxItem{Id: t.ID, Subject: t.Subject, Channel: t.Client, Unread: t.Updated.After(t.Seen), Held: t.Held, Updated: timestamppb.New(t.Updated), Actions: []string{"Inbox.Read", "Inbox.MarkRead"}}
}

// ClientItems projects account-owned inbox records without internal routing or credentials.
func ClientItems(owner string, limit int) []*pb.InboxItem {
	out := []*pb.InboxItem{}
	for _, t := range thread.List(owner, limit) {
		out = append(out, inboxItem(t))
	}
	return out
}
func inboxCall(_ http.ResponseWriter, _ *http.Request, owner, _ string, q proto.Message) (proto.Message, error) {
	switch q := q.(type) {
	case *pb.ListInboxRequest:
		items := ClientItems(owner, 0)
		if q.UnreadOnly {
			filtered := []*pb.InboxItem{}
			for _, item := range items {
				if item.Unread {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}
		page, next, err := mucp.Page(items, q.PageSize, q.PageToken, fmt.Sprintf("%s/inbox/list/%t", owner, q.UnreadOnly))
		return &pb.ListInboxResponse{Items: page, NextPageToken: next}, err
	case *pb.ReadInboxRequest:
		t := thread.Get(owner, q.Id)
		if t == nil {
			return nil, api.Fail(404, "not_found", "Inbox item not found")
		}
		messages := thread.Messages(owner, t.ID, 0)
		page, next, err := mucp.Page(messages, q.PageSize, q.PageToken, owner+"/inbox/read/"+t.ID)
		out := &pb.ReadInboxResponse{Item: inboxItem(*t), NextPageToken: next}
		for _, m := range page {
			role := pb.Role_ROLE_PERSON
			if m.Role == thread.RoleAgent {
				role = pb.Role_ROLE_AGENT
			} else if m.Role != thread.RolePerson {
				continue
			}
			out.Messages = append(out.Messages, &pb.Message{Id: m.ID, Role: role, Text: m.Text, At: timestamppb.New(m.At)})
		}
		return out, err
	case *pb.MarkInboxReadRequest:
		if thread.Get(owner, q.Id) == nil {
			return nil, api.Fail(404, "not_found", "Inbox item not found")
		}
		if q.Unread {
			thread.MarkUnread(owner, q.Id)
		} else {
			thread.MarkSeen(owner, q.Id)
		}
		if err := thread.Flush(); err != nil {
			return nil, err
		}
		t := thread.Get(owner, q.Id)
		if t == nil {
			return nil, api.Fail(404, "not_found", "Inbox item not found")
		}
		return inboxItem(*t), nil
	}
	return nil, api.Fail(400, "invalid_arguments", "Unknown inbox request")
}
