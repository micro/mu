package events

import (
	"context"
	"mu/internal/service"
	"strings"
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
	ExternalEntries = func(owner string, from, to time.Time, limit int) []External {
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

func TestUpdatePublishesRevisedCalendarEntry(t *testing.T) {
	e, err := CreateStanding("invite_owner", "Original", time.Now().Add(time.Hour), "", 30, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer Cancel(e.Owner, e.ID)
	old := OnCreate
	invites := make(chan *Event, 1)
	OnCreate = func(e *Event) { invites <- e }
	defer func() { OnCreate = old }()
	title := "Revised"
	minutes := 60
	var rsp UpdateResponse
	if err := (Server{}).Update(service.WithAccount(context.Background(), e.Owner), &UpdateRequest{ID: e.ID, Title: &title, Minutes: &minutes}, &rsp); err != nil {
		t.Fatal(err)
	}
	select {
	case invite := <-invites:
		ics := ICS(invite, "owner@example.com")
		if invite.ID != e.ID || !strings.Contains(ics, "SEQUENCE:1") || !strings.Contains(ics, "SUMMARY:Revised") {
			t.Fatalf("bad revised invite: %s", ics)
		}
	case <-time.After(time.Second):
		t.Fatal("no revised calendar invite")
	}
}
