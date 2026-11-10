package contacts

import (
	"google.golang.org/protobuf/proto"
	"mu/internal/api"
	"mu/internal/mucp"
	"mu/internal/service"
	pb "mu/proto/service/contacts/v1"
	"net/http"
)

// MUCPOperations adapts typed external contracts to the existing metered gateway.
func MUCPOperations() []mucp.Operation {
	return []mucp.Operation{
		{Service: "contacts", Endpoint: "Contacts.List", Request: func() proto.Message { return &pb.ListRequest{} }, Handle: contactsCall},
		{Service: "contacts", Endpoint: "Contacts.Find", Request: func() proto.Message { return &pb.FindRequest{} }, Handle: contactsCall},
		{Service: "contacts", Endpoint: "Contacts.Add", Writes: true, Request: func() proto.Message { return &pb.AddRequest{} }, Handle: contactsCall},
		{Service: "contacts", Endpoint: "Contacts.Delete", Writes: true, Request: func() proto.Message { return &pb.DeleteRequest{} }, Handle: contactsCall},
	}
}
func contactMessage(c *Contact) *pb.Contact {
	if c == nil {
		return nil
	}
	return &pb.Contact{Id: c.ID, Name: c.Name, Email: c.Email, Phone: c.Phone, Note: c.Note}
}
func contactsCall(_ http.ResponseWriter, r *http.Request, owner, _ string, q proto.Message) (proto.Message, error) {
	ctx := service.WithAccount(r.Context(), owner)
	switch q := q.(type) {
	case *pb.ListRequest:
		var rsp ListResponse
		if err := service.Call(ctx, "contacts", "Server.List", &ListRequest{}, &rsp); err != nil {
			return nil, err
		}
		items := []*pb.Contact{}
		for _, c := range rsp.Contacts {
			items = append(items, contactMessage(c))
		}
		page, next, err := mucp.Page(items, q.PageSize, q.PageToken, owner+"/contacts/list")
		return &pb.ListResponse{Contacts: page, NextPageToken: next}, err
	case *pb.FindRequest:
		if q.Query == "" {
			return nil, api.Fail(400, "invalid_arguments", "A search query is required")
		}
		var rsp FindResponse
		if err := service.Call(ctx, "contacts", "Server.Find", &FindRequest{Query: q.Query}, &rsp); err != nil {
			return nil, err
		}
		out := &pb.FindResponse{Text: rsp.Text}
		for _, c := range rsp.Contacts {
			out.Contacts = append(out.Contacts, contactMessage(c))
		}
		return out, nil
	case *pb.AddRequest:
		if q.Name == "" {
			return nil, api.Fail(400, "invalid_arguments", "A name is required")
		}
		var rsp AddResponse
		if err := service.Call(ctx, "contacts", "Server.Add", &AddRequest{Name: q.Name, Email: q.Email, Phone: q.Phone, Note: q.Note}, &rsp); err != nil {
			return nil, err
		}
		return contactMessage(rsp.Contact), nil
	case *pb.DeleteRequest:
		if q.Id == "" {
			return nil, api.Fail(400, "invalid_arguments", "A contact ID is required")
		}
		var rsp DeleteResponse
		if err := service.Call(ctx, "contacts", "Server.Delete", &DeleteRequest{ID: q.Id}, &rsp); err != nil {
			return nil, err
		}
		return &pb.DeleteResponse{Id: q.Id}, nil
	}
	return nil, api.Fail(400, "invalid_arguments", "Unknown contacts request")
}
