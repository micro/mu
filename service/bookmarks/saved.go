// Package bookmarks exposes the caller's saved reading through pages and tools.
package bookmarks

import (
	"context"
	"fmt"

	"mu/internal/app"
	store "mu/internal/bookmarks"
	"mu/internal/service"
)

type Server struct{}
type AddRequest struct {
	Ref   string `json:"ref,omitempty" description:"Public archive item id for an article, video or blog post"`
	URL   string `json:"url,omitempty" description:"Link to save when no archive id is available"`
	Title string `json:"title,omitempty" description:"Title for a link"`
	Note  string `json:"note,omitempty" description:"Optional private note"`
}
type ItemResponse struct {
	Item *store.Item `json:"item"`
	Text string      `json:"text"`
}

func (Server) Add(ctx context.Context, req *AddRequest, rsp *ItemResponse) error {
	item, err := store.Add(service.AccountFrom(ctx), store.Item{Ref: req.Ref, URL: req.URL, Title: req.Title, Note: req.Note})
	if err != nil {
		return err
	}
	rsp.Item = item
	rsp.Text = "Saved: " + item.Title
	return nil
}

type ListRequest struct {
	Query  string `json:"query,omitempty" description:"Search your saved titles, links, excerpts and private notes"`
	Kind   string `json:"kind,omitempty" description:"article, video, post or link; empty for all"`
	Offset int    `json:"offset,omitempty" description:"Number of matching items to skip"`
	Limit  int    `json:"limit,omitempty" description:"Maximum items, default 20, at most 100"`
}
type ListResponse struct {
	Items []store.Item `json:"items"`
	Total int          `json:"total"`
	Text  string       `json:"text"`
}

func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	var err error
	rsp.Items, rsp.Total, err = store.List(service.AccountFrom(ctx), req.Query, req.Kind, req.Offset, req.Limit)
	for _, i := range rsp.Items {
		rsp.Text += fmt.Sprintf("- %s (%s) [id: %s]\n", i.Title, i.URL, i.ID)
	}
	return err
}

type GetRequest struct {
	ID string `json:"id" required:"true" description:"Saved item id"`
}

func (Server) Get(ctx context.Context, req *GetRequest, rsp *ItemResponse) error {
	item, err := store.Get(service.AccountFrom(ctx), req.ID)
	if err != nil {
		return err
	}
	rsp.Item = item
	rsp.Text = store.Context(item)
	return nil
}

type AnnotateRequest struct {
	ID   string `json:"id" required:"true"`
	Note string `json:"note" description:"Private note, or empty to clear"`
}
type Response struct {
	Text string `json:"text"`
}

func (Server) Annotate(ctx context.Context, req *AnnotateRequest, rsp *Response) error {
	err := store.Annotate(service.AccountFrom(ctx), req.ID, req.Note)
	if err == nil {
		rsp.Text = "Note bookmarks."
	}
	return err
}
func (Server) Delete(ctx context.Context, req *GetRequest, rsp *Response) error {
	err := store.Remove(service.AccountFrom(ctx), req.ID)
	if err == nil {
		rsp.Text = "Removed."
	}
	return err
}
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("bookmarks", "service register failed: %v", err)
	}
}
func DeleteAll(owner string) {
	if err := store.Clear(owner); err != nil {
		app.Log("bookmarks", "account deletion failed: %v", err)
	}
}

var Spec = service.Spec{Name: "bookmarks", Label: "Bookmarks", Description: "Your private saved articles, videos and links", Page: "/bookmarks", Icon: "bookmarks.svg", Scoped: true, Handler: new(Server), Endpoints: map[string]service.Endpoint{
	"Add":      {Aliases: []string{"saved_add"}, Writes: true, Doc: "Save an article, video, blog post or link privately. Re-saving preserves its note"},
	"List":     {Aliases: []string{"saved_list"}, Doc: "Find your saved reading by text or kind, newest first. Private notes are searched too"},
	"Get":      {Aliases: []string{"saved_get"}, Doc: "Read one saved item and its private note, with available archived text. Videos have descriptions, not transcripts"},
	"Annotate": {Aliases: []string{"saved_annotate"}, Writes: true, Doc: "Set or clear a private note on one saved item"},
	"Delete":   {Aliases: []string{"saved_delete"}, Destructive: true, Doc: "Remove one of your saved items"},
}}
