package mail

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/service"
)

// Server is the go-micro service handler for mail.
type Server struct{}

// SearchRequest searches an account's mail.
type SearchRequest struct {
	Query string `json:"query" description:"What to look for"`
	Limit int    `json:"limit" description:"Max results (default 10)"`
}

// SearchResponse is a model-ready list of matching messages.
type SearchResponse struct {
	Text string `json:"text" description:"Matching messages: subject, sender, snippet and id"`
}

// Search searches the account's mail and returns the matching messages. With an
// empty query it falls back to listing the most recent inbox messages, so a bare
// "read my mail" still works.
// @example {"query": "invoice"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	account := service.AccountFrom(ctx)
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	if strings.TrimSpace(req.Query) == "" {
		rsp.Text = renderInbox(ListMessages(account, limit))
		return nil
	}
	msgs := Search(account, req.Query, limit)
	if len(msgs) == 0 {
		rsp.Text = fmt.Sprintf("No mail found for %q.", req.Query)
		return nil
	}
	rsp.Text = fmt.Sprintf("Mail matching %q:\n", req.Query) + renderMessages(msgs)
	return nil
}

// InboxRequest lists the account's recent inbox messages.
type InboxRequest struct {
	Limit int `json:"limit" description:"Max messages (default 10)"`
}

// Inbox lists the account's most recent messages without needing a search query.
// Use this for "read my mail", "check my inbox" or "any new email?".
// @example {}
func (Server) Inbox(ctx context.Context, req *InboxRequest, rsp *SearchResponse) error {
	rsp.Text = renderInbox(ListMessages(service.AccountFrom(ctx), req.Limit))
	return nil
}

// renderInbox formats an inbox listing (or a friendly empty message).
func renderInbox(msgs []*Message) string {
	if len(msgs) == 0 {
		return "Your inbox is empty — no messages."
	}
	unread := 0
	for _, m := range msgs {
		if !m.Read {
			unread++
		}
	}
	header := fmt.Sprintf("Your inbox (%d message%s", len(msgs), plural(len(msgs)))
	if unread > 0 {
		header += fmt.Sprintf(", %d unread", unread)
	}
	header += "):\n"
	return header + renderMessages(msgs)
}

// renderMessages formats a list of messages as model-ready lines.
func renderMessages(msgs []*Message) string {
	var b strings.Builder
	for _, m := range msgs {
		status := ""
		if !m.Read {
			status = " (unread)"
		}
		fmt.Fprintf(&b, "- %s — from %s%s: %s (id: %s)\n",
			clip(m.Subject, 100), clip(m.From, 60), status, clip(m.Body, 140), m.ID)
	}
	return b.String()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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

var Spec = service.Spec{
	Name:        "mail",
	Handler:     new(Server),
	Description: "Private messaging and email, with an SMTP server and DKIM",
	Page:        "/mail",
	Scoped:      true,
	Icon:        "mail.png",
	Endpoints: map[string]service.Endpoint{
		"Inbox":  {Doc: "List the account's most recent messages — read my mail, check my inbox"},
		"Search": {Doc: "Search the account's mail and return matching messages"},
	},
}
