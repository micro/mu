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
	Name:    "mail",
	Handler: new(Server),
	// What it is to whoever is reading, which is almost never an operator.
	//
	// It said "Private messaging and email, with an SMTP server and DKIM". The
	// half that matters is the first three words; the rest names machinery that
	// somebody running this for themselves will never configure and does not
	// need to know about to use the thing. An SMTP server is how one capability
	// is delivered, not what the service is — that belongs in the install guide,
	// where an operator goes looking for it.
	Description: "Private messages, and an inbox each of your agents can be reached at",
	Page:        "/mail",
	Scoped:      true,
	Icon:        "mail.png",
	Endpoints: map[string]service.Endpoint{
		"Inbox":  {Aliases: []string{"mail_read"}, Doc: "List the account's most recent messages — read my mail, check my inbox"},
		"Search": {Doc: "Search the account's mail and return matching messages"},
		// Aliased to the name it had, because an agent that learned mail_address
		// last week should not find it gone. What it returns changed shape, and
		// the name had to follow: an instance with no mail domain has no address
		// to give, only a handle.
		"Info": {Aliases: []string{"mail_address"}, Doc: "How to reach the caller here: the handle, " +
			"the email address if this instance has a mail domain, and whether mail from outside " +
			"can arrive at all. Pass a tag for a separate one (you+tag) — give that out, then read " +
			"only its messages with mail_inbox(tag)"},
		// One way to write to a person, whoever they are.
		//
		// This was two endpoints, Send and Email, and the argument for splitting
		// them was that a price depending on an argument cannot be shown in the
		// catalogue. That was true and it was not worth what it cost: a caller
		// had to know whether the recipient happened to hold an account here
		// before choosing which tool to call, which is a fact about our database
		// that has nothing to do with writing to somebody. Three ways to send a
		// message — mail_send, mail_email, email_send — and the first two
		// differed by a lookup the caller could not perform.
		//
		// So it routes, and charges itself for what it actually did. sms_send
		// already does exactly this because a text is priced per segment; a
		// price settled at run time is a solved problem here, and it was never
		// the reason to make a caller do our routing for us.
		//
		// mail_email stays as an alias, because it named a real distinction —
		// mail that leaves — and an agent that learned it should not find it
		// gone. It resolves to the same call.
		"Send": {
			Aliases: []string{"mail_email"},
			Doc: "Write to somebody, as you. A username reaches them on this instance and is " +
				"free; a full email address leaves over SMTP under this instance's domain, so a " +
				"reply comes back to your inbox, and is charged. Resolve a name with " +
				"contacts_find first. Mail that leaves needs a mail domain configured — without " +
				"one use email_send, which sends from this instance's own sending domain and " +
				"expects no reply",
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

// Send writes to somebody, wherever they are.
//
// The recipient decides the route, which is the thing a caller should not have
// had to decide: a username or a local address stays here and is free, anything
// else leaves over SMTP under this instance's domain and is charged. Asking an
// agent to pick the tool meant asking it to know whether a stranger held an
// account here, which it cannot find out and should not care about.
//
// Mail that leaves requires a mail domain. Without one there is no address to
// send it from, and the answer says where to go instead rather than failing
// somewhere in the transport — email_send exists precisely for the instance
// that has no mail server.
//
// @example {"to": "asim", "subject": "Hello", "body": "..."}
func (Server) Send(ctx context.Context, req *SendRequest, rsp *SendResponse) error {
	acc, err := sender(ctx, req)
	if err != nil {
		return err
	}
	to := strings.TrimSpace(req.To)

	// A local recipient may be a bare username or a full local address, with
	// or without a +tag: asim, asim@micro.mu and asim+claude@micro.mu all
	// reach the same inbox.
	if !IsExternalEmail(to) {
		toAcc, err := auth.GetAccount(LocalRecipient(to))
		if err != nil {
			return fmt.Errorf("no account here called %q", to)
		}
		if err := charge(acc.ID, quota.OpMailSend); err != nil {
			return err
		}
		if err := SendMessage(acc.Name, acc.ID, toAcc.Name, toAcc.ID, req.Subject, req.Body, "", ""); err != nil {
			return fmt.Errorf("failed to send: %w", err)
		}
		rsp.Result = "Sent to " + toAcc.Name + " on this instance."
		return nil
	}

	messageID, err := SendOut(acc.ID, acc.Name, to, req.Subject, req.Body, "", "")
	if err != nil {
		return err
	}
	// Kept in Sent whether or not the copy succeeds: the mail has gone.
	if err := SendMessage(acc.Name, acc.ID, to, to, req.Subject, req.Body, "", messageID); err != nil {
		app.Log("mail", "sent copy not stored: %v", err)
	}
	rsp.Result = "Sent to " + to + "."
	return nil
}

// charge takes the price of whichever route Send took.
//
// The service charges rather than the gateway, because the gateway applies one
// flat Cost per endpoint and this endpoint has two prices. sms_send does the
// same for the same reason — a text is priced per segment — so a price settled
// at run time is a shape this codebase already has rather than a reason to
// split an endpoint in two.
//
// Before the send, unlike the gateway, which charges after. Mail that has left
// cannot be recalled if the charge then fails, and refusing up front is the
// only point at which "there is not enough on this account" is still true.
func charge(owner, op string) error {
	if !quota.Metered(op) {
		return nil
	}
	ok, _, price, err := quota.CheckQuota(owner, op)
	if err != nil {
		return err
	}
	if price == 0 {
		return nil
	}
	if !ok || quota.BalanceOf(owner) < price {
		return fmt.Errorf("sending that costs %d credits and there are not enough on this account", price)
	}
	return quota.ConsumeQuota(owner, op)
}

// sender resolves the caller and checks the message is complete.
func sender(ctx context.Context, req *SendRequest) (*auth.Account, error) {
	who := service.AccountFrom(ctx)
	if who == "" {
		return nil, fmt.Errorf("sign in to send mail")
	}
	acc, err := auth.GetAccount(who)
	if err != nil {
		return nil, fmt.Errorf("account not found")
	}
	if strings.TrimSpace(req.To) == "" || strings.TrimSpace(req.Subject) == "" ||
		strings.TrimSpace(req.Body) == "" {
		return nil, fmt.Errorf("to, subject and body are all required")
	}
	return acc, nil
}

// ── Address ─────────────────────────────────────────────────────

// ── Info ────────────────────────────────────────────────────────

// How mail reaches you here, and how far it goes.
//
// It was Address, and it answered with one string. That was right while every
// instance had a mail domain and wrong the moment one did not: an address is
// not what this service always has. A handle is. Whether that handle is also an
// email address depends on something an operator did or did not do, and a
// caller that cannot see which is which will hand out an address that reaches
// nobody — which is exactly what the old tool did, since it appended a domain
// of "localhost" rather than admitting there was none.
//
// So it answers the whole question instead of one field of it: what to give
// somebody, whether outside mail can arrive, and what to do next. The same
// shape sms_number has, for the same reason — the useful answer to "what is my
// number" includes what may be done with it.

type InfoRequest struct {
	Tag string `json:"tag" description:"A label for this handle, e.g. \"research\" or \"receipts\". Omit for the plain one"`
}

type InfoResponse struct {
	Handle   string `json:"handle" description:"What the caller is called here — asim, or asim+research. Always answers; messaging on this instance needs nothing configured"`
	Address  string `json:"address,omitempty" description:"The same handle as an email address, when this instance has a mail domain. Empty when it does not, and then nothing outside can write to it"`
	External bool   `json:"external" description:"Whether mail from outside this instance can arrive at all"`
	Domain   string `json:"domain,omitempty" description:"The mail domain, when there is one"`
	Reach    string `json:"reach" description:"Give this out — the address where there is one, the handle where there is not"`
	Text     string `json:"text" description:"The same, as a sentence"`
}

// Info says how to reach the caller here.
//
// With a tag it answers a plus-handle — you+tag — which is the whole point:
// give one out per correspondent or per purpose, and mail_inbox(tag) reads only
// that stream. Nothing has to be configured first; it works because the account
// exists.
// @example {"tag": "receipts"}
func (Server) Info(ctx context.Context, req *InfoRequest, rsp *InfoResponse) error {
	who := service.AccountFrom(ctx)
	if who == "" {
		return fmt.Errorf("sign in to have an address")
	}
	if _, err := auth.GetAccount(who); err != nil {
		return fmt.Errorf("account not found")
	}
	tag := strings.TrimSpace(req.Tag)
	rsp.Handle = Handle(who, tag)
	rsp.External, rsp.Domain = Reachable(), Domain()
	if rsp.External {
		rsp.Address = rsp.Handle + "@" + rsp.Domain
	}
	rsp.Reach = AliasFor(who, tag)

	switch {
	case !rsp.External && tag == "":
		rsp.Text = rsp.Handle + " — messages from anyone on this instance reach you here. " +
			"This instance has no mail domain, so nothing from outside can. " +
			"To send an email out, use email_send."
	case !rsp.External:
		rsp.Text = rsp.Handle + " — give this out here, then read only its messages with " +
			"mail_inbox(tag: \"" + tag + "\"). This instance has no mail domain, so nothing " +
			"from outside can reach it."
	case tag == "":
		rsp.Text = rsp.Address + " — mail sent here reaches you. Pass a tag for a separate " +
			"address you can read on its own."
	default:
		rsp.Text = rsp.Address + " — give this out, then read only its mail with " +
			"mail_inbox(tag: \"" + tag + "\")."
	}
	return nil
}
