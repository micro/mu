package video

import "context"

// Server is the go-micro service handler for video.
type Server struct{}

// LatestRequest controls how many videos to return.
type LatestRequest struct {
	Limit int `json:"limit" description:"Optional max number of videos (default all recent)"`
}

// LatestResponse is a model-ready video list.
type LatestResponse struct {
	Text string `json:"text" description:"Latest videos from curated channels"`
}

// Latest returns the latest videos from curated channels.
// @example {}
func (Server) Latest(_ context.Context, req *LatestRequest, rsp *LatestResponse) error {
	rsp.Text = LatestText(req.Limit)
	return nil
}
