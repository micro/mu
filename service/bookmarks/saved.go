// Package bookmarks exposes the caller's saved reading through pages and tools.
package bookmarks

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"
	"mu/internal/app"
	store "mu/internal/bookmarks"
	"mu/internal/service"
	pb "mu/proto/service/bookmarks/v1"
)

type Server struct{}
type AddRequest = pb.AddRequest
type ItemResponse = pb.ItemResponse

func (Server) Add(ctx context.Context, req *AddRequest, rsp *ItemResponse) error {
	item, err := store.Add(service.AccountFrom(ctx), store.Item{Ref: req.Ref, URL: req.Url, Title: req.Title, Note: req.Note})
	if err != nil {
		return err
	}
	rsp.Item = itemMessage(item)
	rsp.Text = "Saved: " + item.Title
	return nil
}

type ListRequest = pb.ListRequest
type ListResponse = pb.ListResponse

func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	items, total, err := store.List(service.AccountFrom(ctx), req.Query, req.Kind, int(max(0, min(req.Offset, int64(store.MaxItems)))), int(max(0, min(req.Limit, 101))))
	rsp.Total = int64(total)
	for i := range items {
		rsp.Items = append(rsp.Items, itemMessage(&items[i]))
	}
	for _, i := range rsp.Items {
		rsp.Text += fmt.Sprintf("- %s (%s) [id: %s]\n", i.Title, i.Url, i.Id)
	}
	return err
}

type GetRequest = pb.GetRequest

func (Server) Get(ctx context.Context, req *GetRequest, rsp *ItemResponse) error {
	item, err := store.Get(service.AccountFrom(ctx), req.Id)
	if err != nil {
		return err
	}
	rsp.Item = itemMessage(item)
	rsp.Text = store.Context(item)
	return nil
}

type AnnotateRequest = pb.AnnotateRequest
type Response = pb.Response

func (Server) Annotate(ctx context.Context, req *AnnotateRequest, rsp *Response) error {
	err := store.Annotate(service.AccountFrom(ctx), req.Id, req.Note)
	if err == nil {
		rsp.Text = "Note bookmarks."
	}
	return err
}
func (Server) Delete(ctx context.Context, req *GetRequest, rsp *Response) error {
	err := store.Remove(service.AccountFrom(ctx), req.Id)
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

var Spec = service.Spec{Name: "bookmarks", Label: "Bookmarks", Description: "Save articles, videos and links", Page: "/bookmarks", Icon: "bookmarks.svg", Scoped: true, Handler: new(Server), Endpoints: map[string]service.Endpoint{
	"Add":      {Aliases: []string{"saved_add"}, Writes: true, Doc: "Save an article, video, blog post or link privately. Re-saving preserves its note"},
	"List":     {Aliases: []string{"saved_list"}, Doc: "Find your saved reading by text or kind, newest first. Private notes are searched too"},
	"Get":      {Aliases: []string{"saved_get"}, Doc: "Read one saved item and its private note, with available archived text. Videos have descriptions, not transcripts"},
	"Annotate": {Aliases: []string{"saved_annotate"}, Writes: true, Doc: "Set or clear a private note on one saved item"},
	"Delete":   {Aliases: []string{"saved_delete"}, Destructive: true, Doc: "Remove one of your saved items"},
}}

func itemMessage(i *store.Item) *pb.Item {
	if i == nil {
		return nil
	}
	out := &pb.Item{Id: i.ID, Ref: i.Ref, Url: i.URL, Title: i.Title, Kind: i.Kind, Source: i.Source, Excerpt: i.Excerpt, Note: i.Note}
	if !i.Created.IsZero() {
		out.Created = timestamppb.New(i.Created)
	}
	return out
}
