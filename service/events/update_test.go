package events

import (
	"context"
	"mu/internal/service"
	"testing"
	"time"
)

func TestUpdatePreservesIdentityAndChecksOwner(t *testing.T) {
	when := time.Now().Add(time.Hour)
	e, err := CreateStanding("update-owner", "Title", when, "note", 30, "daily", "prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer Cancel("update-owner", e.ID)
	empty := ""
	var rsp UpdateResponse
	req := &UpdateRequest{ID: e.ID, Note: &empty}
	if err := (Server{}).Update(service.WithAccount(context.Background(), "other"), req, &rsp); err == nil {
		t.Fatal("changed another owner's event")
	}
	if err := (Server{}).Update(service.WithAccount(context.Background(), "update-owner"), req, &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.Item.ID != e.ID || rsp.Item.Note != "" || rsp.Item.Title != "Title" || rsp.Item.Repeat != "daily" || !rsp.Item.When.Equal(when.UTC()) {
		t.Fatalf("unexpected update: %+v", rsp.Item)
	}
	bad := "yearly-ish"
	req.Repeat = &bad
	if err := (Server{}).Update(service.WithAccount(context.Background(), "update-owner"), req, &rsp); err == nil {
		t.Fatal("invalid recurrence accepted")
	}
}

func TestExternalCalendarPagesAreStructured(t *testing.T) {
	old := ExternalEntries
	ExternalEntries = func(owner string, from, to time.Time) []External {
		return []External{{Title: "First", Start: from}, {Title: "Second", Start: from.Add(time.Hour)}}
	}
	defer func() { ExternalEntries = old }()
	var rsp ListResponse
	if err := (Server{}).List(service.WithAccount(context.Background(), "external_pages"), &ListRequest{Offset: 1, Limit: 1}, &rsp); err != nil {
		t.Fatal(err)
	}
	if rsp.ExternalTotal != 2 || len(rsp.External) != 1 || rsp.External[0].Title != "Second" || rsp.ExternalNextOffset != nil {
		t.Fatalf("bad external page: %+v", rsp)
	}
}
