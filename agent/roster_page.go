package agent

// The agents page: what acts for you, and what each one may touch.
//
// It lists and it revokes; it does not create. Making an agent happens in one
// place, the builder at /agent/new, because this page used to carry a second
// create form that asked different questions and issued a different result.
//
// Nothing here is checked twice: the scope chosen in the builder is written into
// the token's permissions, and the MCP boundary enforces it against every call.
// This page cannot grant more than it shows, because it is not the thing doing
// the granting.

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
)

// Handler serves /agents.
func RosterHandler(w http.ResponseWriter, r *http.Request) {
	sess, _, err := auth.RequireSession(r)
	if err != nil {
		app.RedirectToLogin(w, r)
		return
	}
	owner := sess.Account

	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "delete":
			_ = RemoveAgent(owner, r.FormValue("id"))
			http.Redirect(w, r, "/agents?removed=1", http.StatusSeeOther)
			return
		case "delete-run":
			// One workflow record, from the summary on the builder — the only
			// place runs are still listed, now that the page listing all of them
			// is gone.
			_ = deleteFlow(owner, r.FormValue("id"))
			http.Redirect(w, r, "/agents", http.StatusSeeOther)
			return
		case "token":
			// Back to where it was asked for.
			//
			// Both token forms post here, because this is where the action
			// lives — so pressing "Issue one" on an agent's Connect page landed
			// you on the roster, looking at a different page than the one you
			// were reading, with the secret somewhere on it. The form says
			// where it came from and this honours it.
			back := localPath(r.FormValue("back"))
			secret, err := IssueToken(owner, r.FormValue("id"))
			if err != nil {
				http.Redirect(w, r, back+query(back, "error="+urlSafe(err.Error())), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, back+query(back,
				"created="+r.FormValue("id")+"&secret="+urlSafe(secret)), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/agents", http.StatusSeeOther)
		return
	}

	if app.WantsJSON(r) {
		app.RespondJSON(w, Agents(owner))
		return
	}

	csrf := auth.CSRFToken(r)
	var b strings.Builder
	b.WriteString(app.Column())
	// The model, in one sentence: you make one, it gets an address, you talk
	// to it — and a token lets something else use its tools.
	//
	// The page has been led by three other sentences. One listed what an agent
	// *has* before saying what you can do with it. One promised an inbox. This
	// one is what somebody has to understand to use the page, and it is in the
	// order they will do it.
	b.WriteString(`<p class="lens-lead">Make an agent: give it a name, an instruction and ` +
		`the services it may use. It gets its own address, you can chat with it here, and a ` +
		`token lets Claude, Cursor or your own code use the tools you gave it.</p>`)
	// The catalogue, from the page that scopes it.
	//
	// /tools left the sidebar, because a tool is a property of something rather
	// than a destination: an agent's tools are what it may reach for, and this
	// is the page where that is decided. So the way in is from here, next to
	// the sentence that says an agent is given services.
	//
	// On its own line rather than trailing the paragraph. Buried at the end of
	// three sentences it read as a footnote to the last clause; it is the one
	// place the catalogue is reachable from now, which is a link somebody
	// should be able to find without reading to the end.
	b.WriteString(`<p class="lens-go">` + app.TextLink("See the tools", "/tools") + `</p>`)

	if msg := r.URL.Query().Get("error"); msg != "" {
		// The way out is a link, because it reads as one. Hitting the agent limit
		// names where to change it, and as plain text that is a place you click
		// and nothing happens — so the one route the message exists to open is
		// the one that dead-ends. Named as the page rather than as its route,
		// for the reason the post banner gives: /plans is how it is addressed,
		// "Plans" is what it is called.
		said := html.EscapeString(msg)
		said = strings.ReplaceAll(said, "your account", `<a href="/account">your account</a>`)
		b.WriteString(`<p class="text-error">` + said + `</p>`)
	}
	if r.URL.Query().Get("removed") != "" {
		b.WriteString(`<p class="text-sm text-secondary">Agent removed and its token revoked.</p>`)
	}
	created := r.URL.Query().Get("created")
	if secret := r.URL.Query().Get("secret"); secret != "" {
		b.WriteString(secretPanel(secret, For(owner, created), app.BaseURL(r)))
	} else if created != "" {
		// An agent that runs here has no secret to show, so say what happened
		// and what it is now for — otherwise the page just silently grows a row.
		name := "It"
		if a := For(owner, created); a != nil {
			name = html.EscapeString(a.Name)
		}
		b.WriteString(`<p class="text-sm text-secondary">` + name + ` is ready. Open its name below ` +
			`to talk to it here — it has no token, so nothing outside this instance can call it.</p>`)
	}

	// Yours first, then ours.
	//
	// It was the other way round, because the section listing this instance's own
	// agents was added to a page that had only ever listed yours, and new
	// sections go at the top. That put six things nobody made above the one thing
	// they did, on the page named for what they made — and the empty state read
	// "That is the only one so far", a sentence about the row above it that only
	// worked in that order.
	//
	// The heading is drawn either way now. "Your agents / none yet" is the
	// account saying something true about itself; skipping it when the list is
	// empty means the page tells you what it has and not what you have.
	// One list, with the default first.
	//
	// Micro is not one of "our agents" in the sense the ten specialists were —
	// it is the agent this account already has. It answers agent@, it is who the
	// chat talks to, and it exists before anybody makes anything. Leaving it off
	// meant a new account opened /agents and was told it had none, which is
	// false, and meant the only row on the page after making one was a thing you
	// had to make to see anything at all.
	//
	// No headings. "Your agents" over a list that also contains the default
	// would be wrong, and two headings for two rows is furniture. What
	// distinguishes them is already on the row: the default has no Edit and no
	// Remove, because neither is a thing you can do to it.
	EnsureTags(owner)
	roster := Agents(owner)
	b.WriteString(`<div class="col m-0 mb-6">`)
	// The default carries the same sign of life as the rest. It is the one most
	// accounts have actually used, so a roster where every row but that one says
	// when it last spoke is a roster missing the row that would say the most.
	// Every agent this instance ships, not only the default. That was a single
	// hardcoded row back when there was a single one to draw; a second built-in
	// registered itself, answered at its own address, and appeared nowhere a
	// person could find it.
	for _, id := range PlatformNames() {
		b.WriteString(platformSeenRow(id, owner))
	}
	for _, a := range roster {
		b.WriteString(agentRow(a, csrf, app.BaseURL(r)))
	}
	b.WriteString(`</div>`)

	// One way to make an agent. This page used to carry its own create form
	// alongside the builder at /agent/new, and the two disagreed: one asked for
	// services and issued a token, the other asked for tools and a system prompt
	// and issued nothing, and neither linked to the other. The builder took the
	// missing question ("where does it run?") so this could become a link.
	//
	// At the cap it is not a button to the builder, because the builder is a
	// form you cannot submit. It becomes the thing that would actually change
	// the answer.
	if full, have, max := AtAgentLimit(owner); full {
		b.WriteString(app.ActionLink("/wallet/topup", "Top up to lift the limit"))
		b.WriteString(fmt.Sprintf(
			`<p class="text-sm text-secondary mt-2 m-0">Your plan runs %d agent%s and you have %d. `+
				`Verify your address or put credit on <a href="/account">your account</a>, or delete one first.</p>`,
			max, plural(max), have))
	} else {
		b.WriteString(app.ActionLink("/agent/new", "+ New agent"))
	}

	// The instance's own agents are not listed here.
	//
	// They were, for a while, on the argument that eleven had been in the
	// registry since the router was written and were reachable at agent+news@
	// and nowhere else. True, and the cure was worse: this page is where you
	// understand what an agent *is*, and eleven things nobody made, with no
	// stated principle behind why news but not sport, taught that an agent is
	// something the product hands you rather than something you make. Six rows
	// of somebody else's things above the one thing that is yours.
	//
	// The model is one sentence now: you make an agent, it gets an address, you
	// talk to it. The registry is untouched — it still routes, and agent+news@
	// still answers — it is just not what this page is about.

	// Close the column this page opened. Without it the footer was swallowed
	// into a 720px div and rendered halfway up the page: #content is a flex
	// child sized to hold the footer at the bottom, and an unclosed div puts
	// the footer inside the content instead of after it.
	b.WriteString(`</div>`)
	b.WriteString(agentsCSS)
	app.Respond(w, r, app.Response{Title: "Agents", Description: "The agents that act for you, and what each one may reach", HTML: b.String()})
}

func urlSafe(s string) string { return strings.ReplaceAll(html.EscapeString(s), " ", "%20") }

// localPath is a redirect target this instance is willing to send somebody to.
//
// A path on this host, nothing else. It arrives in a form field, so without
// this an open redirect is one hidden input away — and the roster is the safe
// default because it is where the agent is either way.
func localPath(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "/agents"
	}
	return s
}

// query appends parameters to a path that may already have some.
func query(path, params string) string {
	if strings.Contains(path, "?") {
		return "&" + params
	}
	return "?" + params
}

// secretPanel shows the token once. There is no second chance by design: it is
// stored hashed, so this page could not show it again if it wanted to.
func secretPanel(secret string, a *Agent, base string) string {
	name, endpoint := "your agent", base+"/mcp"
	if a != nil {
		name, endpoint = a.Name, a.Endpoint(base)
	}
	return `<div class="card card-ok m-0 mb-5">` +
		`<h4 class="m-0 mb-2 text-base">` + html.EscapeString(name) + ` is ready</h4>` +
		`<p class="text-sm text-muted m-0 mb-2">Copy this token now — it is stored hashed and cannot be shown again.</p>` +
		`<pre class="agent-secret">` + html.EscapeString(secret) + `</pre>` +
		`<p class="text-sm text-muted mt-2 m-0">Point it at <code>` + html.EscapeString(endpoint) + `</code></p>` +
		`</div>`
}

// agentRow shows an agent the way it is actually used.
//
// It used to show every agent the same: an MCP endpoint, "never used", and an
// "Issue token" button with no hint whether one was needed. For an agent you
// only talk to here that is three kinds of wrong at once — the endpoint cannot
// be called without a token, "never used" describes a token that does not
// exist, and the button reads like a required step. The first question anybody
// asked was "do I need to issue a token?", which is the page's fault.
//
// So it branches on whether there is a credential, not on the declared kind:
// what matters is whether this thing can be called from outside, and a token is
// exactly what decides that.
// entry is one line in the directory: somebody you can write to.
//
// Both lists render through this, which they did not before — an agent you made
// and one that came with the instance were two different pieces of markup with
// different links in a different order, so the page looked like two products
// stacked. "Talk to it" and "How to reach it →" on one, "Connect / Edit / Fork /
// Talk to it →" on the other, all at slightly different sizes.
//
// What a directory entry is: a name, what it is for, its address, and the two
// ways to reach it. Everything else — the token, the endpoint, the scope, where
// it runs — is administration, and lives one click away on the page that exists
// for it.
type entry struct {
	Name string // what it is called
	Path string // where the name opens
	// Chat is where talking to it happens, and empty when there is nowhere.
	//
	// An agent that runs elsewhere is reached over MCP by something outside this
	// instance; there is no conversation with it here, and offering one is a box
	// that will never be used. Its name opens the page it does have.
	Chat  string
	For   string // one line: what it is for
	ID    string // for the links that need it
	Admin bool   // whether Edit and Fork apply, which is only to your own
	Extra string // owner-only controls, in the strip with the links
	// Seen is what it last dealt with, and When is how long ago.
	//
	// The roster was a directory: name, purpose, three links, and nothing
	// saying whether any of them were alive. A list you are scanning to decide
	// who to talk to needs a sign of life against each one.
	//
	// Two fields because they go in two places. The subject reads as a line
	// under the name, where the description already is; the time is a
	// right-hand column, the way it is on every list of conversations here and
	// everywhere else. It was one string reading "Last: Tuesday · 2 hours ago",
	// which put a label nobody needs in front of the only interesting word and
	// buried the timestamp mid-sentence.
	Seen string
	When string
}

// entryRow draws one.
//
// Chat first, because talking to it is the point of it existing. Email second,
// as a mailto — this product's whole claim is that you can write to the thing,
// and a page listing agents that had no way to start a message was describing
// the feature rather than offering it.
func entryRow(e entry) string {
	var b strings.Builder
	b.WriteString(`<div class="agent-row card-hover"><div class="grow min-w-0">`)
	// The name, and how long ago it last spoke out to the right of it — the
	// same pair, in the same places, as a row in the inbox.
	b.WriteString(`<div class="agent-head">`)
	b.WriteString(`<a class="agent-name" href="` + e.Path + `">` + html.EscapeString(e.Name) + `</a>`)
	if e.When != "" {
		b.WriteString(`<span class="agent-when">` + html.EscapeString(e.When) + `</span>`)
	}
	b.WriteString(`</div>`)
	if e.For != "" {
		b.WriteString(`<div class="agent-for">` + html.EscapeString(e.For) + `</div>`)
	}
	if e.Seen != "" {
		b.WriteString(`<div class="agent-seen">` + html.EscapeString(e.Seen) + `</div>`)
	}
	// No address on the row, and no Email link.
	//
	// Both were here on the argument that writing to the thing is what it is
	// for. They made every row two lines longer and neither is what somebody
	// does from a list: an address is something you copy once, from the page
	// about that agent, and a mailto opens a client with an empty message in
	// it. The list is for picking which agent; Connect is where its address is.
	// Every action on the row is in one strip, Remove included.
	//
	// Remove used to be a second child of the row, beside the block holding
	// everything else. On a wide screen that put it out to the right, which was
	// fine; on a phone the row stacks, so it became a block under the card with
	// a 36px mobile touch target around a 14px word — a hand's width of nothing
	// between the last link and the button. It is a link like its neighbours and
	// it now sits with them.
	b.WriteString(`<div class="agent-links">`)
	if e.Chat != "" {
		b.WriteString(`<a href="` + e.Chat + `">Chat</a>`)
	}
	b.WriteString(`<a href="/agent/connect?id=` + html.EscapeString(e.ID) + `">Connect</a>`)
	if e.Admin {
		b.WriteString(`<a href="/agent/new?id=` + html.EscapeString(e.ID) + `">Edit</a>`)
	}
	b.WriteString(e.Extra)
	b.WriteString(`</div></div></div>`)
	return b.String()
}

func agentRow(a *Agent, csrf, base string) string {
	// What it is for, in one line.
	//
	// The row used to carry four: the scope as a coloured list of service
	// names, a meta line about tokens, an address with a verb in front of it,
	// and a badge saying whether it runs here — every fact the system knows,
	// on a list you are scanning to decide who to write to. A directory entry
	// says what somebody does; the rest is on their page.
	//
	// The description is what the agent was made for and what the router reads
	// when deciding who answers, so it is the closest thing to a job title. An
	// agent made before there was a field for it falls back to its scope.
	for_ := strings.TrimSpace(a.Description)
	if for_ == "" {
		if a.Unscoped() {
			for_ = "Everything"
		} else {
			labels := make([]string, 0, len(a.Services))
			for _, sv := range a.Services {
				labels = append(labels, service.Label(sv))
			}
			for_ = strings.Join(labels, ", ")
		}
	}

	// Remove stays on the row, because it is the one thing you do to an entry
	// without opening it — and it is the reason the row has a right-hand side.
	extra := fmt.Sprintf(`<form method="POST" action="/agents" class="m-0" onsubmit="return confirm('Remove this agent?')">
    <input type="hidden" name="_csrf" value="%s">
    <input type="hidden" name="action" value="delete">
    <input type="hidden" name="id" value="%s">
    <button type="submit" class="agent-remove">Remove</button>
  </form>`, html.EscapeString(csrf), html.EscapeString(a.ID))

	// Every agent opens on talking to it.
	//
	// There used to be a fork here: an agent declared "external" opened on the
	// Connect page instead, because there was nothing to talk to — it ran in
	// Claude or Cursor and called in with a token. That distinction is gone.
	// Every agent runs here and answers at POST /agent/<name>; a token is a
	// thing you additionally give something outside, not a different kind of
	// agent. See the Kind field in roster.go.
	open, chat := Path(a.Owner, a.ID), Path(a.Owner, a.ID)

	return entryRow(entry{
		Name:  a.Name,
		Path:  open,
		Chat:  chat,
		For:   for_,
		Seen:  seenLine(a.Owner, a.ID),
		When:  seenWhen(a.Owner, a.ID),
		ID:    a.ID,
		Admin: true,
		Extra: extra,
	})
}

// platformRow is one of this instance's own agents.
// platformSeenRow is platformRow with the account's own history against it.
//
// A second function rather than a parameter on platformRow, because /tools uses
// that one to describe the instance's agents to somebody who may not be signed
// in, where "last used 2 hours ago" would be a fact about a stranger.
func platformSeenRow(name, accountID string) string {
	row := platformRow(name)

	// How long ago, beside the name.
	if when := seenWhen(accountID, name); when != "" {
		if i := strings.Index(row, `</a>`); i >= 0 {
			cut := i + len(`</a>`)
			row = row[:cut] + `<span class="agent-when">` + html.EscapeString(when) + `</span>` + row[cut:]
		}
	}

	// And what it was about, under the description where every other row puts
	// it.
	line := seenLine(accountID, name)
	if line == "" {
		return row
	}
	const at = `</div>`
	if i := strings.Index(row, `class="agent-for">`); i >= 0 {
		if j := strings.Index(row[i:], at); j >= 0 {
			cut := i + j + len(at)
			return row[:cut] + `<div class="agent-seen">` + html.EscapeString(line) + `</div>` + row[cut:]
		}
	}
	return row
}

func platformRow(name string) string {
	a := Platform(name)
	if a == nil {
		return ""
	}

	for_ := strings.TrimSpace(a.Description)
	if for_ == "" {
		for_ = toolWords(a.Tools)
	}
	if len(a.Tools) == 0 {
		for_ = "Everything"
	}
	// The default says so, because nothing else on the row does. It has no Edit
	// and no Remove, which is a difference you notice only by comparing it with
	// a row underneath — and on an account with no agents of its own there is
	// nothing to compare it to.
	if strings.EqualFold(name, DefaultPlatformAgent) {
		for_ = "The default — " + strings.ToLower(for_[:1]) + for_[1:]
	}

	// No Edit and no Remove: this one is the instance's, not yours. Making one
	// of your own starts from the builder rather than from this row.
	path := "/agent/" + strings.ToLower(name)
	return entryRow(entry{
		Name: a.Name,
		Path: path,
		Chat: path,
		For:  for_,
		ID:   strings.ToLower(name),
	})
}

// seenLine is what an agent last dealt with, and seenWhen is how long ago.
//
// Two functions because they go in two places on the row: the subject under the
// name, the time out to the right. No "Last:" in front of either — a label on a
// line that is obviously the last thing is a word the reader has to step over
// to get to the only part that says anything.
//
// "Not used yet" rather than nothing, because a blank where every other row has
// a line reads as a render that failed — and because an agent you made and
// never wrote to is a real state worth seeing on the list that would let you.
func seenLine(accountID, agentID string) string {
	about, at := LastSeen(accountID, agentID)
	if at.IsZero() {
		return "Not used yet"
	}
	if about == "" {
		return "a conversation"
	}
	return trimTo(about, seenChars)
}

func seenWhen(accountID, agentID string) string {
	if _, at := LastSeen(accountID, agentID); !at.IsZero() {
		return app.TimeAgo(at)
	}
	return ""
}

// seenChars bounds the subject on a row. Long enough to recognise a
// conversation, short enough that the line stays one.
const seenChars = 50

func toolWords(tools []string) string {
	var out []string
	seen := map[string]bool{}
	for _, t := range tools {
		name := t
		if i := strings.IndexByte(t, '_'); i > 0 {
			name = t[:i]
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return "Everything"
	}
	return strings.Join(out, ", ")
}

const agentsCSS = `<style>
.lens-lead{color:#666;font-size:14px;margin:0 0 10px;max-width:640px}
/* The way to the catalogue, on its own line. It trailed the paragraph above
   and read as a footnote to the last clause, which is the wrong weight for the
   one place /tools is reachable from since it left the sidebar. */
.lens-go{font-size:14px;margin:0 0 18px}
.agent-note{color:#999;font-size:12px;margin:0 0 12px;max-width:640px}
/* The sign of life on a row. Quieter than the description above it, because
   what an agent is for is why you would pick it and when it last spoke is
   whether it is alive — the second is a check, not a heading. */
.agent-seen{color:#999;font-size:12px;margin:2px 0 0}
/* The name and how long ago it last spoke, on one line with the time out to
   the right — the same pair in the same places as a row in the inbox. */
.agent-head{display:flex;align-items:baseline;gap:10px}
.agent-when{margin-left:auto;flex:none;color:#aaa;font-size:11px;white-space:nowrap}
.agent-row .agent-seen{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
/* Top-aligned, not centred. A row is three or four lines tall now, and
   centring left Remove floating in the middle of the card beside nothing. */
.agent-row{display:flex;align-items:flex-start;gap:12px;border:1px solid #eee;border-radius:8px;padding:12px 14px}
/* No :hover here either. This filled its background grey, which is a row's
   answer — .thread-preview in the inbox — and these are not rows, they are
   bordered cards in a column, the same object as a tile on /services and a
   card on home. They lift now, from .card-hover in mu.css. */
/* One size, one colour, one weight for every link on a row.
   They were three: 12px grey in the link strip, 13px green or amber for the
   scope, 13px for the buttons beside them, and the name at 14px semibold. A
   row is one thing to read, so the parts that are the same rank look the
   same. */
.agent-links{display:flex;flex-wrap:wrap;align-items:center;gap:14px;margin-top:8px}
.agent-links a{font-size:13px;color:#666;text-decoration:none}
.agent-links a:hover{color:var(--text-primary,#111);text-decoration:underline}
/* The form holding Remove is one item in the strip, not a block that breaks it. */
.agent-links form{display:inline;margin:0}
/* What it is for. One line, and it truncates rather than wrapping — a list you
   are scanning stops being a list the moment the rows are different heights. */
.agent-for{font-size:13px;color:#666;margin-top:2px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.agent-meta{font-size:13px;color:#999;margin-top:2px;overflow:hidden;text-overflow:ellipsis}
.agent-meta code{font-size:12px}
.agent-mail{font-size:13px;margin-top:3px}
.agent-mail code{font-size:12px;color:#666;background:#f5f5f5;border-radius:3px;padding:1px 5px}
/* The agent's name is the way into it, and it looks like body text rather
   than one more small grey control. */
.agent-name{display:inline-block;font-weight:600;font-size:14px;color:var(--text-primary,#111);text-decoration:none}
/* No underline: the row behind it is the affordance now, and two of them at
   once is one too many.
   Stated on :hover rather than left to the rule above, which is the whole
   point. mu.css has a global a:hover rule setting underline, and that
   selector is (0,1,1) against .agent-name's (0,1,0) — so deleting this line
   does not remove the underline, it hands it to the global rule. It has to
   outrank it, at (0,2,0). */
.agent-name:hover{text-decoration:none}
/* Two buttons that did the same thing to the eye and different things to the
   account. Both were #bbb — barely visible — and both took the same red hover,
   which said "destructive" about issuing a token. Now issuing reads as an
   ordinary action and only Remove goes red, which is the one that is. */
.agent-act{background:none;border:0;color:#666;font-size:13px;cursor:pointer;padding:0;font-family:inherit}
.agent-act:hover{color:#111;text-decoration:underline}
.agent-remove{background:none;border:0;color:#999;font-size:13px;cursor:pointer;padding:0;font-family:inherit}
.agent-remove:hover{color:#b00;text-decoration:underline}
.agent-by{font-size:12px;color:#999}
.agent-mine{font-size:12px;color:#999}
.agent-publish{display:flex;align-items:center;gap:8px;margin:6px 0 0}
.agent-pub{display:flex;align-items:center;gap:6px;font-size:12px;color:#666;cursor:pointer}
.agent-pub input{width:auto;flex:none;margin:0}
.agent-secret{background:#f5f5f5;padding:10px 12px;font-size:12px;overflow-x:auto;border-radius:6px;margin:0;word-break:break-all}
/* On a phone a row is a card, not a line.
   It was a flex row with the name and its scope squeezed into whatever the
   Remove button and the endpoint left over, so an agent's own address wrapped
   to three lines and its name to two. Stacked, the controls go to the bottom
   where they are reachable with a thumb, and the endpoint scrolls inside its own
   line rather than pushing the page sideways. */
@media(max-width:600px){
  /* The strip's tap targets stand taller than their labels, so the card gives
     back the space below them — see .agent-links. */
  .agent-row{flex-direction:column;align-items:stretch;gap:8px;padding:12px 14px 4px}
  .agent-name{font-size:15px}
    .agent-meta{white-space:normal;overflow-wrap:anywhere}
  /* Every action in the strip is a tap target, so they are all one height.
     Buttons take min-height from the mobile rule in mu.css and links did not,
     so Remove stood 16px taller than Chat beside it and the strip grew to fit
     it.

     A 14px label centred in a 32px target carries 9px of its own space above
     and below. That is the spacing, not an addition to it: with a margin on
     top of it the description sat 21px clear of Chat where the desktop row
     sits 8px clear, which is the gap you can see before the buttons. The
     margin and the card's bottom padding give theirs back and the target
     keeps its height. */
  .agent-links{gap:16px;margin-top:0}
  .agent-links a,.agent-act,.agent-remove{
    display:inline-flex;align-items:center;min-height:32px;font-size:14px;padding:0}
}
.agent-input{display:block;width:100%;padding:9px 11px;border:1px solid #d1d5db;border-radius:6px;font-size:14px;font-family:inherit;margin:0 0 10px}
.agent-scope-pick{border-top:1px solid #eee;padding-top:12px;margin:0 0 14px}
</style>` + chipCSS

// chipCSS is the selection control both agent pages use.
//
// An <input type=checkbox> next to text never lines up: the box sits on the
// text baseline, its height is the browser's rather than the line's, and every
// fix is a different magic number per browser. Hiding the input and styling its
// label removes the alignment problem instead of tuning it — there is nothing
// inline left to misalign.
//
// Shared because /agents was built this way and /agent/new was not, so the same
// choice — which services or tools may this agent reach — was a row of neat
// chips on one page and a column of drifting checkboxes on the other.
// The segmented "where does it run" control moved in here with the chips when
// creating an agent stopped happening in two places: the builder is the one
// form now, so the styles it needs have to travel with it rather than living
// on the roster page that used to own the other copy.
const chipCSS = `<style>
.pick input,.chip input{position:absolute;opacity:0;width:0;height:0}
.pick input:focus-visible+span,.chip input:focus-visible+span{outline:2px solid #111;outline-offset:2px}
.agent-services{display:flex;flex-wrap:wrap;gap:6px}
.chip span{display:block;border:1px solid #ddd;border-radius:6px;padding:6px 12px;
  cursor:pointer;font-size:13px;color:#444;white-space:nowrap}
.chip span:hover{border-color:#bbb}
.chip input:checked+span{background:#111;border-color:#111;color:#fff}
.pick-row{display:grid;grid-template-columns:1fr 1fr;gap:8px;margin:0 0 14px}
.pick span{display:block;height:100%;border:1px solid #ddd;border-radius:8px;padding:10px 12px;
  cursor:pointer;font-size:12px;color:#666;line-height:1.4}
.pick strong{display:block;font-size:13px;color:#111;margin:0 0 2px}
.pick span:hover{border-color:#bbb}
.pick input:checked+span{border-color:#111;background:#fafafa}
.pick input:checked+span strong::after{content:" ✓";color:#0a7d33}
@media only screen and (max-width:600px){.pick-row{grid-template-columns:1fr}}
</style>`
