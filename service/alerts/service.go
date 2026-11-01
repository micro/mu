// Package alerts exposes private and shared attention records. It does not
// detect anomalies or choose notification channels.
package alerts

import (
	"context"
	"errors"
	"strings"

	store "mu/internal/alerts"
	"mu/internal/app"
	"mu/internal/group"
	"mu/internal/service"
)

type Server struct{}
type ListRequest struct {
	Group  string `json:"group,omitempty" description:"Group id; omit for your personal alerts"`
	State  string `json:"state,omitempty" description:"open, acknowledged, resolved, dismissed; empty for all"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty" description:"Default 20, maximum 100"`
}
type ListResponse struct {
	Items []store.Item `json:"items"`
	Total int          `json:"total"`
}
type RaiseRequest struct {
	Group  string `json:"group,omitempty"`
	Key    string `json:"key" required:"true" description:"Stable condition key; repeated calls update one alert"`
	Title  string `json:"title" required:"true"`
	Detail string `json:"detail" description:"What changed, why it matters and what to do"`
	URL    string `json:"url,omitempty" description:"Optional local or https action link"`
}
type ItemResponse struct {
	Item    store.Item `json:"item"`
	Created bool       `json:"created"`
}
type UpdateRequest struct {
	Group string `json:"group,omitempty"`
	ID    string `json:"id" required:"true"`
	State string `json:"state" required:"true" description:"acknowledged, resolved or dismissed"`
}
type Response struct {
	Text string `json:"text"`
}

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
	rsp.Total = len(items)
	offset := max(0, min(req.Offset, len(items)))
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rsp.Items = items[offset:min(len(items), offset+limit)]
	return nil
}
func (Server) Raise(ctx context.Context, req *RaiseRequest, rsp *ItemResponse) error {
	o, err := owner(service.AccountFrom(ctx), req.Group, true)
	if err != nil {
		return err
	}
	rsp.Item, rsp.Created, err = store.Raise(o, store.Item{Source: "custom", Key: req.Key, Title: req.Title, Detail: req.Detail, URL: req.URL})
	return err
}
func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *Response) error {
	o, err := owner(service.AccountFrom(ctx), req.Group, true)
	if err != nil {
		return err
	}
	if err = store.Update(o, req.ID, req.State); err != nil {
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
