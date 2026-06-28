package social

import "context"

// Server is the go-micro service handler for social.
type Server struct{}

// FeedRequest controls how many posts to return.
type FeedRequest struct {
	Limit int `json:"limit" description:"Optional max number of posts (default all recent)"`
}

// FeedResponse is a model-ready social feed.
type FeedResponse struct {
	Text string `json:"text" description:"Latest social posts from the network"`
}

// Feed returns the latest social posts from the network.
// @example {}
func (Server) Feed(_ context.Context, req *FeedRequest, rsp *FeedResponse) error {
	rsp.Text = FeedText(req.Limit)
	return nil
}
