package news

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for news. Its methods are exposed as
// RPC endpoints and, through the agent and gateways, as AI tools.
type Server struct{}

// HeadlinesRequest selects the current home card without filters.
type HeadlinesRequest struct{}

// HeadlinesResponse is the same headline selection shown on the home card.
type HeadlinesResponse struct {
	Text  string     `json:"text" description:"The home card headlines with topics and article URLs"`
	Items []Headline `json:"items" description:"Latest story per topic, freshest first, at most ten, as on the home card"`
}

// Headlines returns the home card's current stories as data.
// @example {}
func (Server) Headlines(_ context.Context, _ *HeadlinesRequest, rsp *HeadlinesResponse) error {
	posts := cardPosts(GetFeed())
	var text strings.Builder
	rsp.Items = make([]Headline, 0, len(posts))
	for _, p := range posts {
		rsp.Items = append(rsp.Items, Headline{
			Title: p.Title, URL: p.URL, Category: p.Category, Description: p.Description,
		})
		fmt.Fprintf(&text, "[%s] %s\n", p.Category, p.Title)
		if p.Description != "" {
			fmt.Fprintln(&text, p.Description)
		}
		if p.URL != "" {
			fmt.Fprintln(&text, p.URL)
		}
		fmt.Fprintln(&text)
	}
	rsp.Text = strings.TrimSpace(text.String())
	if len(posts) == 0 {
		rsp.Text = "No news headlines available right now."
	}
	return nil
}

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
	// What is happening, in front of the question rather than one round trip
	// behind it — see service.Spec.Now and Now, above.
	Now: Now,
	Endpoints: map[string]service.Endpoint{
		"Headlines": {Doc: "Read the home card headlines: latest story per topic, freshest first, at most ten"},
		"List":      {Aliases: []string{"news"}, Doc: "Read recent news headlines with short summaries, balanced across topics"},
		"Read":      {Doc: "Read one news article in full by its id or URL"},
		"Search":    {Doc: "Search indexed and live news for a topic", Cost: quota.OpNewsSearch},
	},
}
