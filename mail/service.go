package mail

import (
	"context"
	"fmt"
	"strings"
)

// Server is the go-micro service handler for mail.
type Server struct{}

// SearchRequest searches an account's mail.
type SearchRequest struct {
	AccountID string `json:"account_id" description:"Account whose mail to search"`
	Query     string `json:"query" description:"What to look for"`
	Limit     int    `json:"limit" description:"Max results (default 10)"`
}

// SearchResponse is a model-ready list of matching messages.
type SearchResponse struct {
	Text string `json:"text" description:"Matching messages: subject, sender, snippet and id"`
}

// Search searches the account's mail and returns the matching messages.
// @example {"query": "invoice"}
func (Server) Search(_ context.Context, req *SearchRequest, rsp *SearchResponse) error {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	msgs := Search(req.AccountID, req.Query, limit)
	if len(msgs) == 0 {
		rsp.Text = fmt.Sprintf("No mail found for %q.", req.Query)
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Mail matching %q:\n", req.Query)
	for _, m := range msgs {
		status := ""
		if !m.Read {
			status = " (unread)"
		}
		fmt.Fprintf(&b, "- %s — from %s%s: %s (id: %s)\n",
			clip(m.Subject, 100), clip(m.From, 60), status, clip(m.Body, 140), m.ID)
	}
	rsp.Text = b.String()
	return nil
}

// clip returns the first line of s, rune-safely truncated to n runes.
func clip(s string, n int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
