package video

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"mu/internal/quota"
	"mu/internal/service"
)

// Server is the go-micro service handler for video.
type Server struct{}

// ListRequest controls how many videos to return.
type ListRequest struct {
	Category string `json:"category" description:"Optional curated category"`
	Offset   int    `json:"offset" description:"Videos to skip"`
	Limit    int    `json:"limit" description:"Maximum videos per page, default 20, maximum 100"`
}

// ListResponse is a model-ready video list.
type ListResponse struct {
	Items      []*Result `json:"items"`
	Total      int       `json:"total"`
	Offset     int       `json:"offset"`
	NextOffset *int      `json:"next_offset,omitempty"`
	Text       string    `json:"text" description:"Latest videos from curated channels"`
}

// List returns the latest videos from curated channels.
// @example {}
func (Server) List(_ context.Context, req *ListRequest, rsp *ListResponse) error {
	seen := map[string]bool{}
	items := []*Result{}
	cached := LatestVideos(0)
	if len(cached) == 0 {
		rsp.Items = items
		rsp.Text = LatestText(req.Limit)
		return nil
	}
	for _, v := range cached {
		if v == nil || seen[v.ID] || req.Category != "" && v.Category != req.Category {
			continue
		}
		seen[v.ID] = true
		cp := *v
		items = append(items, &cp)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Published.Equal(items[j].Published) {
			return items[i].ID < items[j].ID
		}
		return items[i].Published.After(items[j].Published)
	})
	start, end := service.PageRange(len(items), req.Offset, req.Limit)
	rsp.Items, rsp.Total, rsp.Offset = items[start:end], len(items), start
	if end < len(items) {
		rsp.NextOffset = &end
	}
	for _, v := range rsp.Items {
		rsp.Text += fmt.Sprintf("- %s (%s) https://youtube.com/watch?v=%s\n", v.Title, v.Channel, v.ID)
	}
	if len(rsp.Items) == 0 {
		rsp.Text = "No videos match this category or page."
	}
	return nil
}

// ── Search ──────────────────────────────────────────────────────

// SearchRequest asks YouTube, through this instance's key.
type SearchRequest struct {
	Query string `json:"query" required:"true" description:"What to search for"`
}

// SearchResponse is a model-ready list of matches.
type SearchResponse struct {
	Items []*Result `json:"items"`
	Total int       `json:"total"`
	Text  string    `json:"text" description:"Matching videos: title, channel and link"`
}

// Search looks for videos by keyword.
//
// Free, and account-only anyway: the YouTube quota is 10,000 units a day
// across everyone and a search costs 100, so it is rationed per account by
// allowSearch. There is nothing to ration an anonymous caller by.
// @example {"query": "go concurrency"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to search video")
	}
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return fmt.Errorf("query is required")
	}
	if len(q) > 256 {
		return fmt.Errorf("search query must not exceed 256 characters")
	}
	if err := allowSearch(who); err != nil {
		return err
	}

	_, results, err := getResults(q, "")
	if err != nil {
		return err
	}
	rsp.Items = results
	rsp.Total = len(results)
	if len(results) == 0 {
		rsp.Text = "No videos found for " + q + "."
		return nil
	}
	var b strings.Builder
	b.WriteString("Videos matching " + q + ":\n")
	for _, v := range results {
		if v == nil {
			continue
		}
		b.WriteString("- " + v.Title)
		if v.Channel != "" {
			b.WriteString(" (" + v.Channel + ")")
		}
		if v.ID != "" {
			b.WriteString(" https://youtube.com/watch?v=" + v.ID)
		}
		b.WriteString("\n")
	}
	rsp.Text = b.String()
	return nil
}

var Spec = service.Spec{
	Name:        "video",
	Handler:     new(Server),
	Description: "Video from curated channels, without ads or recommendations",
	Page:        "/video",
	Icon:        "video.png",
	Card:        service.Timed(func() (string, time.Time) { return Latest(), CardAt() }),
	Endpoints: map[string]service.Endpoint{
		"Read": {Doc: "Read the metadata and description of one video already discovered by this instance. No transcript is available"},
		"List": {Aliases: []string{"video"}, Doc: "Read the latest videos from curated channels"},
		"Search": {
			Doc: "Search YouTube by keyword using this instance's search quota. Results are not restricted to the curated channels",
			// Priced at zero and still not for strangers: it spends this
			// instance's YouTube quota, which is shared and cannot be topped
			// up per caller. Rationing needs somebody to ration — but a wallet
			// that has signed is somebody, so Account rather than AccountOnly.
			Cost:  quota.OpVideoSearch,
			Needs: service.Caller,
		},
	},
}
