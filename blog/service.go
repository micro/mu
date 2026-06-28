package blog

import "context"

// Server is the go-micro service handler for blog.
type Server struct{}

// RecentRequest controls how many posts to return.
type RecentRequest struct {
	Limit int `json:"limit" description:"Optional max number of posts (default all recent)"`
}

// RecentResponse is a model-ready list of recent posts.
type RecentResponse struct {
	Text string `json:"text" description:"Recent blog posts: titles, snippets and ids"`
}

// Recent returns recent blog posts (titles, snippets and ids).
// @example {}
func (Server) Recent(_ context.Context, req *RecentRequest, rsp *RecentResponse) error {
	rsp.Text = RecentText(req.Limit)
	return nil
}
