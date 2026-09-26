// Package groups exposes shared membership through the UI and service tools.
package groups

import (
	"context"
	"errors"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/group"
	"mu/internal/service"
)

type Server struct{}
type ListRequest struct{}
type ListResponse struct {
	Groups      []group.Group   `json:"groups"`
	Invitations []group.Pending `json:"invitations"`
}

func (Server) List(ctx context.Context, _ *ListRequest, rsp *ListResponse) error {
	var err error
	rsp.Groups, rsp.Invitations, err = group.List(service.AccountFrom(ctx))
	return err
}

type CreateRequest struct {
	Encrypted bool   `json:"encrypted,omitempty" description:"Require end-to-end encrypted XMPP clients; web chat cannot decrypt these messages"`
	Name      string `json:"name" required:"true" description:"Group name"`
}
type GroupResponse struct {
	Group group.Group `json:"group"`
	Chat  string      `json:"chat"`
}

func (Server) Create(ctx context.Context, req *CreateRequest, rsp *GroupResponse) error {
	g, err := group.Create(service.AccountFrom(ctx), req.Name, req.Encrypted)
	if err != nil {
		return err
	}
	rsp.Group = g
	rsp.Chat = "/chat?id=" + group.RoomPrefix + g.ID
	return nil
}

type ReadRequest struct {
	ID string `json:"id" required:"true" description:"Group ID"`
}

func (Server) Read(ctx context.Context, req *ReadRequest, rsp *GroupResponse) error {
	g, err := group.Read(req.ID, service.AccountFrom(ctx))
	if err != nil {
		return err
	}
	rsp.Group = g
	rsp.Chat = "/chat?id=" + group.RoomPrefix + g.ID
	return nil
}

type UpdateRequest struct {
	ID      string `json:"id" required:"true"`
	Action  string `json:"action" required:"true" description:"invite, accept, decline, remove, role, rename or delete"`
	Account string `json:"account,omitempty" description:"Local username for invite, remove or role; defaults to yourself for remove"`
	Value   string `json:"value,omitempty" description:"New name for rename, or member/admin/owner for role"`
}
type UpdateResponse struct {
	Result string `json:"result"`
}

func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *UpdateResponse) error {
	actor := service.AccountFrom(ctx)
	if actor == "" {
		return errors.New("sign in to manage groups")
	}
	target := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.Account), "@"))
	if req.Action == "invite" {
		acc, err := auth.GetAccount(target)
		if err != nil || acc == nil || acc.Agent || auth.IsBanned(target) {
			return errors.New("choose an existing person's Micro username")
		}
	}
	if req.Action == "remove" && target == "" {
		target = actor
	}
	if err := group.Change(req.ID, actor, req.Action, target, req.Value); err != nil {
		return err
	}
	event.Publish(event.Event{Type: group.Changed, Data: map[string]interface{}{"id": req.ID}})
	rsp.Result = "Updated"
	return nil
}
func Forget(account string) {
	if err := group.Forget(account); err != nil {
		app.Log("groups", "account removal: %v", err)
	}
	event.Publish(event.Event{Type: group.Changed})
}
func Load() {
	if err := group.Load(); err != nil {
		app.Log("groups", "membership unavailable: %v", err)
	}
	if err := service.Register(Spec); err != nil {
		app.Log("groups", "register: %v", err)
	}
}

var Spec = service.Spec{
	Name: "groups", Handler: new(Server), Description: "Private groups for friends, family and shared conversations", Page: "/groups", Icon: "contacts.svg", Scoped: true,
	Endpoints: map[string]service.Endpoint{
		"List":   {Doc: "List your groups and pending invitations"},
		"Read":   {Doc: "Read a group you belong to and open its chat"},
		"Create": {Writes: true, Doc: "Create a private group"},
		"Update": {Writes: true, Destructive: true, Doc: "Manage invitations, membership and roles. Accept invitations before accessing a group. Removing yourself leaves; owners must transfer ownership first. Delete closes the group for everyone."},
	},
}
