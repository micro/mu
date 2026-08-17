// Package recall is the caller's own past: what was said, on which
// conversation, wherever it was said.
//
// Every client writes to internal/thread on every turn, and that store is
// deliberately not a service — a system of record is not something a caller may
// choose to use, because an agent that forgot to call it would simply stop
// remembering. Reading is the other half and it is a choice: searching your own
// history is a decision an agent takes, in the same way it decides to look
// something up on the web.
//
// The test for whether this is in the right place is that deleting it breaks
// nothing. Clients still record, the agent is still handed the last few turns
// of the conversation it is in, /agent/threads still renders. What goes is the
// ability to go looking on purpose — which is exactly what a service should be.
//
// # Recall and notes are different kinds of remembering
//
// notes is what was written down on purpose: a title, some text, no expiry —
// "remember that I'm in London". Small, deliberate, and the caller chose every
// word of it. This is the transcript: everything anybody said, whether or not
// it turned out to matter, which is where the answer lives when nobody thought
// to write it down at the time.
//
// So the two are asked in different situations and neither replaces the other.
// If an agent knows what it is looking for and expects one fact, that is notes.
// If it is looking for something that was mentioned, this is where it happened.
//
// # Named for what it is
//
// A conversation belongs to the record; recall is the faculty of getting at it.
// The alternative names were worse in the way this repo has been bitten by
// before: "history" is what the agent is already handed each turn and would
// have meant two things, "search" is an action and would have derived
// recall.Search into search_search.
package recall

import (
	"context"
	"fmt"
	"strings"

	"mu/internal/app"
	"mu/internal/service"
	"mu/internal/thread"
)

// Server is the go-micro handler. Its exported methods become the recall_*
// tools.
type Server struct{}

// caller resolves the account from call metadata rather than from a field.
//
// No owner argument anywhere here, and this is the service where that matters
// most: an argument is chosen by whoever makes the call, and everything behind
// this one is somebody's private correspondence.
func caller(ctx context.Context) (string, error) {
	id := service.AccountFrom(ctx)
	if id == "" {
		return "", fmt.Errorf("sign in to search your own history")
	}
	return id, nil
}

// ── Search ──────────────────────────────────────────────────────

type SearchRequest struct {
	Query  string `json:"query" required:"true" description:"What to look for — a word or phrase that was said"`
	Client string `json:"client" description:"Narrow to where it was said: web, mail, discord, telegram, whatsapp. Omit for everywhere"`
	Limit  int    `json:"limit" description:"Max results (default 20, max 200)"`
}

type SearchResponse struct {
	Text string `json:"text" description:"Matching messages, most recent first: who said it, when, where, and the conversation it was on"`
}

// Search finds something that was said, across every client.
// @example {"query": "invoice"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return fmt.Errorf("query is required — say what to look for")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	hits := thread.Search(owner, query, strings.ToLower(strings.TrimSpace(req.Client)), limit)
	if len(hits) == 0 {
		rsp.Text = fmt.Sprintf("Nothing in your history mentions %q.", query)
		return nil
	}

	var b strings.Builder
	for _, h := range hits {
		who := "you"
		switch {
		case h.Role == thread.RoleAgent:
			who = "the agent"
		case h.From != "":
			who = h.From
		}
		b.WriteString(fmt.Sprintf("%s · %s said (%s)\n%s\n\n",
			h.At.Format("2 Jan 2006 15:04"), who, h.Client, snippet(h.Text, query)))
		// The conversation it was on, so a caller that wants the rest can ask
		// for it — a search result nothing follows from is a dead end.
		b.WriteString("  conversation: " + h.Thread)
		if h.Subject != "" {
			b.WriteString(" — " + h.Subject)
		}
		b.WriteString("\n\n")
	}
	rsp.Text = strings.TrimSpace(b.String())
	return nil
}

// ── Conversation ────────────────────────────────────────────────

type ConversationRequest struct {
	ID    string `json:"id" required:"true" description:"The conversation's id, as recall_search reports it"`
	Limit int    `json:"limit" description:"Max messages, most recent kept (default 50)"`
}

type ConversationResponse struct {
	Text string `json:"text" description:"The conversation, oldest first"`
}

// Conversation reads one conversation back, whichever client it happened on.
// @example {"id": "0c34a639398dc3969cd21f96"}
func (Server) Conversation(ctx context.Context, req *ConversationRequest, rsp *ConversationResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		return fmt.Errorf("id is required — recall_search reports one for every result")
	}
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	t := thread.Get(owner, id)
	if t == nil {
		return fmt.Errorf("no conversation %q — it may have been deleted, or belong to somebody else", id)
	}
	msgs := thread.Messages(owner, id, limit)
	if len(msgs) == 0 {
		rsp.Text = "Nothing was said on that one."
		return nil
	}

	var b strings.Builder
	if t.Subject != "" {
		b.WriteString(t.Subject + " (" + t.Client + ")\n\n")
	}
	for _, m := range msgs {
		who := "You"
		switch {
		case m.Role == thread.RoleAgent:
			who = "Agent"
		case m.From != "":
			who = m.From
		}
		b.WriteString(who + ": " + m.Text + "\n\n")
	}
	rsp.Text = strings.TrimSpace(b.String())
	return nil
}

// ── List ────────────────────────────────────────────────────────

type ListRequest struct {
	Client string `json:"client" description:"Only conversations on one client: web, mail, discord, telegram, whatsapp"`
	Limit  int    `json:"limit" description:"Max conversations (default 20)"`
}

type ListResponse struct {
	Text string `json:"text" description:"Conversations, most recently active first: what each is about, where, and its id"`
}

// List names the caller's conversations, most recently active first.
// @example {}
func (Server) List(ctx context.Context, req *ListRequest, rsp *ListResponse) error {
	owner, err := caller(ctx)
	if err != nil {
		return err
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	only := strings.ToLower(strings.TrimSpace(req.Client))

	var b strings.Builder
	shown := 0
	for _, t := range thread.List(owner, 0) {
		if only != "" && t.Client != only {
			continue
		}
		if shown >= limit {
			break
		}
		shown++
		subject := t.Subject
		if subject == "" {
			subject = "Untitled"
		}
		b.WriteString(fmt.Sprintf("- %s (%s, %s)\n  %s\n",
			subject, t.Client, t.Updated.Format("2 Jan 2006 15:04"), t.ID))
	}
	if shown == 0 {
		rsp.Text = "No conversations yet."
		return nil
	}
	rsp.Text = strings.TrimSpace(b.String())
	return nil
}

// Delete removes the caller's whole record when their account goes.
//
// Registered with the account-deletion hooks. This service owns none of the
// data it reads — the store is written by every client on every turn and
// belongs to nobody — which is precisely why the deletion had to be hung
// somewhere, and the reader is the only thing in the catalogue that knows the
// record exists.
func Delete(owner string) { thread.Forget(owner) }

// Load registers the service.
func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("recall", "service register failed: %v", err)
	}
}

// Headless: there is no /recall page, because the page over this record already
// exists and belongs to the agent — /agent/threads, beside the runs and the
// chat. A second one listing the same conversations under a different heading
// would be the catalogue disagreeing with itself about where your history is.
//
// Nothing is charged. Reading your own record touches this instance's storage
// and no model and no third party, which is the whole of the free/paid rule in
// quota.json.
var Spec = service.Spec{
	Name:        "recall",
	Handler:     new(Server),
	Description: "Your own past: search what was said, on any client, and read a conversation back",
	Scoped:      true,
	Endpoints: map[string]service.Endpoint{
		"Search": {Aliases: []string{"history_search", "recall_history"},
			Doc: "Search everything the caller has said to an agent and been told, across every client — the web chat, email, Discord, Telegram, WhatsApp. Use it when something was mentioned and you need to find where; notes_get is for a fact somebody wrote down on purpose"},
		"Conversation": {Aliases: []string{"recall_thread"},
			Doc: "Read one whole conversation back by id, as recall_search and recall_list report it"},
		"List": {Aliases: []string{"recall_conversations"},
			Doc: "List the caller's conversations, most recently active first, with what each is about and where it happened"},
	},
}

// snippet keeps a match readable: the line it was on, trimmed around the term.
//
// The whole message is the wrong answer for a search result — an agent's reply
// can be a thousand words, and twenty of those fill a context window with the
// parts nobody asked about.
func snippet(text, query string) string {
	flat := strings.Join(strings.Fields(text), " ")
	if len(flat) <= 240 {
		return flat
	}
	at := strings.Index(strings.ToLower(flat), strings.ToLower(query))
	if at < 0 {
		return flat[:240] + "…"
	}
	start := at - 100
	if start < 0 {
		start = 0
	}
	end := start + 240
	if end > len(flat) {
		end = len(flat)
	}
	out := flat[start:end]
	if start > 0 {
		out = "…" + out
	}
	if end < len(flat) {
		out += "…"
	}
	return out
}
