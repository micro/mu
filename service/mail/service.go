package mail

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
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
		"Inbox":   {Aliases: []string{"mail_read"}, Doc: "List the account's most recent messages — read my mail, check my inbox"},
		"Search":  {Doc: "Search the account's mail and return matching messages"},
		"Address": {Aliases: []string{"mail_address"}, Doc: "Get an email address that reaches the caller. Pass a tag for a plus-address (you+tag@) — give that out, then read only its mail with mail_inbox(tag)"},
		"Send": {
			Doc: "Send an email from the caller's own address on this instance. Takes a recipient address, a subject and a body; resolve a name to an address with contacts_find first. The mail really is delivered — there is no draft state to undo from",
			// A funded wallet is not accountable for this sending domain, so
			// paying cannot stand in for having an account. The price is for
			// external delivery, and it is charged inside Send once the
			// recipient is known — a local message costs nothing.
			Cost:        quota.OpExternalEmail,
			AccountOnly: true,
			Destructive: true,
		},
	},
}

// ── Send ────────────────────────────────────────────────────────

// Sending was a tool with no service behind it, pointed at POST /mail — so an
// agent sending mail went through the page's form handler, and the two paths
// that decide what a message costs and where it goes lived in a web handler
// rather than anywhere a service could reach.
//
// The rule it keeps: a local recipient is free and stays on this instance; an
// address with a domain leaves over SMTP and is charged. Admins are not
// charged either way. The charge happens after the send succeeds, so a failure
// never bills.

type SendRequest struct {
	To      string `json:"to" required:"true" description:"Recipient: a username on this instance, or a full email address. Resolve a name with contacts_find first"`
	Subject string `json:"subject" required:"true" description:"Message subject"`
	Body    string `json:"body" required:"true" description:"Message body, plain text"`
}

type SendResponse struct {
	Result string `json:"result" description:"Confirmation, saying whether it went out over SMTP or stayed on this instance"`
}

// Send sends mail from the caller's own address on this instance.
//
// The mail really is delivered — there is no draft state to undo from.
// @example {"to": "sarah@example.com", "subject": "Hello", "body": "..."}
func (Server) Send(ctx context.Context, req *SendRequest, rsp *SendResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to send mail")
	}
	acc, err := auth.GetAccount(who)
	if err != nil {
		return fmt.Errorf("account not found")
	}
	to := strings.TrimSpace(req.To)
	if to == "" || strings.TrimSpace(req.Subject) == "" || strings.TrimSpace(req.Body) == "" {
		return fmt.Errorf("to, subject and body are all required")
	}

	if IsExternalEmail(to) {
		if !acc.Admin {
			if ok, _, cost, err := quota.CheckQuota(acc.ID, quota.OpExternalEmail); err != nil || !ok {
				return fmt.Errorf("external email costs %d credits — top up at /wallet/topup", cost)
			}
		}
		from := GetEmailForUser(acc.ID, GetConfiguredDomain())
		messageID, err := SendExternalEmail(acc.Name, from, to, req.Subject, req.Body,
			convertPlainTextToHTML(req.Body), "")
		if err != nil {
			return fmt.Errorf("failed to send: %w", err)
		}
		if !acc.Admin {
			if err := quota.ConsumeQuota(acc.ID, quota.OpExternalEmail); err != nil {
				app.Log("mail", "external email sent but not charged: %v", err)
			}
		}
		// Kept in Sent whether or not the copy succeeds: the mail has gone.
		if err := SendMessage(acc.Name, acc.ID, to, to, req.Subject, req.Body, "", messageID); err != nil {
			app.Log("mail", "sent copy not stored: %v", err)
		}
		rsp.Result = "Sent to " + to + "."
		return nil
	}

	// A local recipient may be a bare username or a full local address, with
	// or without a +tag: asim, asim@micro.mu and asim+claude@micro.mu all
	// reach the same inbox.
	toAcc, err := auth.GetAccount(LocalRecipient(to))
	if err != nil {
		return fmt.Errorf("no account here called %q", to)
	}
	if !acc.Admin {
		if ok, _, cost, _ := quota.CheckQuota(acc.ID, quota.OpMailSend); !ok {
			return fmt.Errorf("sending mail costs %d credits — top up at /wallet/topup", cost)
		}
	}
	if err := SendMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, req.Subject, req.Body, "", ""); err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}
	if !acc.Admin {
		_ = quota.ConsumeQuota(acc.ID, quota.OpMailSend)
	}
	rsp.Result = "Sent to " + toAcc.Name + " on this instance."
	return nil
}

// ── Address ─────────────────────────────────────────────────────

// What your own address is, which the mail service could not answer.
//
// It had Inbox, Search and Send, and no way to ask where mail reaches you —
// so an agent could read and write mail but not tell anyone where to write
// back. It was a tool in the assembly instead.

type AddressRequest struct {
	Tag string `json:"tag" description:"A label for this address, e.g. \"research\" or \"receipts\". Omit for the plain one"`
}

type AddressResponse struct {
	Address string `json:"address" description:"An email address that reaches the caller"`
	Text    string `json:"text" description:"The address, with what to do with it"`
}

// Address returns an email address that reaches the caller.
//
// With a tag it returns a plus-address — you+tag@ — which is the whole point:
// give one out per correspondent or per purpose, and mail_inbox(tag) reads only
// that stream. Nothing has to be configured first; the address works because
// the account exists.
// @example {"tag": "receipts"}
func (Server) Address(ctx context.Context, req *AddressRequest, rsp *AddressResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to have an address")
	}
	if _, err := auth.GetAccount(who); err != nil {
		return fmt.Errorf("account not found")
	}
	addr := AliasFor(who, strings.TrimSpace(req.Tag))
	if addr == "" {
		return fmt.Errorf("this instance has no mail domain configured")
	}
	rsp.Address = addr
	if req.Tag == "" {
		rsp.Text = addr + " — mail sent here reaches you. Pass a tag for a separate address you can read on its own."
		return nil
	}
	rsp.Text = addr + " — give this out, then read only its mail with mail_inbox(tag: \"" + req.Tag + "\")."
	return nil
}
