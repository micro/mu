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

// FetchRequest is a URL to fetch and clean.
type FetchRequest struct {
	URL string `json:"url" description:"The URL to fetch"`
}

// FetchResponse is the cleaned readable page content.
type FetchResponse struct {
	Title   string `json:"title"`
	Content string `json:"content" description:"Cleaned readable content (ads/nav stripped)"`
}

// Fetch fetches a web page and returns its cleaned readable content.
// @example {"url": "https://example.com"}
func (Server) Fetch(_ context.Context, req *FetchRequest, rsp *FetchResponse) error {
	title, content, err := FetchAndExtract(req.URL)
	if err != nil {
		return err
	}
	rsp.Title = title
	rsp.Content = content
	return nil
}
