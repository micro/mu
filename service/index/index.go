// Package index searches everything Mu holds for a caller: it merges the
// public indexed corpus (news, blog, social, video) with the caller's own mail
// into a compact, model-ready list.
//
// Public content is searched through the shared index without an owner scope
// (private entries excluded by default). Mail is scanned in memory and strictly
// scoped to the account, so nothing leaks across users and mail bodies never
// need to live in the shared index in plaintext. Both halves are at-rest data —
// they differ in scope and storage, not in whether they are indexed.
package index

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/data"
	"mu/internal/service"
	"mu/service/mail"
)

// Server is the go-micro service handler for cross-source recall.
type Server struct{}

// Request searches everything mu knows for an account.
type Request struct {
	Query string `json:"query" required:"true" description:"What to look for"`
	Limit int    `json:"limit" description:"Max results (default 12)"`
}

// Response is a model-ready list of matches.
type Response struct {
	Text string `json:"text" description:"Most relevant items with ids"`
}

// Search searches indexed news, blog, social and video, plus the account's own
// mail, and returns the most relevant items with ids.
// @example {"query": "bitcoin"}
func (Server) Search(ctx context.Context, req *Request, rsp *Response) error {
	limit := req.Limit
	if limit <= 0 {
		limit = 12
	}
	rsp.Text = search(service.AccountFrom(ctx), req.Query, limit)
	return nil
}

func search(accountID, query string, limit int) string {
	// WithOwner adds this account's private entries; without it the index
	// returns public content only, which is the safe default and was also the
	// only thing this ever returned. So "search across the caller's own
	// content" found everybody's public content and none of the caller's own —
	// the one thing its name promises — and the omission was invisible, because
	// an empty result from a search reads as "nothing matched".
	var opts []data.SearchOption
	if accountID != "" {
		opts = append(opts, data.WithOwner(accountID))
	}
	pub := data.Search(query, limit, opts...)

	var mails []*mail.Message
	if accountID != "" {
		mails = mail.Search(accountID, query, 6)
	}

	if len(pub) == 0 && len(mails) == 0 {
		return fmt.Sprintf("No matches found for %q.", query)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Results for %q:\n\n", query)
	for _, m := range mails {
		fmt.Fprintf(&b, "[mail] %s — from %s: %s (id: %s)\n",
			firstLine(m.Subject, 120), firstLine(m.From, 60), snippet(m.Body, 160), m.ID)
	}
	for _, e := range pub {
		t := e.Type
		if t == "post" {
			t = "blog"
		}
		fmt.Fprintf(&b, "[%s] %s — %s (id: %s)\n", t, firstLine(e.Title, 120), snippet(e.Content, 160), e.ID)
	}
	return b.String()
}

// snippet strips tags, collapses whitespace and truncates to max runes.
func snippet(s string, max int) string {
	s = strings.Join(strings.Fields(stripTags(s)), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// firstLine trims to the first line and truncates to max runes.
func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// stripTags removes HTML tags without pulling in a dependency.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// Not Scoped: a guest may search, they just get less. Search adds the caller's
// own entries and their mail only when there is a caller, so an unauthenticated
// search returns public indexed content and nothing else.
//
// This was true of the service and false of the tool for a long time.
// index_search was registered with RegisterToolWithAuth, which refuses anyone
// without an account, so guests were closed out — the "two lists, two answers"
// this comment warned about, happening to the comment. The tool takes optional
// auth now, which is one mechanism giving both answers.
var Spec = service.Spec{
	Name:        "index",
	Icon:        "index.svg",
	Handler:     Server{},
	Description: "Search across the caller's own content",
	Endpoints: map[string]service.Endpoint{
		"Search": {
			Aliases: []string{"index", "recall", "search"},
			Doc: "Search across everything this instance knows — indexed news, blog, social and video, plus the caller's own mail — and return what matches. " +
				"Free, and the first thing to reach for before searching the web",
			// Answers a guest with the public half and a signed-in caller with
			// their own content on top. Refusing guests would be the easy
			// reading and the wrong one.
			OptionalAuth: true,
		},
	},
}
