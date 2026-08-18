package news

import (
	"context"
	"time"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for news. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// ListRequest filters the headline list.
type ListRequest struct {
	Topic string `json:"topic" description:"Optional topic/category filter (e.g. tech, world, business)"`
	Limit int    `json:"limit" description:"Optional max number of headlines (default 30)"`
}

// Headline is one story, for a caller that has to do something with it rather
// than read it.
type Headline struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Description string `json:"description,omitempty"`
}

// ListResponse is a model-ready list of headlines, and the same headlines as
// data.
//
// Both, because there are two kinds of caller and they want different things. A
// model reads Text — prose is cheaper than JSON and needs no schema explained.
// A program wants the fields: agent/blog picks the top stories of a category to
// research, and parsing them back out of prose it had just formatted was how it
// came to import this package instead of calling it.
//
// Text stays first and stays the answer. Items is what it is made of.
type ListResponse struct {
	Text  string     `json:"text" description:"Recent headlines with short summaries, balanced across topics"`
	Items []Headline `json:"items" description:"The same headlines as data: title, url, category, description"`
}

// List returns recent news headlines with short summaries, balanced across
// topics (not dominated by one topic like crypto).
// @example {"topic": "tech"}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = HeadlinesText(req.Topic, req.Limit)
	rsp.Items = HeadlineItems(req.Topic, req.Limit)
	return nil
}

// ReadRequest selects one article.
type ReadRequest struct {
	ID string `json:"id" required:"true" description:"Article id (from Headlines) or article URL"`
}

// ReadResponse is the full article text.
type ReadResponse struct {
	Text string `json:"text" description:"Title, source, summary and body of the article"`
}

// SearchRequest searches indexed and live news for a user topic.
type SearchRequest struct {
	Query string `json:"query" required:"true" description:"Search terms, e.g. latest AI news"`
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
	Card:        service.Timed(func() (string, time.Time) { return Headlines(), CardAt() }),
	Endpoints: map[string]service.Endpoint{
		"List":   {Aliases: []string{"news", "news_headlines"}, Doc: "Read recent news headlines with short summaries, balanced across topics"},
		"Read":   {Doc: "Read one news article in full by its id or URL"},
		"Search": {Doc: "Search indexed and live news for a topic", Cost: quota.OpNewsSearch},
	},
}
