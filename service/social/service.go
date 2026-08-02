package social

import (
	"context"

	"mu/internal/service"
)

// Server is the go-micro service handler for social.
type Server struct{}

// ListRequest controls how many posts to return.
type ListRequest struct {
	Limit int `json:"limit" description:"Optional max number of posts (default all recent)"`
}

// ListResponse is a model-ready social feed.
type ListResponse struct {
	Text string `json:"text" description:"Latest social posts from the network"`
}

// List returns the latest social posts from the network.
// @example {}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = FeedText(req.Limit)
	return nil
}

var Spec = service.Spec{
	Name:        "social",
	Handler:     new(Server),
	Description: "Public threads, replies and status",
	Page:        "/social",
	Icon:        "social.svg",
	Endpoints: map[string]service.Endpoint{
		"List": {Doc: "Read the latest social posts from the network"},
	},
}
