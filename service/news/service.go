package news

import (
	"context"

	"mu/internal/service"
	"mu/service/wallet"
)

// Server is the go-micro service handler for news. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// ListRequest filters the headline list.
type ListRequest struct {
	Topic string `json:"topic" description:"Optional topic/category filter (e.g. tech, world, business)"`
	Limit int    `json:"limit" description:"Optional max number of headlines (default 30)"`
}

// ListResponse is a model-ready list of headlines.
type ListResponse struct {
	Text string `json:"text" description:"Recent headlines with short summaries, balanced across topics"`
}

// List returns recent news headlines with short summaries, balanced across
// topics (not dominated by one topic like crypto).
// @example {"topic": "tech"}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = HeadlinesText(req.Topic, req.Limit)
	return nil
}

// ReadRequest selects one article.
type ReadRequest struct {
	ID string `json:"id" description:"Article id (from Headlines) or article URL"`
}

// ReadResponse is the full article text.
type ReadResponse struct {
	Text string `json:"text" description:"Title, source, summary and body of the article"`
}

// SearchRequest searches indexed and live news for a user topic.
type SearchRequest struct {
	Query string `json:"query" description:"Search terms, e.g. latest AI news"`
}

// SearchResponse is model-ready news_search JSON, including freshness metadata
// for date-sensitive queries.
type SearchResponse struct {
	Text string `json:"text" description:"JSON payload with query, results, count, and freshness caveats when relevant"`
}

// Read reads one news article in full by its id (from Headlines) or by URL.
// @example {"id": "https://example.com/article"}
func (Server) Read(_ context.Context, req *ReadRequest, rsp *ReadResponse) error {
	text, err := ArticleText(req.ID)
	rsp.Text = text
	return err
}

// Search returns the same model-ready payload as the public news_search tool so
// native go-micro agent calls receive freshness caveats before stale stories.
// @example {"query":"latest AI news"}
func (Server) Search(_ context.Context, req *SearchRequest, rsp *SearchResponse) error {
	text, err := SearchToolText(req.Query)
	rsp.Text = text
	return err
}

var Spec = service.Spec{
	Name:        "news",
	Handler:     new(Server),
	Description: "Headlines aggregated from RSS feeds, with search and full articles",
	Page:        "/news",
	Icon:        "news.png",
	Endpoints: map[string]service.Endpoint{
		"List":   {Doc: "Read recent news headlines with short summaries, balanced across topics"},
		"Read":   {Doc: "Read one news article in full by its id or URL"},
		"Search": {Doc: "Search indexed and live news for a topic", Cost: wallet.OpNewsSearch},
	},
}
