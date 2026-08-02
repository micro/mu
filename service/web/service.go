// Package web is the read-a-URL capability: fetch a page and return its
// cleaned, readable content.
//
// It is a service in its own right rather than a method on search because the
// two are different jobs — search queries a paid index, web reads a page you
// already have the address of — and because everything else about this
// capability was already called "web": the routes are /web/fetch and
// /web/preview, and the tool has always carried the web_fetch name. Only the
// service disagreed.
//
// Headless, like index: a capability with no page of its own.
package web

import (
	"context"

	"mu/internal/app"
	"mu/internal/service"
	"mu/service/search"
)

// Server is the service handler. Its methods are exposed as RPC endpoints and,
// through the agent and gateways, as AI tools.
type Server struct{}

// FetchRequest names the page to read.
type FetchRequest struct {
	URL string `json:"url" description:"The URL to fetch"`
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
	title, content, err := search.FetchAndExtract(req.URL)
	if err != nil {
		return err
	}
	rsp.Title = title
	rsp.Content = content
	return nil
}

// Load registers the service.
func Load() {
	if err := service.Register("web", new(Server), toolDocs); err != nil {
		app.Log("web", "service register failed: %v", err)
	}
}

var toolDocs = service.Docs{
	"Fetch": "Fetch a web page by URL and return its readable content",
}
