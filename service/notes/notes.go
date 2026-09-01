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

	"mu/internal/app"
	"mu/internal/notes"
	"mu/internal/service"
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

type AddRequest struct {
	Title string `json:"title" required:"true" description:"What the note is called, e.g. \"location\" or \"project brief\". Writing a title that exists rewrites that note"`
	Text  string `json:"text" required:"true" description:"What the note says"`
}

type AddResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

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
	notes.Add(owner, title, text)
	rsp.Result = "saved"
	return nil
}

// ── Get ─────────────────────────────────────────────────────────

// GetRequest reads one note.
type GetRequest struct {
	Title string `json:"title" required:"true" description:"The note's title, as given to notes_add"`
}

// GetResponse is the note.
type GetResponse struct {
	Text string `json:"text" description:"What the note says, or empty if there is no note by that title"`
	Note string `json:"note" description:"The title and its text, or a note that nothing is written under it"`
}

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
	rsp.Note = title + ": " + rsp.Text
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct{}

type Note struct {
	Title string `json:"title" description:"What the note is called"`
	Text  string `json:"text" description:"What it says"`
}

type ListResponse struct {
	Notes []Note `json:"notes" description:"Every note the caller has written"`
}

// List returns every note the caller has.
// @example {}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	for _, e := range notes.All(owner) {
		rsp.Notes = append(rsp.Notes, Note{Title: e.Title, Text: e.Text})
	}
	return nil
}

// ── Delete ──────────────────────────────────────────────────────

type DeleteRequest struct {
	Title string `json:"title" required:"true" description:"The note to delete, as returned by notes_list"`
}

type DeleteResponse struct {
	Result string `json:"result" description:"Confirmation"`
}

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
	Description: "What you wrote down, and what an agent wrote down for you",
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
