package agent

// Every conversation, wherever it happened.
//
// internal/thread has been written on every turn from every client since the
// surround was made one thing — a mail chain, a WhatsApp exchange, a Discord
// DM, a line typed on the page — and nothing read it. A record nobody can see
// is indistinguishable from one that is not being kept, which is the state the
// clients were in before: three of them held history in a map in memory and it
// was gone on restart, and nobody noticed for a year because there was no page
// that would have looked wrong.
//
// So this is the read side, and deliberately the plainest possible one: the
// account's conversations newest first, filtered by client, and one of them
// opened. A mail thread and a WhatsApp thread sitting next to each other is the
// whole claim — the agent is the same one behind every client, and now the
// record says so.
//
// # Threads and Runs are two records, not two views
//
// Runs is agent.Flow: what was asked, which tools ran, whether it finished.
// That is *how an answer was produced*, it is debugging, and it expires. This
// is *what was said*, it is memory, and it should not. They were one struct
// once — which is how a workflow record came to stand in as conversation
// history — and the two tabs sit beside each other so the difference is visible
// rather than argued about. Where a message names the run that produced it,
// this page shows what that run called, which is the join between them.
//
// # What this is not
//
// Not a service. Reading your own past on purpose — searching it, asking an
// agent to recall something — is a decision, and a service over the store is
// welcome. A page is not that: it renders what is already there for the person
// it belongs to. Delete this file and the record is unaffected.
//
// The chat rail on /agent is still built from flows rather than from here. It
// is a conversation list assembled from a workflow store, which is exactly the
// confusion above, and it should read this instead — left alone for now because
// the rail also drives which turn a reply continues, and that is a second
// change.

import (
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/thread"
)

// threadsShown bounds the list. High enough that nobody scrolls past it and
// finds the record apparently ends, low enough that a busy account does not
// render five thousand rows.
const threadsShown = 200

// messagesShown bounds one conversation. 0 is all of them; a conversation that
// has run for months is the case this page exists for, so it is not truncated.
const messagesShown = 0

// ThreadsHandler serves /agent/threads — the list, or one conversation.
func ThreadsHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	// Forgetting one. A conversation is somebody's own memory of talking to
	// their agent, and until now there was no way to be rid of one.
	if r.Method == http.MethodPost && r.FormValue("action") == "delete" {
		thread.Delete(owner, r.FormValue("id"))
		http.Redirect(w, r, "/agent/threads"+agentQuery(r.FormValue("agent")), http.StatusSeeOther)
		return
	}

	// Which agent this page is for, carried by the tab strip so moving between
	// Chat, Threads, Runs and Connect keeps your place.
	onlyAgent := strings.TrimSpace(r.URL.Query().Get("agent"))

	if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
		oneThread(w, r, owner, id)
		return
	}

	threads := thread.List(owner, 0)
	if onlyAgent != "" {
		var mine []thread.Thread
		for _, t := range threads {
			if t.Agent == onlyAgent {
				mine = append(mine, t)
			}
		}
		threads = mine
	}

	// One client's conversations. The chips below are built from what is
	// actually there rather than from the list of clients that exist, so an
	// instance with no Discord bot does not offer to filter by Discord.
	only := strings.TrimSpace(r.URL.Query().Get("client"))
	present := clientsPresent(threads)
	if only != "" {
		var mine []thread.Thread
		for _, t := range threads {
			if t.Client == only {
				mine = append(mine, t)
			}
		}
		threads = mine
	}
	if len(threads) > threadsShown {
		threads = threads[:threadsShown]
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"threads": threads})
		return
	}

	var b strings.Builder
	b.WriteString(`<p class="lens-lead">Everything you and your agents have said to each other, ` +
		`whichever way you said it — the page, email, Discord, Telegram, WhatsApp. One record, so a ` +
		`conversation you started in a browser and carried on by mail is one conversation. ` +
		`How an answer was produced — the tools it called, whether it finished — is on ` +
		app.Link("Runs", "/agent/runs"+agentQuery(onlyAgent)) + `</p>`)

	if onlyAgent != "" {
		name := onlyAgent
		if a := For(owner, onlyAgent); a != nil {
			name = a.Name
		} else if n := platformName(onlyAgent); n != "" {
			name = n
		}
		b.WriteString(`<p class="lens-lead" style="margin-top:-8px">Showing only <strong>` +
			html.EscapeString(name) + `</strong>. ` + app.Link("Every agent", "/agent/threads") + `</p>`)
	}

	if len(present) > 1 {
		b.WriteString(clientChips(present, only, onlyAgent))
	}

	switch {
	case len(threads) == 0 && only != "":
		b.WriteString(`<p class="th-empty">No conversations on ` + html.EscapeString(clientName(only)) +
			` yet. ` + app.Link("Every client", "/agent/threads"+agentQuery(onlyAgent)) + `</p>`)
	case len(threads) == 0 && onlyAgent != "":
		b.WriteString(`<p class="th-empty">No conversations with this agent yet. Ask it something ` +
			`on the Chat tab, or write to its address, and the conversation appears here. ` +
			app.Link("Every agent", "/agent/threads") + `</p>`)
	case len(threads) == 0:
		b.WriteString(`<p class="th-empty">Nothing yet. Ask an agent something — here, or by ` +
			`email, or from any client you have connected — and the conversation appears here.</p>`)
	default:
		csrf := auth.CSRFToken(r)
		b.WriteString(`<div class="threads">`)
		for _, t := range threads {
			b.WriteString(threadRow(owner, t, csrf, onlyAgent))
		}
		b.WriteString(`</div>`)
	}

	page := agentPage("threads", onlyAgent, b.String()) + threadsCSS
	w.Write([]byte(app.RenderHTMLForRequest("Threads", "Every conversation, wherever it happened", page, r)))
}

// oneThread renders a single conversation.
func oneThread(w http.ResponseWriter, r *http.Request, owner, id string) {
	t := thread.Get(owner, id)
	if t == nil {
		http.NotFound(w, r)
		return
	}
	msgs := thread.Messages(owner, id, messagesShown)

	if app.WantsJSON(r) {
		app.RespondJSON(w, map[string]any{"thread": t, "messages": msgs})
		return
	}

	subject := t.Subject
	if subject == "" {
		subject = "Untitled"
	}

	var b strings.Builder
	b.WriteString(`<p class="th-back">` + app.Link("← All threads", "/agent/threads"+agentQuery(t.Agent)) + `</p>`)
	b.WriteString(`<h2 class="th-title">` + html.EscapeString(subject) + `</h2>`)
	b.WriteString(`<div class="th-meta">` + clientChip(t.Client) + ` · started ` +
		html.EscapeString(app.TimeAgo(t.Started)) + ` · ` +
		strconv.Itoa(len(msgs)) + ` message` + plural(len(msgs)) +
		`<form method="POST" action="/agent/threads" style="margin:0 0 0 auto" ` +
		`onsubmit="return confirm('Delete this conversation? What was said in it is gone.')">` +
		`<input type="hidden" name="_csrf" value="` + html.EscapeString(auth.CSRFToken(r)) + `">` +
		`<input type="hidden" name="action" value="delete">` +
		`<input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `">` +
		`<input type="hidden" name="agent" value="` + html.EscapeString(t.Agent) + `">` +
		`<button type="submit" class="th-del">Delete</button></form></div>`)

	if len(msgs) == 0 {
		b.WriteString(`<p class="th-empty">Nothing was said on this one.</p>`)
	}
	b.WriteString(`<div class="th-msgs">`)
	for _, m := range msgs {
		b.WriteString(messageBlock(m))
	}
	b.WriteString(`</div>`)

	page := agentPage("threads", t.Agent, b.String()) + threadsCSS
	w.Write([]byte(app.RenderHTMLForRequest(subject, "A conversation", page, r)))
}

// agentQuery is ?agent=<id>, or nothing. The tab strip and every link between
// these pages carries the selected agent; this is the one place that spells it.
func agentQuery(agentID string) string {
	if strings.TrimSpace(agentID) == "" {
		return ""
	}
	return "?agent=" + url.QueryEscape(agentID)
}

// threadRow is one conversation in the list.
func threadRow(owner string, t thread.Thread, csrf, agentID string) string {
	subject := t.Subject
	if subject == "" {
		subject = "Untitled"
	}
	if len(subject) > 90 {
		subject = strings.TrimSpace(subject[:90]) + "…"
	}

	// The last thing said, which is what tells two conversations on the same
	// subject apart — and on mail, whether the agent actually answered.
	msgs := thread.Messages(owner, t.ID, 1)
	last := ""
	if len(msgs) == 1 {
		who := "You"
		switch {
		case msgs[0].Role == thread.RoleAgent:
			who = "Agent"
		case msgs[0].From != "":
			who = msgs[0].From
		}
		text := strings.TrimSpace(strings.ReplaceAll(msgs[0].Text, "\n", " "))
		if len(text) > 120 {
			text = strings.TrimSpace(text[:120]) + "…"
		}
		last = `<div class="th-last"><span class="th-who">` + html.EscapeString(who) + `</span> ` +
			html.EscapeString(text) + `</div>`
	}

	// The whole row is not one anchor: a delete control cannot live inside one,
	// and a conversation you cannot be rid of is the complaint this answers.
	del := ""
	if csrf != "" {
		del = `<form method="POST" action="/agent/threads" style="margin:0" ` +
			`onsubmit="return confirm('Delete this conversation? What was said in it is gone.')">` +
			`<input type="hidden" name="_csrf" value="` + html.EscapeString(csrf) + `">` +
			`<input type="hidden" name="action" value="delete">` +
			`<input type="hidden" name="id" value="` + html.EscapeString(t.ID) + `">` +
			`<input type="hidden" name="agent" value="` + html.EscapeString(agentID) + `">` +
			`<button type="submit" class="th-del" title="Delete">Delete</button></form>`
	}

	return `<div class="th-row">
  <a class="th-open" href="/agent/threads?id=` + url.QueryEscape(t.ID) + `">
    <div class="th-subject">` + html.EscapeString(subject) + `</div>
    ` + last + `
  </a>
  <div class="th-side">` + clientChip(t.Client) + `<span class="th-when">` +
		html.EscapeString(app.TimeAgo(t.Updated)) + `</span>` + del + `</div>
</div>`
}

// messageBlock is one message. What a person wrote is escaped and shown as
// typed; what an agent wrote is markdown, and is rendered the same way the chat
// renders it — through the untrusted renderer, because model output is exactly
// what that renderer is for.
func messageBlock(m thread.Message) string {
	if m.Role == thread.RoleAgent {
		ran := ""
		if m.Workflow != "" {
			ran = runTools(m.Workflow)
		}
		return `<div class="th-msg th-agent"><div class="th-from">Agent · ` +
			html.EscapeString(app.TimeAgo(m.At)) + `</div>` +
			`<div class="th-body">` + app.RenderString(m.Text) + `</div>` + ran + `</div>`
	}
	who := "You"
	if m.From != "" {
		who = m.From
	}
	return `<div class="th-msg th-person"><div class="th-from">` + html.EscapeString(who) + ` · ` +
		html.EscapeString(app.TimeAgo(m.At)) + `</div>` +
		`<div class="th-body th-typed">` + html.EscapeString(m.Text) + `</div></div>`
}

// runTools shows what the run behind an answer actually called.
//
// This is the one place the two records meet: a message names its workflow, and
// the workflow knows the tools. Silent when the run has expired, which it will —
// workflow records are evicted and messages are not, so an old conversation
// keeps what was said and loses how it was produced. That is the intended
// asymmetry, not a gap to fill in.
func runTools(workflow string) string {
	f := getFlow(workflow)
	if f == nil {
		return ""
	}
	var chips strings.Builder
	seen := map[string]bool{}
	for _, s := range f.Steps {
		if s.Tool == "" || seen[s.Tool] {
			continue
		}
		seen[s.Tool] = true
		chips.WriteString(`<span class="th-tool">` + html.EscapeString(s.Tool) + `</span>`)
	}
	if f.Status == "error" && f.Error != "" {
		chips.WriteString(`<span class="th-failed">` + html.EscapeString(f.Error) + `</span>`)
	}
	if chips.Len() == 0 {
		return ""
	}
	return `<div class="th-tools">` + chips.String() + `</div>`
}

// clientsPresent names the clients this account has actually used, in a fixed
// order so the chips do not reshuffle between page loads.
func clientsPresent(threads []thread.Thread) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range threads {
		if t.Client == "" || seen[t.Client] {
			continue
		}
		seen[t.Client] = true
		out = append(out, t.Client)
	}
	sort.Strings(out)
	return out
}

func clientChips(present []string, active, agentID string) string {
	chip := func(label, client string) string {
		cls := "th-filter"
		if client == active {
			cls += " on"
		}
		q := url.Values{}
		if client != "" {
			q.Set("client", client)
		}
		if agentID != "" {
			q.Set("agent", agentID)
		}
		href := "/agent/threads"
		if len(q) > 0 {
			href += "?" + q.Encode()
		}
		return `<a class="` + cls + `" href="` + href + `">` + html.EscapeString(label) + `</a>`
	}
	var b strings.Builder
	b.WriteString(`<div class="th-filters">` + chip("Everywhere", ""))
	for _, c := range present {
		b.WriteString(chip(clientName(c), c))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func clientChip(client string) string {
	if client == "" {
		return ""
	}
	return `<span class="th-client">` + html.EscapeString(clientName(client)) + `</span>`
}

// clientName is what a client is called in front of somebody. A client not
// named here is shown as it names itself — a new one should appear on this page
// the day it is written, not the day somebody remembers to add it to a map.
func clientName(client string) string {
	switch client {
	case WebClient:
		return "Web"
	case "mail":
		return "Email"
	case "discord":
		return "Discord"
	case "telegram":
		return "Telegram"
	case "whatsapp":
		return "WhatsApp"
	case "sms":
		return "SMS"
	case "cli":
		return "CLI"
	case "a2a":
		return "A2A"
	}
	return client
}

const threadsCSS = `<style>
.threads{display:flex;flex-direction:column;gap:8px}
.th-row{display:flex;align-items:flex-start;gap:12px;border:1px solid #eee;border-radius:8px;
  padding:10px 14px}
.th-row:hover{border-color:#ddd;background:#fcfcfc}
.th-open{flex:1;min-width:0;text-decoration:none;color:inherit}
.th-del{background:none;border:0;color:#bbb;font-size:12px;cursor:pointer;padding:0}
.th-del:hover{color:#b00;text-decoration:underline}
.th-subject{font-weight:600;font-size:14px;color:var(--text-primary,#111)}
.th-last{font-size:13px;color:#888;margin-top:3px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.th-who{color:#aaa}
.th-side{display:flex;flex-direction:column;align-items:flex-end;gap:4px;flex-shrink:0}
.th-when{font-size:12px;color:#aaa;white-space:nowrap}
.th-client{border:1px solid #eee;border-radius:999px;padding:2px 9px;font-size:11px;color:#666;white-space:nowrap}
.th-filters{display:flex;flex-wrap:wrap;gap:6px;margin:0 0 16px}
.th-filter{border:1px solid #eee;border-radius:999px;padding:3px 11px;font-size:12px;color:#666;text-decoration:none}
.th-filter:hover{border-color:#ccc}
.th-filter.on{background:var(--text-primary,#111);border-color:var(--text-primary,#111);color:#fff}
.th-empty{color:#888;font-size:14px}
.th-back{font-size:13px;margin:0 0 10px}
.th-title{font-size:20px;margin:0 0 6px}
.th-meta{font-size:12px;color:#999;margin:0 0 20px;display:flex;align-items:center;gap:8px}
.th-msgs{display:flex;flex-direction:column;gap:16px}
.th-msg{border-left:2px solid #eee;padding-left:14px}
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
