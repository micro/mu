package chat

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/service"
	pb "mu/proto/service/chat/v1"
)

type SourceRequest = pb.ServiceSourceRequest
type SourceResponse = pb.ServiceSourceResponse
type SourcesRequest = pb.ServiceSourcesRequest
type SourcesResponse = pb.ServiceSourcesResponse

func (Server) Source(ctx context.Context, req *SourceRequest, rsp *SourceResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read messages")
	}
	saidMu.RLock()
	defer saidMu.RUnlock()
	for _, m := range said[owner] {
		if m.ID == req.Id {
			rsp.Item = &pb.ServiceSourceMessage{Id: m.ID, Ref: m.ID, Conversation: m.Conv, Text: m.Text, From: m.From, To: m.To, At: timestamppb.New(m.At)}
			rsp.Item.Facts = map[string]*structpb.Value{}
			for key, value := range m.Facts {
				v, err := structpb.NewValue(value)
				if err != nil {
					return err
				}
				rsp.Item.Facts[key] = v
			}
			if sender := accountFor(m.From); m.From == owner || (sender != nil && sender.ID == owner) {
				rsp.Item.Direction = "out"
			}
			return nil
		}
	}
	return nil
}

func (Server) Sources(ctx context.Context, req *SourcesRequest, rsp *SourcesResponse) error {
	owner := service.AccountFrom(ctx)
	if owner == "" {
		return fmt.Errorf("sign in to read messages")
	}
	saidMu.RLock()
	defer saidMu.RUnlock()
	all := said[owner]
	start, end := service.PageRange(len(all), int(req.Offset), int(req.Limit))
	for _, m := range all[start:end] {
		rsp.Ids = append(rsp.Ids, m.ID)
	}
	if end < len(all) {
		rsp.Next = int32(end)
	}
	return nil
}
