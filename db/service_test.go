package db

import (
	"context"
	"strings"
	"testing"

	"mu/internal/service"
)

// Identity is never taken from the request — there is no account field on any
// request in this package. A caller with no authenticated context is a guest
// and gets nothing, however it phrases the call.
func TestGuestIsRefused(t *testing.T) {
	var s Server
	ctx := context.Background()

	if err := s.Create(ctx, &CreateRequest{Collection: "notes", Data: map[string]any{"a": 1}}, &CreateResponse{}); err == nil {
		t.Error("Create succeeded for a guest")
	}
	if err := s.List(ctx, &ListRequest{Collection: "notes"}, &ListResponse{}); err == nil {
		t.Error("List succeeded for a guest")
	}
	if err := s.Get(ctx, &GetRequest{Collection: "notes", ID: "x"}, &GetResponse{}); err == nil {
		t.Error("Get succeeded for a guest")
	}
	if err := s.Update(ctx, &UpdateRequest{Collection: "notes", ID: "x"}, &UpdateResponse{}); err == nil {
		t.Error("Update succeeded for a guest")
	}
	if err := s.Delete(ctx, &DeleteRequest{Collection: "notes", ID: "x"}, &DeleteResponse{}); err == nil {
		t.Error("Delete succeeded for a guest")
	}
}

func TestCollectionIsRequired(t *testing.T) {
	var s Server
	ctx := service.WithAccount(context.Background(), "alice")

	err := s.Create(ctx, &CreateRequest{Data: map[string]any{"a": 1}}, &CreateResponse{})
	if err == nil || !strings.Contains(err.Error(), "collection") {
		t.Errorf("err = %v, want a complaint about the collection", err)
	}
}

// Records belong to the account on the context, and one account cannot read
// another's by any means the request offers.
func TestRecordsAreScopedToTheContextAccount(t *testing.T) {
	var s Server
	alice := service.WithAccount(context.Background(), "alice-scope-test")
	bob := service.WithAccount(context.Background(), "bob-scope-test")

	var created CreateResponse
	if err := s.Create(alice, &CreateRequest{
		Collection: "notes",
		Data:       map[string]any{"title": "alice's note"},
	}, &created); err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Record.Owner != "alice-scope-test" {
		t.Fatalf("owner = %q, want the context account", created.Record.Owner)
	}

	var mine ListResponse
	if err := s.List(alice, &ListRequest{Collection: "notes"}, &mine); err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, r := range mine.Records {
		if r.ID == created.Record.ID {
			found = true
		}
	}
	if !found {
		t.Error("alice cannot see her own record")
	}

	var theirs ListResponse
	if err := s.List(bob, &ListRequest{Collection: "notes"}, &theirs); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, r := range theirs.Records {
		if r.ID == created.Record.ID {
			t.Error("bob can see alice's private record")
		}
	}

	// Cleanup.
	s.Delete(alice, &DeleteRequest{Collection: "notes", ID: created.Record.ID}, &DeleteResponse{}) //nolint:errcheck
}
