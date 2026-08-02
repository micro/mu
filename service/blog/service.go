package blog

import (
	"context"

	"mu/internal/service"
)

// Server is the go-micro service handler for blog.
type Server struct{}

// ListRequest controls how many posts to return.
type ListRequest struct {
	Limit int `json:"limit" description:"Optional max number of posts (default all recent)"`
}

// ListResponse is a model-ready list of recent posts.
type ListResponse struct {
	Text string `json:"text" description:"Recent blog posts: titles, snippets and ids"`
}

// List returns recent blog posts (titles, snippets and ids).
// @example {}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = RecentText(req.Limit)
	return nil
}

var toolDocs = service.Docs{
	"List": "Read recent blog posts — titles, snippets and ids",
}
