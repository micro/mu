package alerts

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	store "mu/internal/alerts"
	"mu/internal/group"
	"mu/internal/service"
)

func TestScopedToolsAndRevokedMembership(t *testing.T) {
	actor := uuid.NewString()
	member := uuid.NewString()
	ctx := service.WithAccount(context.Background(), actor)
	srv := Server{}
	var raised ItemResponse
	if err := srv.Raise(context.Background(), &RaiseRequest{Key: "one", Title: "private"}, &raised); err == nil {
		t.Fatal("anonymous raise")
	}
	if err := srv.Raise(ctx, &RaiseRequest{Key: "one", Title: "private"}, &raised); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Clear(actor) })
	if err := srv.Update(service.WithAccount(context.Background(), member), &UpdateRequest{Id: raised.Item.Id, State: "resolved"}, &Response{}); err == nil {
		t.Fatal("cross-account mutation")
	}
	g, err := group.Create(actor, "Test shared alerts", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = group.Change(g.ID, actor, "delete", "", ""); _ = store.Clear(group.Scope(g.ID)) })
	if err = srv.Raise(ctx, &RaiseRequest{Group: g.ID, Key: "shared", Title: "Shared condition"}, &raised); err != nil {
		t.Fatal(err)
	}
	memberCtx := service.WithAccount(context.Background(), member)
	for _, action := range []string{"invite", "accept"} {
		who := actor
		target := member
		if action == "accept" {
			who = member
			target = ""
		}
		if err = group.Change(g.ID, who, action, target, ""); err != nil {
			t.Fatal(err)
		}
	}
	var list ListResponse
	if err = srv.List(memberCtx, &ListRequest{Group: g.ID}, &list); err != nil || len(list.Items) != 1 {
		t.Fatal(&list, err)
	}
	if err = srv.Update(memberCtx, &UpdateRequest{Group: g.ID, Id: raised.Item.Id, State: "resolved"}, &Response{}); err == nil {
		t.Fatal("member changed shared state")
	}
	if !strings.Contains(Preview(member), "alerts needing attention") {
		t.Fatal("shared preview missing")
	}
	if err = group.Change(g.ID, actor, "remove", member, ""); err != nil {
		t.Fatal(err)
	}
	if err = srv.List(memberCtx, &ListRequest{Group: g.ID}, &list); err == nil {
		t.Fatal("removed member read alerts")
	}
	if strings.Contains(Preview(member), "Test shared alerts") {
		t.Fatal("removed member preview leaked")
	}
	if _, err = owner(group.Scope(g.ID), "", true); err == nil {
		t.Fatal("unverified group agent principal admitted")
	}
}
