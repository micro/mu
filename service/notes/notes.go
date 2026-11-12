// Package notes is what the caller wrote down: a title, what is under it, and
// nothing that expires.
//
// It was memory, then cache, and the second name was the worse of the two. A
// cache is a thing you are allowed to lose; nothing here is evictable, nothing
// has a lifetime, and the word only ever described the shape of the lookup. The
// register the rest of the catalogue is written in — Contacts, Docs, Events,
// Mail — is what a person owns, and what a person owns here is notes.
//
// A note is addressed by its title, so writing the same title twice rewrites
// the note rather than duplicating it. That is what makes "remember that I'm in
// London" safe to say twice, and it is why there is no separate update.
//
// What separates it from docs is shape, not durability: a note is a title and
// some text you read back whole; docs holds named collections you query.
package notes

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/app"
	"mu/internal/notes"
	"mu/internal/service"
	"mu/internal/thread"
	pb "mu/proto/service/notes/v1"
)

// Server is the go-micro handler. Its exported methods become the notes_* tools.
type Server struct{}

// caller resolves the authenticated account from call metadata. No owner field
// on any request here, deliberately: an argument can be chosen by whoever makes
// the call, context metadata cannot — and a person's notes are theirs.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to use notes")
	}
	return id, nil
}

// ── Add ─────────────────────────────────────────────────────────

type AddRequest = pb.AddRequest

type AddResponse = pb.AddResponse

// Add writes a note.
// @example {"title": "location", "text": "London"}
func (Server) Add(ctx context.Context, req *AddRequest, rsp *AddResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	title, text := strings.TrimSpace(req.Title), strings.TrimSpace(req.Text)
	if title == "" || text == "" {
		return fmt.Errorf("a note needs a title and some text")
	}
	source := service.SourceThreadFrom(ctx)
	if thread.Get(owner, source) == nil {
		source = ""
	}
	notes.AddFrom(owner, title, text, source)
	rsp.Url = "/notes"
	for _, e := range notes.All(owner) {
		if strings.EqualFold(e.Title, title) {
			rsp.Url = "/notes?id=" + e.ID
			rsp.Item = &pb.ResultEmbed{Kind: "note", Id: e.ID, Title: e.Title, Url: rsp.Url, Body: e.Text}
			break
		}
	}
	rsp.Result = "Saved. Open: " + rsp.Url
	return nil
}

// ── Get ─────────────────────────────────────────────────────────

// GetRequest reads one note.
type GetRequest = pb.GetRequest

// GetResponse is the note.
type GetResponse = pb.GetResponse

// Get reads one note by title. Use notes_list when you do not know the title.
// @example {"title": "location"}
func (Server) Get(ctx context.Context, req *GetRequest, rsp *GetResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return fmt.Errorf("say which note to read")
	}
	rsp.Text = notes.Get(owner, title)
	if rsp.Text == "" {
		rsp.Note = "Nothing written under " + title + "."
		return nil
	}
	for _, e := range notes.All(owner) {
		if strings.EqualFold(e.Title, title) {
			rsp.Item = &pb.ResultEmbed{Kind: "note", Id: e.ID, Title: e.Title, Url: "/notes?id=" + e.ID, Body: e.Text}
			break
		}
	}
	rsp.Note = title + ": " + rsp.Text
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest = pb.ListRequest

type Note = pb.Note

type ListResponse = pb.ListResponse

// List returns every note the caller has.
// @example {}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	for _, e := range notes.All(owner) {
		rsp.Notes = append(rsp.Notes, &Note{Title: e.Title, Text: e.Text, SourceThread: e.SourceThread, UpdatedAt: timestamppb.New(e.UpdatedAt)})
	}
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest = pb.DeleteRequest

type DeleteResponse = pb.DeleteResponse

// Delete removes one note.
// @example {"title": "location"}
func (Server) Delete(ctx context.Context, req *DeleteRequest, rsp *DeleteResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return fmt.Errorf("say which note to delete")
	}
	notes.Delete(owner, title)
	rsp.Result = "deleted"
	return nil
}

// LoadService registers notes as a service.
func LoadService() {
	if err := service.Register(Spec); err != nil {
		app.Log("notes", "service register failed: %v", err)
	}

	// The index is a separate file from the store, so an instance that has one
	// and not the other answers every search with nothing — including every
	// instance upgrading to the first build that indexes notes at all.
	//
	// Here rather than in the store's init(): init runs before the index is
	// open, and a package that indexed from init would write into whatever
	// happened to exist at the time. In the background because it is a
	// boot-time cost proportional to how much has been written down, and
	// nothing needs it before the first search.
	go notes.Reindex()
}

var Spec = service.Spec{
	Name:        "notes",
	Icon:        "notes.svg",
	Handler:     new(Server),
	Description: "Write and save notes",
	Page:        "/notes",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		// Two rounds of renaming happened before this one, and every name is
		// still callable: an agent written against memory_set or cache_set
		// should not break because the label above it changed.
		"Add": {Writes: true, Aliases: []string{"cache_set", "memory_set"},
			Doc: "Write a note under a title, so it is there next conversation. Writing a title that exists rewrites that note"},
		"Get": {Aliases: []string{"cache_get"},
			Doc: "Read one note by title. Use it when you know the title; notes_list when you do not"},
		"List": {Aliases: []string{"cache_list", "memory_list"},
			Doc: "List every note the caller has written, with its text"},
		"Delete": {Aliases: []string{"cache_delete", "memory_delete"}, Destructive: true,
			Doc: "Delete one note by title"},
	},
}
