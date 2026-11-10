// Package groups exposes shared membership through the UI and service tools.
package groups

import (
	"context"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/event"
	"mu/internal/group"
	"mu/internal/service"
	pb "mu/proto/service/groups/v1"
)

type Server struct{}
type ListRequest = pb.ListRequest
type ListResponse = pb.ListResponse

func (Server) List(ctx context.Context, _ *ListRequest, rsp *ListResponse) error {
	groups, invitations, err := group.List(service.AccountFrom(ctx))
	if err != nil {
		return err
	}
	for _, g := range groups {
		rsp.Groups = append(rsp.Groups, groupMessage(g))
	}
	for _, i := range invitations {
		rsp.Invitations = append(rsp.Invitations, &pb.GroupPending{Id: i.ID, Name: i.Name, Account: i.Account, InvitedBy: i.InvitedBy, Expires: timestamppb.New(i.Expires)})
	}
	return nil
}

type CreateRequest = pb.CreateRequest
type GroupResponse = pb.GroupResponse

func (Server) Create(ctx context.Context, req *CreateRequest, rsp *GroupResponse) error {
	g, err := group.Create(service.AccountFrom(ctx), req.Name, req.Encrypted, req.Description)
	if err != nil {
		return err
	}
	rsp.Group = groupMessage(g)
	rsp.Chat = "/chat?id=" + group.RoomPrefix + g.ID
	return nil
}

type ReadRequest = pb.ReadRequest

func (Server) Read(ctx context.Context, req *ReadRequest, rsp *GroupResponse) error {
	g, err := group.Read(req.Id, service.AccountFrom(ctx))
	if err != nil {
		return err
	}
	rsp.Group = groupMessage(g)
	rsp.Chat = "/chat?id=" + group.RoomPrefix + g.ID
	return nil
}

type UpdateRequest = pb.UpdateRequest
type UpdateResponse = pb.UpdateResponse

func (Server) Update(ctx context.Context, req *UpdateRequest, rsp *UpdateResponse) error {
	actor := service.AccountFrom(ctx)
	if actor == "" {
		return service.Fail(401, "sign in to manage groups")
	}
	target := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(req.Account), "@"))
	if req.Action == "invite" {
		acc, err := auth.GetAccount(target)
		if err != nil || acc == nil || acc.Agent || auth.IsBanned(target) {
			return service.Fail(400, "choose an existing person's Micro username")
		}
	}
	if req.Action == "remove" && target == "" {
		target = actor
	}
	if err := group.Change(req.Id, actor, req.Action, target, req.Value); err != nil {
		return err
	}
	event.Publish(event.Event{Type: group.Changed, Data: map[string]interface{}{"id": req.Id}})
	rsp.Result = "Updated"
	return nil
}
func Forget(account string) error {
	if err := group.Forget(account); err != nil {
		return err
	}
	event.Publish(event.Event{Type: group.Changed})
	return nil
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

// groupMessage projects public membership data; storage-only invitation hashes
// never enter the service contract.
func groupMessage(g group.Group) *pb.GroupGroup {
	out := &pb.GroupGroup{Id: g.ID, Name: g.Name, Description: g.Description, Owner: g.Owner, Encrypted: g.Encrypted, Created: timestamppb.New(g.Created), SpendLimit: int32(g.SpendLimit), MembersUseAi: g.MembersUseAI, Members: g.Members, Muted: g.Muted, Activity: map[string]*timestamppb.Timestamp{}, Seen: map[string]*timestamppb.Timestamp{}, Conversations: map[string]*pb.GroupConversation{}, Invitations: map[string]*pb.GroupInvitation{}}
	if !g.BillingUntil.IsZero() {
		out.BillingUntil = timestamppb.New(g.BillingUntil)
	}
	if !g.LinkExpires.IsZero() {
		out.LinkExpires = timestamppb.New(g.LinkExpires)
	}
	for k, v := range g.Activity {
		out.Activity[k] = timestamppb.New(v)
	}
	for k, v := range g.Seen {
		out.Seen[k] = timestamppb.New(v)
	}
	for k, v := range g.Conversations {
		out.Conversations[k] = &pb.GroupConversation{Id: v.ID, Name: v.Name, Agent: v.Agent, Created: timestamppb.New(v.Created)}
	}
	for k, v := range g.Invitations {
		out.Invitations[k] = &pb.GroupInvitation{Account: v.Account, InvitedBy: v.InvitedBy, Expires: timestamppb.New(v.Expires)}
	}
	return out
}
