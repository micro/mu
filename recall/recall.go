// Package recall is the cross-source recall service: it merges the public
// indexed corpus (news, blog, social, video) with the caller's own mail into a
// compact, model-ready list. Public content is searched without an owner scope
// (private entries excluded by default); mail is searched live and strictly
// scoped to the account, so nothing leaks across users and mail bodies never
// need to live in the shared index.
package recall

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/data"
	"mu/mail"
)

// Server is the go-micro service handler for cross-source recall.
type Server struct{}

// Request searches everything mu knows for an account.
type Request struct {
	AccountID string `json:"account_id" description:"Account whose mail to include (optional)"`
	Query     string `json:"query" description:"What to look for"`
	Limit     int    `json:"limit" description:"Max results (default 12)"`
}

// Response is a model-ready list of matches.
type Response struct {
	Text string `json:"text" description:"Most relevant items with ids"`
}

// Search searches indexed news, blog, social and video, plus the account's own
// mail, and returns the most relevant items with ids.
// @example {"query": "bitcoin"}
func (Server) Search(_ context.Context, req *Request, rsp *Response) error {
	limit := req.Limit
	if limit <= 0 {
		limit = 12
	}
	rsp.Text = search(req.AccountID, req.Query, limit)
	return nil
}

func search(accountID, query string, limit int) string {
	pub := data.Search(query, limit)
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
