// Package archive is everything this instance has collected, searchable as one
// thing.
//
// Six services write to the same index — news, video, markets, blog, prayer,
// social — and have done for a long time. Every reader over it was filtered to
// one type: news_search passes WithType("news"), the social page searches its
// own, the chat pulls a handful for context. So the archive existed, it was
// large, and there was no way to ask it a question that crossed a service. An
// agent could ask what the news said about a company and could not ask what
// *this instance knows* about one — which is the same question, and the second
// phrasing is the one somebody actually has.
//
// # This is a reader, not a store
//
// Nothing here archives anything. The services do that as they go, which is
// right — news knows when an article is worth keeping and this does not. The
// test is the same one recall passes: delete this package and nothing stops
// being archived, nothing stops working, and what goes is the ability to ask
// across the whole of it on purpose.
//
// That is also why it is a service rather than substrate. Searching the archive
// is a decision a caller takes, in the same way searching the web is; writing
// to it is not a decision at all.
//
// # Public only
//
// An index entry with an owner is somebody's private record and is never
// returned here, whoever is asking. The archive is what this instance has
// collected about the world, not about its users — service/recall is the other
// question and it is scoped to the caller.
//
// # Named for the thing, not the act
//
// "search" would have derived search_search, which is how service/web got its
// name after service/search proved the point. The archive is the noun; searching
// it is what you do to it.
package archive

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"mu/internal/app"
	"mu/internal/data"
	"mu/internal/service"
)

// Server is the go-micro handler. Its exported methods become the archive_*
// tools.
type Server struct{}

// shown bounds a result set. Each entry can carry a whole article, so a
// generous limit is a context window spent on the parts nobody asked about.
const shown = 20

// ── Search ──────────────────────────────────────────────────────

type SearchRequest struct {
	Query string `json:"query" required:"true" description:"What to look for"`
	Kind  string `json:"kind" description:"Narrow to one kind: news, video, market, blog, prayer. Omit to search everything"`
	Limit int    `json:"limit" description:"Max results (default 20, max 100)"`
}

type SearchResponse struct {
	Text string `json:"text" description:"Matching entries, best first: what each is, when it was collected, and what it says"`
}

// Search looks through everything this instance has collected.
// @example {"query": "interest rates"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return fmt.Errorf("query is required — say what to look for")
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = shown
	}

	var opts []data.SearchOption
	if kind := strings.ToLower(strings.TrimSpace(req.Kind)); kind != "" {
		opts = append(opts, data.WithType(kind))
	}

	entries := data.Search(query, limit, opts...)
	if len(entries) == 0 {
		rsp.Text = fmt.Sprintf("Nothing in the archive mentions %q.", query)
		return nil
	}
	rsp.Text = render(entries)
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Kind  string `json:"kind" description:"One kind: news, video, market, blog, prayer. Omit for a summary of what is here"`
	Limit int    `json:"limit" description:"Max entries (default 20, max 100)"`
}

type ListResponse struct {
	Text string `json:"text" description:"The most recent entries of that kind, or what kinds exist and how much of each"`
}

// List is the most recent of one kind, or a summary of what is here.
//
// Two answers from one method because they are the same question asked with and
// without an argument: "what is in the archive" and "what is in the archive of
// this kind". Splitting them would put a tool in the catalogue whose whole job
// is to name the argument of the tool beside it.
// @example {"kind": "video"}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind == "" {
		kinds := data.Kinds()
		if len(kinds) == 0 {
			rsp.Text = "The archive is empty."
			return nil
		}
		var b strings.Builder
		total := 0
		for _, k := range kinds {
			total += k.Count
			b.WriteString(fmt.Sprintf("- %s: %d\n", k.Name, k.Count))
		}
		rsp.Text = fmt.Sprintf("%d entries in the archive.\n\n%s", total, b.String())
		return nil
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = shown
	}
	entries := data.ByType(kind, limit)
	if len(entries) == 0 {
		rsp.Text = fmt.Sprintf("Nothing of kind %q is archived. archive_list with no kind says what is.", kind)
		return nil
	}
	rsp.Text = render(entries)
	return nil
}

// render is a result set as prose an agent can read.
//
// Trimmed, because an entry may hold a whole article and twenty of those is a
// context window spent on the parts nobody asked about. The id is on every line
// so a caller that wants the whole of one can go and get it from the service
// that owns it.
func render(entries []*data.IndexEntry) string {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].IndexedAt.After(entries[j].IndexedAt)
	})
	var b strings.Builder
	for _, e := range entries {
		title := strings.TrimSpace(e.Title)
		if title == "" {
			title = "Untitled"
		}
		b.WriteString(fmt.Sprintf("%s · %s\n%s\n", e.Type,
			e.IndexedAt.Format("2 Jan 2006"), title))
		if body := trim(e.Content, 300); body != "" {
			b.WriteString(body + "\n")
		}
		b.WriteString("  id: " + e.ID + "\n\n")
	}
	return strings.TrimSpace(b.String())
}

// trim flattens an entry to one readable paragraph.
func trim(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("archive", "service register failed: %v", err)
	}
}

// Nothing is charged. Reading this instance's own store touches no model and no
// third party, which is the whole of the free/paid rule in quota.json.
//
// Not Scoped: the archive is what this instance has collected about the world,
// and an entry that belongs to somebody is never returned — see the package
// comment.
var Spec = service.Spec{
	Name:        "archive",
	Handler:     new(Server),
	Description: "Everything this instance has collected — news, video, markets, posts — searchable as one thing",
	Page:        "/archive",
	Icon:        "archive.svg",
	Endpoints: map[string]service.Endpoint{
		"Search": {Aliases: []string{"archive_find"},
			Doc: "Search everything this instance has collected, across news, video, markets and posts at once. Use it when the question crosses a service, or when you do not know which service would hold the answer; news_search is narrower and better when you know it is news"},
		"List": {Aliases: []string{"archive_kinds", "archive_recent"},
			Doc: "With a kind, the most recent entries of it. Without one, what kinds are archived and how much of each"},
	},
}
