// Package alerts exposes private and shared attention records. It does not
// detect anomalies or choose notification channels.
package alerts

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"
	store "mu/internal/alerts"
	"mu/internal/app"
	"mu/internal/group"
	"mu/internal/service"
	pb "mu/proto/service/alerts/v1"
)

type Server struct{}
type ListRequest = pb.ListRequest
type ListResponse = pb.ListResponse
type RaiseRequest = pb.RaiseRequest
type ItemResponse = pb.ItemResponse
type UpdateRequest = pb.UpdateRequest
type Response = pb.Response

// Personal tools cannot select another account. Shared reads require current
// membership; changing shared attention state requires a group administrator.
func owner(actor, id string, write bool) (string, error) {
	if actor == "" || strings.HasPrefix(actor, "group:") {
		return "", errors.New("a signed-in account is required")
	}
	if id == "" {
		return actor, nil
	}
	if !group.Member(id, actor) || (write && !group.Admin(id, actor)) {
		return "", errors.New("group access required")
	}
	return group.Scope(id), nil
}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	o, err := owner(service.AccountFrom(ctx), req.Group, false)
	if err != nil {
		return err
	}
	if req.State != "" && req.State != "open" && req.State != "acknowledged" && req.State != "resolved" && req.State != "dismissed" {
		return errors.New("invalid state")
	}
	all, err := store.List(o)
	if err != nil {
		return err
	}
	items := []store.Item{}
	for _, i := range all {
		if req.State == "" || i.State == req.State {
			items = append(items, i)
		}
	}
	rsp.Total = int64(len(items))
	offset := max(0, min(req.Offset, int64(len(items))))
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	for _, item := range items[offset:min(int64(len(items)), offset+limit)] {
		rsp.Items = append(rsp.Items, itemMessage(item))
	}
	return nil
}
func (Server) Raise(ctx context.Context, req *RaiseRequest, rsp *ItemResponse) error {
	o, err := owner(service.AccountFrom(ctx), req.Group, true)
	if err != nil {
		return err
	}
	item, created, err := store.Raise(o, store.Item{Source: "custom", Key: req.Key, Title: req.Title, Detail: req.Detail, URL: req.Url})
	rsp.Item, rsp.Created = itemMessage(item), created
	return err
}
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *Response) error {
	o, err := owner(service.AccountFrom(ctx), req.Group, true)
	if err != nil {
		return err
	}
	if err = store.Update(o, req.Id, req.State); err != nil {
		return err
	}
	rsp.Text = "Alert updated."
	return nil
}
func DeleteAll(owner string) {
	if err := store.Clear(owner); err != nil {
		app.Log("alerts", "account deletion failed: %v", err)
	}
}
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("alerts", "register failed: %v", err)
	}
}

var Spec = service.Spec{Name: "alerts", Description: "Track conditions that need attention", Page: "/alerts", Icon: "notify.svg", Scoped: true, Handler: new(Server), Endpoints: map[string]service.Endpoint{
	"List":   {Doc: "List your alerts or a group's alerts. Requires current group membership."},
	"Raise":  {Writes: true, Doc: "Record or update a condition under a stable key. No notification is sent. Group administrators may raise shared alerts."},
	"Update": {Writes: true, Destructive: true, Doc: "Acknowledge, resolve or dismiss an alert. Group administrators may update shared alerts. Resolving a custom alert allows the next occurrence to reopen it."},
}}

func itemMessage(i store.Item) *pb.Item {
	out := &pb.Item{Id: i.ID, Source: i.Source, Key: i.Key, Title: i.Title, Detail: i.Detail, Url: i.URL, State: i.State, Active: i.Active}
	if !i.Created.IsZero() {
		out.Created = timestamppb.New(i.Created)
	}
	if !i.Updated.IsZero() {
		out.Updated = timestamppb.New(i.Updated)
	}
	return out
}
