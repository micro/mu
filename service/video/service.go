package video

import (
	"context"

	"mu/internal/service"
)

// Server is the go-micro service handler for video.
type Server struct{}

// ListRequest controls how many videos to return.
type ListRequest struct {
	Limit int `json:"limit" description:"Optional max number of videos (default all recent)"`
}

// ListResponse is a model-ready video list.
type ListResponse struct {
	Text string `json:"text" description:"Latest videos from curated channels"`
}

// List returns the latest videos from curated channels.
// @example {}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = LatestText(req.Limit)
	return nil
}

var toolDocs = service.Docs{
	"List": "Read the latest videos from curated channels",
}
