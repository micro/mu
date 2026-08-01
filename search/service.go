package search

import "context"

// Server is the go-micro service handler for web search.
type Server struct{}

// SearchRequest is a web search query.
type SearchRequest struct {
	Query string `json:"query" description:"Search query"`
	Limit int    `json:"limit" description:"Optional max number of results"`
}

// SearchResponse is a model-ready set of results.
type SearchResponse struct {
	Text string `json:"text" description:"Search results for the query"`
}

// Search searches the web for current information and news.
// @example {"query": "latest AI news"}
func (Server) Search(_ context.Context, req *SearchRequest, rsp *SearchResponse) error {
	rsp.Text = WebSearchText(req.Query, req.Limit)
	return nil
}
