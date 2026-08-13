// Package web is the open web as a capability: search it, and read a page from
// it. Two jobs, one domain — the same way news is Headlines + Read + Search and
// places is Search + Nearby + Geocode. A service groups by what it is about,
// not by how many things it does.
//
// This is where web search lives too, rather than in a service of its own
// called "search". Every other service is named for a thing — news, mail,
// markets, places — and "search" was named for an action, which is why its one
// method had to be called Search: service.Method degenerated to search.Search,
// and the tool name to search_search. Naming the domain fixes it at the source
// and gives web_search alongside web_fetch, matching the routes (/web/fetch,
// /web/read) that already said "web".
//
// Headless, like index: the /search page is a surface over this service, not a
// service of its own.
package web

import (
	"context"

	"mu/internal/app"
	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the service handler. Its methods are exposed as RPC endpoints and,
// through the agent and gateways, as AI tools.
type Server struct{}

// SearchRequest is a web search query.
type SearchRequest struct {
	Query string `json:"query" required:"true" description:"Search query"`
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

// FetchRequest names the page to read.
type FetchRequest struct {
	URL string `json:"url" required:"true" description:"The URL to fetch"`
}

// FetchResponse is the cleaned readable page content.
type FetchResponse struct {
	Title   string `json:"title"`
	Content string `json:"content" description:"Cleaned readable content (ads/nav stripped)"`
}

// Fetch fetches a web page and returns its cleaned readable content.
//
// The underlying fetch validates the raw URL and then re-validates the
// resolved addresses, so a hostname that resolves to a private range is
// rejected even if it looked public — the order that matters against DNS
// rebinding. That guard is why this is safe to expose to other services.
//
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

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("web", "service register failed: %v", err)
	}
}

var Spec = service.Spec{
	Name:        "web",
	Handler:     new(Server),
	Description: "The open web: search it, read a page from it",
	Page:        "/search",
	Label:       "Search",
	Icon:        "search.svg",
	Endpoints: map[string]service.Endpoint{
		"Fetch": {
			Aliases: []string{"search_fetch"},
			Doc: "Fetch a web page by URL and return its cleaned readable content, " +
				"stripping ads, popups and navigation. Needs an account",
			Cost: quota.OpWebFetch,
			// Costs nothing to run — an http.Get and a readability pass in
			// this process — but fetching any URL a caller names is a request
			// this server makes on their behalf, to wherever they say. That
			// wants somebody accountable behind it, which is a different
			// question from what it costs.
			//
			// Account rather than AccountOnly, now that a wallet can prove
			// itself by signing. What this needs is somebody to hold
			// responsible and to ration, and a key that has identified itself
			// is exactly that: durable, unforgeable, and the same caller
			// tomorrow. Requiring a signup as well was refusing the accountable
			// agent along with the anonymous one.
			Account: true,
		},
		"Search": {Aliases: []string{"search_web"}, Doc: "Search the web for current information and news", Cost: quota.OpWebSearch},
	},
}
