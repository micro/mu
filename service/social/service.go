package social

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/data"
	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for social.
type Server struct{}

// ListRequest controls how many posts to return.
type ListRequest struct {
	Limit int `json:"limit" description:"Optional max number of posts (default all recent)"`
}

// ListResponse is a model-ready social feed.
type ListResponse struct {
	Text string `json:"text" description:"Latest social posts from the network"`
}

// List returns the latest social posts from the network.
// @example {}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	rsp.Text = FeedText(req.Limit)
	return nil
}

var Spec = service.Spec{
	Name:        "social",
	Handler:     new(Server),
	Description: "Public threads, replies and status",
	Page:        "/social",
	Icon:        "social.svg",
	Card:        CardHTML,
	Endpoints: map[string]service.Endpoint{
		"List": {Aliases: []string{"social"}, Doc: "Read the latest social posts from the network"},
		"Search": {
			Doc: "Search public posts on this instance by keyword. This is the instance's own feed, not the wider internet — use web_search for that",
			// The index is this instance's, so the search costs a model call
			// rather than a vendor lookup; the price is on the operation.
			Cost:  quota.OpSocialSearch,
			Needs: service.Caller,
		},
	},
}

// ── Search ──────────────────────────────────────────────────────

// Searching is over this instance's own index, which is why it is here and not
// in web: these are the posts people wrote on this server. It was a tool with
// no service behind it, pointed at POST /social — so an agent searching the
// feed went through the page that renders it.

type SearchRequest struct {
	Query string `json:"query" required:"true" description:"What to look for in posts on this instance"`
	Limit int    `json:"limit" description:"Max results (default 20)"`
}

type SearchResponse struct {
	Text string `json:"text" description:"Matching posts with their author and time"`
}

// Search finds public posts on this instance by keyword. This is the
// instance's own feed, not the wider internet — use web_search for that.
// @example {"query": "go generics"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to search")
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return fmt.Errorf("query is required")
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var b strings.Builder
	n := 0
	for _, entry := range data.Search(q, 50) {
		if entry.Type != "social" {
			continue
		}
		if n >= limit {
			break
		}
		n++
		b.WriteString("- " + strings.TrimSpace(entry.Title) + "\n")
		if c := strings.TrimSpace(entry.Content); c != "" {
			b.WriteString("  " + c + "\n")
		}
	}
	if n == 0 {
		rsp.Text = "No posts here match " + q + "."
		return nil
	}
	rsp.Text = fmt.Sprintf("Posts matching %s:\n%s", q, b.String())
	return nil
}
