package inbox

// Reading a conversation that did not happen here.
//
// There is one list of conversations and it is the rail on /agent. There was
// briefly a second page listing the same conversations under the heading
// Threads, beside a third listing the workflow records behind them under the
// heading Runs, and a tab strip switching between the three. Four names —
// chat, threads, runs, connect — for what is one thing: you, an agent, and
// what you have said to each other.
//
// So the rail lists everything, whichever client it happened on. A conversation
// from the web opens in the chat and can be continued. One that happened by
// email or on WhatsApp opens here instead: the same messages, read-only, with a
// line saying where it took place and how to carry it on — which is by replying
// there, not by typing into a box on this page that would send nothing.

import (
	"html"
	"net/http"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// SessionHandler serves /agent/session/<id>: DELETE removes a conversation.
//
// The rail lists conversations, so the thing it deletes is a conversation. It
// used to DELETE a workflow record, because the rail was built out of those.
func SessionHandler(w http.ResponseWriter, r *http.Request) {
	_, acc, err := auth.RequireSession(r)
	if err != nil {
		app.Unauthorized(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		app.MethodNotAllowed(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/agent/session/")
	if id == "" {
		app.NotFound(w, r, "no conversation named")
		return
	}
	thread.Delete(acc.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

// MessagesShown bounds one conversation.
//
// A mail thread somebody has been adding to for a year is the case this page
// exists for, and rendering every message of it — each through the markdown
// renderer — is a page that takes a second to draw and that nobody reads the
// top of. The most recent are the ones being read.
const MessagesShown = 100

// ConversationView renders a conversation from another client, read-only.
func ConversationView(accountID string, t *thread.Thread) string {
	msgs := thread.Messages(accountID, t.ID, MessagesShown)

	subject := t.Subject
	if subject == "" {
		subject = "Untitled"
	}

	var b strings.Builder
	b.WriteString(`<div class="els"><div class="els-head"><span class="els-where">` +
		html.EscapeString(app.ClientName(t.Client)) + `</span><span class="els-when">started ` +
		html.EscapeString(app.TimeAgo(t.Started)) + `</span></div>`)
	b.WriteString(`<h2 class="els-title">` + html.EscapeString(subject) + `</h2>`)
	b.WriteString(partyLine(accountID, t))
	if len(msgs) >= MessagesShown {
		b.WriteString(`<p class="els-trimmed">Showing the most recent ` +
			strconv.Itoa(MessagesShown) + `. ` +
			app.Link("Search the whole conversation", "/recall") + `</p>`)
	}

	for _, m := range msgs {
		b.WriteString(messageBlock(accountID, t, m))
	}

	// Where a reply goes, which is not this page.
	//
	// Worth saying now that there is a box below it: the box talks to the agent
	// about the conversation, and replying to whoever wrote in is a different
	// act that happens where the conversation is.
	b.WriteString(`<p class="els-note">This happened on ` +
		html.EscapeString(app.ClientName(t.Client)) + `, so a reply carries on there — answer it ` +
		`the way it arrived and the agent picks it up in the same thread.</p>`)
	b.WriteString(`</div>` + conversationCSS)
	return b.String()
}

// partyLine says who is on a conversation.
//
// Worth its own line because a thread is not always two-sided, and where it is
// not, nothing on the page said so: a stranger writes to agent@, the agent
// answers, and the owner reads an exchange they were never in. "You" and
// "Agent" describe that badly enough to be misleading.
//
// Silent for the ordinary case — you and your agent, which every conversation
// started here is — because a line naming the two people already obvious from
// the messages is furniture.
func partyLine(accountID string, t *thread.Thread) string {
	people := 0
	var names []string
	for _, p := range thread.Parties(accountID, t.ID) {
		if p.Kind == thread.RoleAgent {
			continue
		}
		people++
		names = append(names, partyName(p))
	}
	if people < 2 {
		return ""
	}
	return `<div class="els-parties">Between ` + html.EscapeString(strings.Join(names, ", ")) +
		` and the agent</div>`
}

// partyName is what to call somebody: the name the client knew, the address
// they wrote from, or "You" for the account this conversation belongs to.
func partyName(p thread.Party) string {
	switch {
	case p.Name != "":
		return p.Name
	case p.Key != "":
		return p.Key
	}
	return "You"
}

// messageBlock is one message. What a person wrote is escaped and shown as
// typed; what an agent wrote is markdown, rendered the way the chat renders it —
// through the untrusted renderer, because model output is exactly what that
// renderer is for.
func messageBlock(accountID string, t *thread.Thread, m thread.Message) string {
	if m.Role == thread.RoleAgent {
		ran := ""
		if m.Workflow != "" {
			ran = runTools(m.Workflow)
		}
		return `<div class="th-msg th-agent"><div class="th-from">Agent · ` +
			html.EscapeString(app.TimeAgo(m.At)) + `</div>` +
			`<div class="th-body">` + app.RenderString(m.Text) + `</div>` + ran + `</div>`
	}
	// The author, by the name the conversation knows them under rather than the
	// address on the message. A thread where three people have written is three
	// names; one where the display name arrived later says it on every line.
	who := "You"
	if m.From != "" {
		who = m.From
		for _, p := range thread.Parties(accountID, t.ID) {
			if p.Kind == thread.RolePerson && p.Key == m.From && p.Name != "" {
				who = p.Name
				break
			}
		}
	}
	return `<div class="th-msg th-person"><div class="th-from">` + html.EscapeString(who) + ` · ` +
		html.EscapeString(app.TimeAgo(m.At)) + `</div>` +
		`<div class="th-body th-typed">` + html.EscapeString(m.Text) + `</div></div>`
}

// Tools renders which tools produced an answer, and is filled in by the agent
// because the workflow record is the agent's own — how an answer was made is a
// different question, with a different lifetime, from what was said.
//
// Nil on a build with no agent wired in, and silent when the run has expired,
// which it will: workflow records are evicted and messages are not, so an old
// conversation keeps what was said and loses how it was produced. That
// asymmetry is intended.
var Tools func(workflow string) string

func runTools(workflow string) string {
	if Tools == nil {
		return ""
	}
	return Tools(workflow)
}

const conversationCSS = `<style>
.els-head{display:flex;align-items:center;gap:10px;margin-bottom:4px}
.els-where{border:1px solid #eee;border-radius:999px;padding:2px 9px;font-size:11px;color:#666}
.els-when{font-size:12px;color:#aaa}
.els-title{font-size:20px;margin:0 0 6px}
.els-parties{font-size:13px;color:#888;margin:0 0 20px}
.els-trimmed{font-size:12px;color:#999;margin:0 0 18px}
.els-note{margin-top:24px;padding-top:14px;border-top:1px solid #eee;font-size:13px;color:#888}
.th-msg{border-left:2px solid #eee;padding-left:14px;margin-bottom:16px}
.th-agent{border-left-color:#ddd}
.th-from{font-size:12px;color:#999;margin-bottom:4px}
.th-body{font-size:14px;line-height:1.6}
.th-body p:first-child{margin-top:0}
.th-body p:last-child{margin-bottom:0}
.th-typed{white-space:pre-wrap}
.th-tools{display:flex;flex-wrap:wrap;gap:4px;margin-top:8px}
.th-tool{border:1px solid #eee;border-radius:999px;padding:2px 9px;font-size:11px;color:#666;white-space:nowrap}
.th-failed{font-size:12px;color:#b00}
</style>`
